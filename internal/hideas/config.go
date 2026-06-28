package hideas

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the on-disk hideas configuration shared by the CLI client and the
// hideas serve process. It uses TOML as the file format.
type Config struct {
	// Server is the hideas server URL used by the CLI client.
	Server string `toml:"server"`
	// Token is an optional static bearer token. On the client, it overrides any
	// stored credentials. On the server, it enables a CI/self-test login path.
	Token string `toml:"token"`
	// CredentialsPath is the client-side credentials file path.
	CredentialsPath string `toml:"credentials"`

	// Server-only fields.
	DB       string `toml:"db"`
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	BasePath string `toml:"base_path"`

	SSO SSOConfig `toml:"sso"`
}

// SSOConfig describes how the hideas server reaches an OIDC-compatible SSO.
type SSOConfig struct {
	Issuer       string `toml:"issuer"`
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
	RedirectURL  string `toml:"redirect_url"`
	Scopes       string `toml:"scopes"`
}

func defaultHideasDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".hideas")
}

func defaultConfigPath() string {
	if v := os.Getenv("HIDEAS_CONFIG"); v != "" {
		return v
	}
	dir := defaultHideasDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config")
}

func loadConfig(path string) (Config, error) {
	if path == "" {
		path = defaultConfigPath()
	}
	if path == "" {
		return Config{}, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func saveConfig(path string, cfg Config) error {
	if path == "" {
		path = defaultConfigPath()
	}
	if path == "" {
		return errors.New("config path is not available")
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0700); err != nil && !os.IsPermission(err) {
			return err
		}
	}
	var b strings.Builder
	if err := toml.NewEncoder(&b).Encode(cfg); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

// applySSOEnv overlays SSO settings from environment variables. Empty env
// values do not clear configured fields.
func (c *Config) applySSOEnv() {
	if v := os.Getenv("HIDEAS_SSO_ISSUER"); v != "" {
		c.SSO.Issuer = v
	}
	if v := os.Getenv("HIDEAS_SSO_CLIENT_ID"); v != "" {
		c.SSO.ClientID = v
	}
	if v := os.Getenv("HIDEAS_SSO_CLIENT_SECRET"); v != "" {
		c.SSO.ClientSecret = v
	}
	if v := os.Getenv("HIDEAS_SSO_REDIRECT_URL"); v != "" {
		c.SSO.RedirectURL = v
	}
	if v := os.Getenv("HIDEAS_SSO_SCOPES"); v != "" {
		c.SSO.Scopes = v
	}
}
