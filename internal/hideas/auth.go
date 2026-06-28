package hideas

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultSessionTTL   = 7 * 24 * time.Hour
	defaultLoginTTL     = 10 * time.Minute
	defaultScopes       = "openid profile email"
	loginStatusPending  = "pending"
	loginStatusReady    = "ready"
	loginStatusExpired  = "expired"
	defaultHTTPTimeout  = 15 * time.Second
)

// Credentials is the on-disk credentials.json file. It stores per-server
// hideas session tokens and any pending SSO login session ID waiting for
// browser-side completion.
type Credentials struct {
	Servers map[string]CredentialEntry `json:"servers"`
}

type CredentialEntry struct {
	Token            string `json:"token,omitempty"`
	ExpiresAt        int64  `json:"expires_at,omitempty"`
	PendingSessionID string `json:"pending_session_id,omitempty"`
}

// ServerAuthConfig configures the server-side authenticator.
type ServerAuthConfig struct {
	StaticToken string
	SessionTTL  time.Duration
	LoginTTL    time.Duration
	SSO         SSOConfig
	HTTPClient  *http.Client
}

// serverAuth is the server-side authenticator. It holds:
//   - the optional static bearer token,
//   - issued hideas session tokens,
//   - pending SSO login sessions, indexed both by session ID and by OAuth state.
type serverAuth struct {
	staticToken string
	sessionTTL  time.Duration
	loginTTL    time.Duration
	sso         SSOConfig
	provider    *oidcProvider
	httpClient  *http.Client

	mu             sync.Mutex
	sessions       map[string]*loginSession // session_id -> session
	stateIndex     map[string]string        // oauth state -> session_id
	sessionTokens  map[string]issuedToken   // bearer token -> issued info
}

type loginSession struct {
	ID            string
	State         string
	CodeVerifier  string
	Status        string
	Token         string
	Subject       string
	ExpiresAt     time.Time
	TokenExpires  time.Time
	Error         string
	CreatedAt     time.Time
}

type issuedToken struct {
	Token     string
	ExpiresAt time.Time
	Subject   string
}

type authLoginStartResponse struct {
	SessionID    string `json:"session_id"`
	AuthorizeURL string `json:"authorize_url"`
	ExpiresAt    int64  `json:"expires_at"`
}

type authLoginPollResponse struct {
	Status    string `json:"status"`
	Token     string `json:"token,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

type authMeResponse struct {
	Subject   string `json:"subject"`
	ExpiresAt int64  `json:"expires_at"`
}

// oidcProvider holds the OIDC endpoints discovered from the issuer.
type oidcProvider struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

func newServerAuth(cfg ServerAuthConfig) (*serverAuth, error) {
	auth := &serverAuth{
		staticToken:   strings.TrimSpace(cfg.StaticToken),
		sessionTTL:    cfg.SessionTTL,
		loginTTL:      cfg.LoginTTL,
		sso:           cfg.SSO,
		httpClient:    cfg.HTTPClient,
		sessions:      map[string]*loginSession{},
		stateIndex:    map[string]string{},
		sessionTokens: map[string]issuedToken{},
	}
	if auth.sessionTTL <= 0 {
		auth.sessionTTL = defaultSessionTTL
	}
	if auth.loginTTL <= 0 {
		auth.loginTTL = defaultLoginTTL
	}
	if auth.httpClient == nil {
		auth.httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if auth.ssoConfigured() {
		provider, err := discoverOIDC(auth.httpClient, auth.sso.Issuer)
		if err != nil {
			return nil, fmt.Errorf("oidc discovery failed: %w", err)
		}
		auth.provider = provider
	}
	return auth, nil
}

// ssoConfigured reports whether SSO login can be served. It requires the full
// set of OIDC client credentials and redirect URL.
func (a *serverAuth) ssoConfigured() bool {
	return a != nil &&
		strings.TrimSpace(a.sso.Issuer) != "" &&
		strings.TrimSpace(a.sso.ClientID) != "" &&
		strings.TrimSpace(a.sso.ClientSecret) != "" &&
		strings.TrimSpace(a.sso.RedirectURL) != ""
}

// enabled reports whether any authentication mechanism is configured. When
// false, the server serves data unauthenticated. This matches the local-dev
// fallback used by the test suite and personal-use bare-bones deployments.
func (a *serverAuth) enabled() bool {
	return a != nil && (a.staticToken != "" || a.ssoConfigured())
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

// subjectFor returns the subject claim bound to a bearer token, when it was
// issued through SSO. Static tokens have no subject.
func (a *serverAuth) subjectFor(header string) (issuedToken, bool) {
	if !strings.HasPrefix(header, "Bearer ") {
		return issuedToken{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(time.Now().UTC())
	info, ok := a.sessionTokens[token]
	return info, ok && info.ExpiresAt.After(time.Now().UTC())
}

// loginStart creates a new login session and returns the authorize URL the
// user should open in a browser. The session ID is then used by the CLI to
// poll for completion.
func (a *serverAuth) loginStart() (authLoginStartResponse, error) {
	if !a.ssoConfigured() || a.provider == nil {
		return authLoginStartResponse{}, errors.New("sso login is not configured")
	}
	sessionID, err := randomToken(24)
	if err != nil {
		return authLoginStartResponse{}, err
	}
	state, err := randomToken(24)
	if err != nil {
		return authLoginStartResponse{}, err
	}
	verifier, err := randomToken(32)
	if err != nil {
		return authLoginStartResponse{}, err
	}
	now := time.Now().UTC()
	session := &loginSession{
		ID:           sessionID,
		State:        state,
		CodeVerifier: verifier,
		Status:       loginStatusPending,
		CreatedAt:    now,
		ExpiresAt:    now.Add(a.loginTTL),
	}
	a.mu.Lock()
	a.pruneLocked(now)
	a.sessions[sessionID] = session
	a.stateIndex[state] = sessionID
	a.mu.Unlock()

	scopes := strings.TrimSpace(a.sso.Scopes)
	if scopes == "" {
		scopes = defaultScopes
	}
	challenge := pkceChallenge(verifier)
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", a.sso.ClientID)
	q.Set("redirect_uri", a.sso.RedirectURL)
	q.Set("scope", scopes)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	authURL := a.provider.AuthorizationEndpoint
	if strings.Contains(authURL, "?") {
		authURL += "&" + q.Encode()
	} else {
		authURL += "?" + q.Encode()
	}
	return authLoginStartResponse{
		SessionID:    sessionID,
		AuthorizeURL: authURL,
		ExpiresAt:    session.ExpiresAt.UnixMilli(),
	}, nil
}

// loginCallback finishes the SSO flow: exchange code -> access_token,
// call userinfo to obtain the subject, mint a hideas session token, and mark
// the matching login session as ready so the CLI's next poll succeeds.
func (a *serverAuth) loginCallback(ctx context.Context, state, code string) error {
	if !a.ssoConfigured() || a.provider == nil {
		return errors.New("sso login is not configured")
	}
	a.mu.Lock()
	sessionID, ok := a.stateIndex[state]
	if !ok {
		a.mu.Unlock()
		return errors.New("unknown or expired state")
	}
	session, ok := a.sessions[sessionID]
	if !ok {
		delete(a.stateIndex, state)
		a.mu.Unlock()
		return errors.New("unknown or expired state")
	}
	now := time.Now().UTC()
	if !session.ExpiresAt.After(now) {
		session.Status = loginStatusExpired
		a.mu.Unlock()
		return errors.New("login session expired")
	}
	verifier := session.CodeVerifier
	a.mu.Unlock()

	tokenResp, err := exchangeCode(ctx, a.httpClient, a.provider.TokenEndpoint, a.sso.ClientID, a.sso.ClientSecret, code, verifier, a.sso.RedirectURL)
	if err != nil {
		a.failSession(sessionID, err.Error())
		return err
	}
	subject, err := fetchUserInfo(ctx, a.httpClient, a.provider.UserInfoEndpoint, tokenResp.AccessToken)
	if err != nil {
		a.failSession(sessionID, err.Error())
		return err
	}
	token, err := randomToken(32)
	if err != nil {
		a.failSession(sessionID, err.Error())
		return err
	}
	expiresAt := now.Add(a.sessionTTL)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessionTokens[token] = issuedToken{Token: token, ExpiresAt: expiresAt, Subject: subject}
	session.Status = loginStatusReady
	session.Token = token
	session.TokenExpires = expiresAt
	session.Subject = subject
	delete(a.stateIndex, state)
	return nil
}

func (a *serverAuth) failSession(sessionID, msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if session, ok := a.sessions[sessionID]; ok {
		session.Status = loginStatusExpired
		session.Error = msg
		delete(a.stateIndex, session.State)
	}
}

// loginPoll returns the current state of a login session. Once the response
// status is "ready", the token is returned exactly once; subsequent polls of
// the same session report "expired".
func (a *serverAuth) loginPoll(sessionID string) (authLoginPollResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now().UTC()
	a.pruneLocked(now)
	session, ok := a.sessions[sessionID]
	if !ok {
		return authLoginPollResponse{Status: loginStatusExpired}, nil
	}
	if !session.ExpiresAt.After(now) && session.Status != loginStatusReady {
		session.Status = loginStatusExpired
	}
	resp := authLoginPollResponse{Status: session.Status, Error: session.Error}
	if session.Status == loginStatusReady {
		resp.Token = session.Token
		resp.ExpiresAt = session.TokenExpires.UnixMilli()
		// Consume the session so the token is only delivered once.
		delete(a.sessions, sessionID)
		delete(a.stateIndex, session.State)
	}
	return resp, nil
}

func (a *serverAuth) pruneLocked(now time.Time) {
	for id, session := range a.sessions {
		if session.Status == loginStatusReady {
			continue
		}
		if !session.ExpiresAt.After(now) {
			delete(a.sessions, id)
			delete(a.stateIndex, session.State)
		}
	}
	for token, info := range a.sessionTokens {
		if !info.ExpiresAt.After(now) {
			delete(a.sessionTokens, token)
		}
	}
}

// discoverOIDC fetches the OIDC discovery document and returns the resolved
// endpoint URLs.
func discoverOIDC(client *http.Client, issuer string) (*oidcProvider, error) {
	base := strings.TrimRight(strings.TrimSpace(issuer), "/")
	if base == "" {
		return nil, errors.New("issuer is empty")
	}
	url := base + "/.well-known/openid-configuration"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery returned %d", resp.StatusCode)
	}
	var p oidcProvider
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	if p.AuthorizationEndpoint == "" || p.TokenEndpoint == "" || p.UserInfoEndpoint == "" {
		return nil, errors.New("incomplete discovery document")
	}
	return &p, nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	IDToken     string `json:"id_token,omitempty"`
}

func exchangeCode(ctx context.Context, client *http.Client, tokenURL, clientID, clientSecret, code, codeVerifier, redirectURL string) (tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURL)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code_verifier", codeVerifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return tokenResponse{}, fmt.Errorf("token exchange failed: %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return tokenResponse{}, err
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return tokenResponse{}, errors.New("empty access_token")
	}
	return out, nil
}

func fetchUserInfo(ctx context.Context, client *http.Client, userInfoURL, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("userinfo failed: %d", resp.StatusCode)
	}
	var info struct {
		Sub string `json:"sub"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	if strings.TrimSpace(info.Sub) == "" {
		return "", errors.New("userinfo missing sub")
	}
	return info.Sub, nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
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

// credentialForServer returns the stored entry for a server, treating expired
// tokens as missing. Pending sessions are surfaced regardless of token state.
func credentialForServer(path, server string) (CredentialEntry, bool, error) {
	creds, err := loadCredentials(path)
	if err != nil {
		return CredentialEntry{}, false, err
	}
	entry, ok := creds.Servers[normalizeServerKey(server)]
	if !ok {
		return CredentialEntry{}, false, nil
	}
	if entry.Token != "" && entry.ExpiresAt != 0 && entry.ExpiresAt <= time.Now().UTC().UnixMilli() {
		entry.Token = ""
		entry.ExpiresAt = 0
	}
	if entry.Token == "" && entry.PendingSessionID == "" {
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
