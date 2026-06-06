package hideas

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	defaultSessionTTL   = 7 * 24 * time.Hour
	defaultChallengeTTL = 60 * time.Second
)

type Credentials struct {
	Servers map[string]CredentialEntry `json:"servers"`
}

type CredentialEntry struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type ServerAuthConfig struct {
	StaticToken        string
	AuthorizedKeysPath string
	SessionTTL         time.Duration
	ChallengeTTL       time.Duration
}

type serverAuth struct {
	staticToken  string
	sessionTTL   time.Duration
	challengeTTL time.Duration

	mu            sync.Mutex
	allowedKeys   map[string]ssh.PublicKey
	challenges    map[string]authChallenge
	sessionTokens map[string]issuedToken
}

type authChallenge struct {
	PublicChallenge []byte
	ExpiresAt       time.Time
}

type issuedToken struct {
	Token     string
	ExpiresAt time.Time
	PublicKey string
}

type authChallengeResponse struct {
	ChallengeID string `json:"challenge_id"`
	Challenge   string `json:"challenge"`
	ExpiresAt   int64  `json:"expires_at"`
}

type authLoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

func newServerAuth(cfg ServerAuthConfig) (*serverAuth, error) {
	auth := &serverAuth{
		staticToken:   strings.TrimSpace(cfg.StaticToken),
		sessionTTL:    cfg.SessionTTL,
		challengeTTL:  cfg.ChallengeTTL,
		allowedKeys:   map[string]ssh.PublicKey{},
		challenges:    map[string]authChallenge{},
		sessionTokens: map[string]issuedToken{},
	}
	if auth.sessionTTL <= 0 {
		auth.sessionTTL = defaultSessionTTL
	}
	if auth.challengeTTL <= 0 {
		auth.challengeTTL = defaultChallengeTTL
	}
	if strings.TrimSpace(cfg.AuthorizedKeysPath) == "" {
		return auth, nil
	}
	keys, err := loadAuthorizedKeys(cfg.AuthorizedKeysPath)
	if err != nil {
		return nil, err
	}
	auth.allowedKeys = keys
	return auth, nil
}

func (a *serverAuth) enabled() bool {
	return a != nil && (a.staticToken != "" || len(a.allowedKeys) > 0)
}

func (a *serverAuth) checkBearer(header string) bool {
	if !a.enabled() {
		return true
	}
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		return false
	}
	if a.staticToken != "" && token == a.staticToken {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(time.Now().UTC())
	session, ok := a.sessionTokens[token]
	return ok && session.ExpiresAt.After(time.Now().UTC())
}

func (a *serverAuth) issueChallenge() (authChallengeResponse, error) {
	if a == nil || len(a.allowedKeys) == 0 {
		return authChallengeResponse{}, errors.New("ssh login is not configured")
	}
	challengeID, err := randomToken(24)
	if err != nil {
		return authChallengeResponse{}, err
	}
	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return authChallengeResponse{}, err
	}
	expiresAt := time.Now().UTC().Add(a.challengeTTL)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(time.Now().UTC())
	a.challenges[challengeID] = authChallenge{PublicChallenge: payload, ExpiresAt: expiresAt}
	return authChallengeResponse{
		ChallengeID: challengeID,
		Challenge:   base64.StdEncoding.EncodeToString(payload),
		ExpiresAt:   expiresAt.UnixMilli(),
	}, nil
}

func (a *serverAuth) login(challengeID, publicKeyText string, signature []byte) (authLoginResponse, error) {
	if a == nil || len(a.allowedKeys) == 0 {
		return authLoginResponse{}, errors.New("ssh login is not configured")
	}
	now := time.Now().UTC()
	a.mu.Lock()
	challenge, ok := a.challenges[challengeID]
	if ok {
		delete(a.challenges, challengeID)
	}
	a.pruneLocked(now)
	a.mu.Unlock()
	if !ok || !challenge.ExpiresAt.After(now) {
		return authLoginResponse{}, errors.New("invalid or expired challenge")
	}

	normalizedKey, key, err := parseAuthorizedKeyLine(publicKeyText)
	if err != nil {
		return authLoginResponse{}, errors.New("invalid public key")
	}
	allowed, ok := a.allowedKeys[normalizedKey]
	if !ok || string(allowed.Marshal()) != string(key.Marshal()) {
		return authLoginResponse{}, errors.New("public key is not authorized")
	}

	var sig ssh.Signature
	if err := ssh.Unmarshal(signature, &sig); err != nil {
		return authLoginResponse{}, errors.New("invalid signature")
	}
	if err := key.Verify(challenge.PublicChallenge, &sig); err != nil {
		return authLoginResponse{}, errors.New("signature verification failed")
	}

	token, err := randomToken(32)
	if err != nil {
		return authLoginResponse{}, err
	}
	expiresAt := now.Add(a.sessionTTL)
	a.mu.Lock()
	a.sessionTokens[token] = issuedToken{Token: token, ExpiresAt: expiresAt, PublicKey: normalizedKey}
	a.mu.Unlock()
	return authLoginResponse{Token: token, ExpiresAt: expiresAt.UnixMilli()}, nil
}

func (a *serverAuth) pruneLocked(now time.Time) {
	for id, challenge := range a.challenges {
		if !challenge.ExpiresAt.After(now) {
			delete(a.challenges, id)
		}
	}
	for token, session := range a.sessionTokens {
		if !session.ExpiresAt.After(now) {
			delete(a.sessionTokens, token)
		}
	}
}

func loadAuthorizedKeys(path string) (map[string]ssh.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	keys := map[string]ssh.PublicKey{}
	rest := b
	for len(rest) > 0 {
		pub, _, _, next, err := ssh.ParseAuthorizedKey(rest)
		if err != nil {
			return nil, err
		}
		normalized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
		keys[normalized] = pub
		rest = next
	}
	if len(keys) == 0 {
		return nil, errors.New("authorized keys file is empty")
	}
	return keys, nil
}

func parseAuthorizedKeyLine(v string) (string, ssh.PublicKey, error) {
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(v)))
	if err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))), pub, nil
}

func signChallenge(identityPath, challengeBase64 string) (string, string, error) {
	b, err := os.ReadFile(identityPath)
	if err != nil {
		return "", "", err
	}
	signer, err := ssh.ParsePrivateKey(b)
	if err != nil {
		return "", "", err
	}
	challenge, err := base64.StdEncoding.DecodeString(challengeBase64)
	if err != nil {
		return "", "", err
	}
	sig, err := signer.Sign(rand.Reader, challenge)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), base64.StdEncoding.EncodeToString(ssh.Marshal(sig)), nil
}

func defaultCredentialsPath() string {
	if v := os.Getenv("HIDEAS_CREDENTIALS"); v != "" {
		return v
	}
	dir := defaultHideasDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "credentials.json")
}

func loadCredentials(path string) (Credentials, error) {
	if path == "" {
		path = defaultCredentialsPath()
	}
	if path == "" {
		return Credentials{}, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Credentials{}, nil
	}
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal(b, &creds); err != nil {
		return Credentials{}, err
	}
	if creds.Servers == nil {
		creds.Servers = map[string]CredentialEntry{}
	}
	return creds, nil
}

func saveCredentials(path string, creds Credentials) error {
	if path == "" {
		path = defaultCredentialsPath()
	}
	if path == "" {
		return errors.New("credentials path is not available")
	}
	if creds.Servers == nil {
		creds.Servers = map[string]CredentialEntry{}
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0700); err != nil && !errors.Is(err, os.ErrPermission) {
			return err
		}
	}
	b, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func credentialForServer(path, server string) (CredentialEntry, bool, error) {
	creds, err := loadCredentials(path)
	if err != nil {
		return CredentialEntry{}, false, err
	}
	entry, ok := creds.Servers[normalizeServerKey(server)]
	if !ok {
		return CredentialEntry{}, false, nil
	}
	if entry.ExpiresAt != 0 && entry.ExpiresAt <= time.Now().UTC().UnixMilli() {
		return CredentialEntry{}, false, nil
	}
	return entry, true, nil
}

func storeCredential(path, server string, entry CredentialEntry) error {
	creds, err := loadCredentials(path)
	if err != nil {
		return err
	}
	if creds.Servers == nil {
		creds.Servers = map[string]CredentialEntry{}
	}
	creds.Servers[normalizeServerKey(server)] = entry
	return saveCredentials(path, creds)
}

func removeCredential(path, server string) error {
	creds, err := loadCredentials(path)
	if err != nil {
		return err
	}
	if creds.Servers == nil {
		return nil
	}
	delete(creds.Servers, normalizeServerKey(server))
	return saveCredentials(path, creds)
}

func normalizeServerKey(server string) string {
	return strings.TrimRight(strings.TrimSpace(server), "/")
}

func randomToken(numBytes int) (string, error) {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validateCredentialFileMode(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("credentials file must not be accessible by group or others: %s", path)
	}
	return nil
}
