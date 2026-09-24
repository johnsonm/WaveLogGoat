package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ProfileConfig represents the configuration for a specific radio profile.
type ProfileConfig struct {
	WavelogURL      string  `json:"wavelog_url"`
	WavelogKey      string  `json:"wavelog_key"`
	WavelogKeyV1    string  `json:"wavelog_key_v1"`
	RadioName       string  `json:"radio_name"`
	FlrigHost       string  `json:"flrig_host"`
	FlrigPort       int     `json:"flrig_port"`
	HamlibHost      string  `json:"hamlib_host"`
	HamlibPort      int     `json:"hamlib_port"`
	MaxPower        float64 `json:"max_power"` // rig max RF power in watts; if >0 hamlib reports watts, else percent
	Interval        string  `json:"interval"`
	DataSource      string  `json:"data_source"`      // "flrig" or "hamlib"
	LogLevel        string  `json:"log_level"`        // "error", "warn", "info", "debug"
	WebSocketEnable bool    `json:"websocket_enable"` // enable WebSocket server
	WebSocketPort   int     `json:"websocket_port"`   // WebSocket server port (default: 54322)
	WSSEnable       bool    `json:"wss_enable"`       // enable WebSocket Secure server
	WSSPort         int     `json:"wss_port"`         // WebSocket Secure server port (default: 54323)
	QSYEnable       bool    `json:"qsy_enable"`       // enable HTTP QSY server
	QSYPort         int     `json:"qsy_port"`         // HTTP QSY server port (default: 54321)
	QSYEnableSSL    bool    `json:"qsy_enable_ssl"`   // enable HTTPS for QSY server (dual HTTP/HTTPS)
}

// ConfigFile represents the structure of the overall configuration file.
type ConfigFile struct {
	DefaultProfile string                   `json:"default_profile"`
	Profiles       map[string]ProfileConfig `json:"profiles"`
}

// GetDefaultConfig returns a default configuration object.
func GetDefaultConfig() ProfileConfig {
	return ProfileConfig{
		WavelogURL:      "http://localhost/index.php",
		WavelogKey:      "wl2_YOUR_API_KEY",
		RadioName:       "RIG",
		FlrigHost:       "127.0.0.1",
		FlrigPort:       12345,
		HamlibHost:      "127.0.0.1",
		HamlibPort:      4532,
		MaxPower:        100,
		Interval:        "1s",
		DataSource:      "flrig",
		LogLevel:        "error",
		WebSocketEnable: true,
		WebSocketPort:   54322,
		WSSEnable:       false,
		WSSPort:         54323,
		QSYEnable:       false,
		QSYPort:         54321,
		QSYEnableSSL:    false,
	}
}

// getConfigPath returns the path to the configuration file based on the OS.
func getConfigPath() (string, error) {
	var configDir string
	switch runtime.GOOS {
	case "windows":
		configDir = os.Getenv("APPDATA")
	case "darwin":
		configDir = filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
	case "linux":
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	configDir = filepath.Join(configDir, "WaveLogGoat")
	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}

// loadConfig reads the configuration from the specified path.
func loadConfig(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	var cfg ConfigFile
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	return cfg, nil
}

// saveConfig saves the configuration to the specified path.
func saveConfig(path string, cfg ConfigFile) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}
