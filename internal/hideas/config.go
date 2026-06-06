package hideas

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ModeLocal        = "local"
	ModeRemoteClient = "remote-client"
)

type Config struct {
	Mode            string
	DB              string
	Server          string
	Token           string
	Identity        string
	CredentialsPath string
	AuthorizedKeys  string
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
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	var cfg Config
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "mode":
			cfg.Mode = value
		case "db":
			cfg.DB = value
		case "server":
			cfg.Server = value
		case "token":
			cfg.Token = value
		case "identity":
			cfg.Identity = value
		case "credentials":
			cfg.CredentialsPath = value
		case "authorized_keys":
			cfg.AuthorizedKeys = value
		}
	}
	return cfg, scanner.Err()
}

func saveConfig(path string, cfg Config) error {
	if path == "" {
		path = defaultConfigPath()
	}
	if path == "" {
		return fmt.Errorf("config path is not available")
	}
	if cfg.Mode == ModeLocal {
		cfg.Mode = ""
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
	writeConfigLine := func(key, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "%s = %q\n", key, value)
	}
	writeConfigLine("mode", cfg.Mode)
	writeConfigLine("db", cfg.DB)
	writeConfigLine("server", cfg.Server)
	writeConfigLine("token", cfg.Token)
	writeConfigLine("identity", cfg.Identity)
	writeConfigLine("credentials", cfg.CredentialsPath)
	writeConfigLine("authorized_keys", cfg.AuthorizedKeys)
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
