package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"syncbot/models"
)

func getDefaultBasePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "syncbot")
}

type Device struct {
	Name        string `yaml:"name" json:"name"`
	URL         string `yaml:"url" json:"url"`
	Description string `yaml:"description" json:"description"`
	Location    string `yaml:"location" json:"location"`
}

// TerminalConfig stores settings for the web terminal feature
// Authentication is handled client-side with credentials in localStorage
type TerminalConfig struct {
	Shell string `yaml:"shell"`
}

type Config struct {
	// Device info
	DeviceName        string `yaml:"device_name"`
	DeviceDescription string `yaml:"device_description"`
	DeviceLocation    string `yaml:"device_location"`

	// Paths
	EndpointsPath         string `yaml:"endpoints_path"`
	ActiveSymlink         string `yaml:"active_symlink"`
	PostActivationCommand string `yaml:"post_activation_command"`

	// Network
	Port        int    `yaml:"port"`
	DeviceIP    string `yaml:"device_ip"`
	SSHUsername string `yaml:"ssh_username"`

	// Other devices registry
	Devices []Device `yaml:"devices"`

	// User-defined commands
	Commands []models.Command `yaml:"commands"`

	// Terminal settings
	Terminal TerminalConfig `yaml:"terminal"`

	configPath string `yaml:"-"`
}

func Load(path string) (*Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default config
			basePath := getDefaultBasePath()
			hostname, _ := os.Hostname()
			cfg = Config{
				DeviceName:            hostname,
				DeviceDescription:     "SyncBot Device",
				DeviceLocation:        "",
				EndpointsPath:         filepath.Join(basePath, "endpoints"),
				ActiveSymlink:         filepath.Join(basePath, "active"),
				PostActivationCommand: "",
				Port:                  8081,
				DeviceIP:              "localhost",
				SSHUsername:           "sync",
				Devices:               []Device{},
				Commands:              []models.Command{},
				configPath:            path,
			}
			if err := cfg.Save(); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}
			return &cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if cfg.EndpointsPath == "" {
		cfg.EndpointsPath = "/var/sync"
	}
	if cfg.ActiveSymlink == "" {
		cfg.ActiveSymlink = "/var/active"
	}
	if cfg.Port == 0 {
		cfg.Port = 8081
	}
	if cfg.DeviceIP == "" {
		cfg.DeviceIP = "localhost"
	}
	if cfg.SSHUsername == "" {
		cfg.SSHUsername = "sync"
	}
	if cfg.Terminal.Shell == "" {
		cfg.Terminal.Shell = "/bin/bash"
	}

	cfg.configPath = path

	return &cfg, nil
}

func (c *Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(c.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
