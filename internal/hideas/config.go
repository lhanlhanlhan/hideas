package hideas

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DB     string
	Server string
	Token  string
}

func defaultConfigPath() string {
	if v := os.Getenv("HIDEAS_CONFIG"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".hideas", "config")
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
		case "db":
			cfg.DB = value
		case "server":
			cfg.Server = value
		case "token":
			cfg.Token = value
		}
	}
	return cfg, scanner.Err()
}
