package main

import (
	"encoding/json"
	"flag"
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

// LoadAndMergeConfig loads the config file, merges with CLI flags, and handles profile selection.
func LoadAndMergeConfig(cliConfig *CLIConfig, fs *flag.FlagSet) (ConfigFile, ProfileConfig, string, error) {
	defaultConfig := GetDefaultConfig()
	configPath, err := getConfigPath()
	if err != nil {
		return ConfigFile{}, ProfileConfig{}, "", fmt.Errorf("could not determine configuration path: %w", err)
	}

	cfgFile := ConfigFile{
		DefaultProfile: "default",
		Profiles:       make(map[string]ProfileConfig),
	}
	loadedCfgFile, err := loadConfig(configPath)
	if err == nil {
		cfgFile = loadedCfgFile
	} else if !os.IsNotExist(err) {
		return ConfigFile{}, ProfileConfig{}, "", fmt.Errorf("configuration file found but failed to load (%s): %w", configPath, err)
	}

	profileToUse := cfgFile.DefaultProfile
	if *cliConfig.Profile != "" {
		profileToUse = *cliConfig.Profile
	}
	if profileToUse == "" {
		profileToUse = "default"
	}

	// Merge configuration (Default -> File -> Flags)
	currentProfileConfig := defaultConfig
	if p, ok := cfgFile.Profiles[profileToUse]; ok {
		currentProfileConfig = p
	}

	// Override config with command-line flags (only those that were set explicitly)
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "wavelog-url":
			currentProfileConfig.WavelogURL = *cliConfig.WavelogURL
		case "wavelog-key":
			currentProfileConfig.WavelogKey = *cliConfig.WavelogKey
		case "wavelog-key-v1":
			currentProfileConfig.WavelogKeyV1 = *cliConfig.WavelogKeyV1
		case "radio-name":
			currentProfileConfig.RadioName = *cliConfig.RadioName
		case "max-power":
			currentProfileConfig.MaxPower = *cliConfig.MaxPower
		case "flrig-host":
			currentProfileConfig.FlrigHost = *cliConfig.FlrigHost
		case "flrig-port":
			currentProfileConfig.FlrigPort = *cliConfig.FlrigPort
		case "hamlib-host":
			currentProfileConfig.HamlibHost = *cliConfig.HamlibHost
		case "hamlib-port":
			currentProfileConfig.HamlibPort = *cliConfig.HamlibPort
		case "interval":
			currentProfileConfig.Interval = *cliConfig.Interval
		case "data-source":
			currentProfileConfig.DataSource = *cliConfig.DataSource
		case "log-level":
			currentProfileConfig.LogLevel = *cliConfig.LogLevel
		case "websocket-enable":
			currentProfileConfig.WebSocketEnable = *cliConfig.WebSocketEnable
		case "websocket-port":
			currentProfileConfig.WebSocketPort = *cliConfig.WebSocketPort
		case "wss-enable":
			currentProfileConfig.WSSEnable = *cliConfig.WSSEnable
		case "wss-port":
			currentProfileConfig.WSSPort = *cliConfig.WSSPort
		case "qsy-enable":
			currentProfileConfig.QSYEnable = *cliConfig.QSYEnable
		case "qsy-port":
			currentProfileConfig.QSYPort = *cliConfig.QSYPort
		case "qsy-enable-ssl":
			currentProfileConfig.QSYEnableSSL = *cliConfig.QSYEnableSSL
		}
	})

	// Handle initial config file creation
	if os.IsNotExist(err) {
		cfgFile.Profiles["default"] = defaultConfig
		if err := saveConfig(configPath, cfgFile); err != nil {
			return ConfigFile{}, ProfileConfig{}, "", fmt.Errorf("failed to save default configuration file: %w", err)
		}
		fmt.Printf("Configuration created successfully at %s\n", configPath)
	}

	return cfgFile, currentProfileConfig, profileToUse, nil
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

func setDefaultProfile(cliConfig *CLIConfig, cfgFile ConfigFile) bool {
	if *cliConfig.SetDefaultProfile == "" {
		return false
	}
	configPath, err := getConfigPath()
	if err != nil {
		log.Fatalf("Fatal: Could not determine configuration path: %v", err)
	}
	if _, ok := cfgFile.Profiles[*cliConfig.SetDefaultProfile]; !ok {
		log.Fatalf("Cannot set default profile. Profile '%s' does not exist in the configuration file.", *cliConfig.SetDefaultProfile)
	}
	cfgFile.DefaultProfile = *cliConfig.SetDefaultProfile
	if err := saveConfig(configPath, cfgFile); err != nil {
		log.Fatalf("Fatal: Failed to save configuration file: %v", err)
	}
	return true
}

func saveProfile(config ProfileConfig, cliConfig *CLIConfig, cfgFile ConfigFile) string {
	if *cliConfig.SaveProfile == "" {
		return ""
	}
	configPath, err := getConfigPath()
	if err != nil {
		log.Fatalf("Fatal: Could not determine configuration path: %v", err)
	}
	cfgFile.Profiles[*cliConfig.SaveProfile] = config
	if err := saveConfig(configPath, cfgFile); err != nil {
		log.Fatalf("Fatal: Failed to save configuration file: %v", err)
	}
	return configPath
}
