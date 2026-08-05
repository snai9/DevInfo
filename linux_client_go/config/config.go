package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var DefaultURLs = []string{
	"http://10.44.179.88:8080",
	"http://10.130.175.88:8080",
	"http://172.16.0.88:8080",
}

type Config struct {
	Used       string   `json:"used"`
	Unit       string   `json:"unit"`
	Dept       string   `json:"dept"`
	ServerURLs []string `json:"serverUrls"`
	ActiveURL  string   `json:"activeUrl"`
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "hwinfo")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "config.json")
}

func Load() *Config {
	path := getConfigPath()
	cfg := &Config{}

	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, cfg)
	}

	if cfg.ServerURLs == nil || len(cfg.ServerURLs) == 0 {
		cfg.ServerURLs = append([]string{}, DefaultURLs...)
	}
	if cfg.ActiveURL == "" && len(cfg.ServerURLs) > 0 {
		cfg.ActiveURL = cfg.ServerURLs[0]
	}

	return cfg
}

func (c *Config) Save() error {
	path := getConfigPath()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
