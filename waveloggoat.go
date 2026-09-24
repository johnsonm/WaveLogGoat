package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// version is set at build time using ldflags
var version = "dev"

// WebSocketMessage represents the JSON message sent to wavelog
// Matches WaveLogGate format exactly
type WebSocketMessage struct {
	Type        string `json:"type"`                   // radio_status
	Message     string `json:"message,omitempty"`      // Welcome message only
	Frequency   int    `json:"frequency,omitempty"`    // Frequency in Hz
	FrequencyRX int    `json:"frequency_rx,omitempty"` // RX frequency for split mode
	Mode        string `json:"mode,omitempty"`         // Operating mode
	Power       int    `json:"power,omitempty"`        // Power in watts
	Radio       string `json:"radio,omitempty"`        // Radio name
	Timestamp   int64  `json:"timestamp,omitempty"`    // Unix timestamp
}

// RigData holds the radio state as provided by flrig or hamlib.
type RigData struct {
	FreqVFOA   float64
	FreqVFOB   float64
	Mode       string
	ModeB      string
	Split      int
	Power      float64
	PowerValid bool
}

// WavelogJSONRequest matches the required JSON payload for the Wavelog API update.
type WavelogJSONRequest struct {
	Radio       string      `json:"radio"`
	Power       interface{} `json:"power,omitempty"`
	Frequency   int         `json:"frequency"`
	Mode        string      `json:"mode"`
	FrequencyRX int         `json:"frequency_rx,omitempty"`
	ModeRX      string      `json:"mode_rx,omitempty"`
}

type WavelogErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

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

type ConfigFile struct {
	DefaultProfile string                   `json:"default_profile"`
	Profiles       map[string]ProfileConfig `json:"profiles"`
}

// interface for interacting with a radio source (flrig or hamlib)
type RadioClient interface {
	GetData() (RigData, error)
	SetData(freq float64, mode string) error
}

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

func loadConfig(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err // Error includes file not found
	}
	var cfg ConfigFile
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	return cfg, nil
}

func saveConfig(path string, cfg ConfigFile) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

func setupLogging(levelStr string) {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		log.SetLevel(logrus.ErrorLevel)
		log.Errorf("Invalid log level '%s'. Defaulting to 'error'.", levelStr)
		return
	}
	log.SetLevel(level)
}

func fetchWavelogVersion(config ProfileConfig) (string, error) {
	url := config.WavelogURL + "/api/version"

	payload := map[string]string{"key": config.WavelogKeyV1}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: CustomDialer,
		},
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status: %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return result.Version, nil
}

func postToWavelog(config ProfileConfig, data RigData, version string) error {
	payload := WavelogJSONRequest{
		Radio:     config.RadioName,
		Frequency: int(data.FreqVFOA),
		Mode:      data.Mode,
	}

	if data.PowerValid {
		maxPower := config.MaxPower
		if maxPower == 0.0 {
			maxPower = 100
		}
		power := data.Power / 100 * maxPower
		if version == "3.2.0" {
			payload.Power = int(power)
		} else {
			payload.Power = power
		}
	}

	if data.Split != 0 {
		payload.Frequency = int(data.FreqVFOB)
		payload.Mode = data.ModeB
		payload.FrequencyRX = int(data.FreqVFOA)
		payload.ModeRX = data.Mode
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON payload: %w", err)
	}
	url := config.WavelogURL + "/api/v2/radio"
	log.Infof("Sending to %s: %s", url, string(jsonPayload))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.WavelogKey)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: CustomDialer,
		},
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		// Check for v1/v2 key mismatch
		var errResp WavelogErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			if errResp.Error.Code == "invalid_token" && strings.Contains(errResp.Error.Message, "legacy v1 API keys are not accepted") {
				log.Fatalf("Fatal: API v2 requires a wl2_ token. Please mint a new v2 token in Wavelog 'API Keys' with `radio:read` and `radio:write` scopes.")
			}
		}

		return fmt.Errorf("wavelog API returned non-200 status code: %d. Body: %s", resp.StatusCode, string(body))
	}

	return nil
}

func main() {
	defaultConfig := ProfileConfig{
		WavelogURL:      "http://localhost/index.php",
		WavelogKey:      "wl2_YOUR_API_KEY",
		RadioName:       "RIG",
		FlrigHost:       "127.0.0.1",
		FlrigPort:       12345,
		HamlibHost:      "127.0.0.1",
		HamlibPort:      4532,
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

	var currentProfileName string
	var saveProfileName string
	var setDefaultProfileName string
	var wavelogVersion string

	showVersion := flag.Bool("version", false, "Print version information and exit")

	flag.StringVar(&currentProfileName, "profile", "", "Select a named configuration profile to run (overrides default).")
	flag.StringVar(&saveProfileName, "save-profile", "", "Saves the current configuration flags (excluding this flag) to the specified profile name and exits.")
	flag.StringVar(&setDefaultProfileName, "set-default-profile", "", "Sets the default profile to the specified name and exits.")

	wavelogURL := flag.String("wavelog-url", defaultConfig.WavelogURL, "Wavelog API URL for radio status.")
	wavelogKey := flag.String("wavelog-key", defaultConfig.WavelogKey, "Wavelog API Key, starting with `wl2_`.")
	wavelogKeyV1 := flag.String("wavelog-key-v1", defaultConfig.WavelogKeyV1, "Wavelog V1 API Key.")
	radioName := flag.String("radio-name", defaultConfig.RadioName, "Name of the radio (e.g., FT-891).")
	maxPower := flag.Float64("max-power", defaultConfig.MaxPower, "Maximum RF power in watts (default 100).")
	flrigHost := flag.String("flrig-host", defaultConfig.FlrigHost, "flrig XML-RPC host address.")
	flrigPort := flag.Int("flrig-port", defaultConfig.FlrigPort, "flrig XML-RPC port.")
	hamlibHost := flag.String("hamlib-host", defaultConfig.HamlibHost, "Hamlib rigctld host address.")
	hamlibPort := flag.Int("hamlib-port", defaultConfig.HamlibPort, "Hamlib rigctld port.")
	interval := flag.String("interval", defaultConfig.Interval, "Polling interval (e.g., 1s, 1500ms).")
	dataSource := flag.String("data-source", defaultConfig.DataSource, "Data source: 'flrig' or 'hamlib'.")
	logLevel := flag.String("log-level", defaultConfig.LogLevel, "Logging level: 'debug', 'info', 'warn', or 'error'.")
	websocketEnable := flag.Bool("websocket-enable", defaultConfig.WebSocketEnable, "Enable WebSocket server for real-time radio status.")
	websocketPort := flag.Int("websocket-port", defaultConfig.WebSocketPort, "WebSocket server port (default: 54322).")
	wssEnable := flag.Bool("wss-enable", defaultConfig.WSSEnable, "Enable WebSocket Secure server (WSS) for encrypted connections.")
	wssPort := flag.Int("wss-port", defaultConfig.WSSPort, "WebSocket Secure server port (default: 54323).")
	qsyEnable := flag.Bool("qsy-enable", defaultConfig.QSYEnable, "Enable QSY HTTP server.")
	qsyPort := flag.Int("qsy-port", defaultConfig.QSYPort, "QSY HTTP server port (default: 54321).")
	qsyEnableSSL := flag.Bool("qsy-enable-ssl", defaultConfig.QSYEnableSSL, "Enable HTTPS for QSY server (dual HTTP/HTTPS on same port).")

	// Parse flags initially to handle the special -save-profile and -set-default-profile flags
	flag.Parse()

	if *showVersion {
		fmt.Println("WaveLogGoat version:", version)
		return
	}

	configPath, err := getConfigPath()
	if err != nil {
		log.Fatalf("Fatal: Could not determine configuration path: %v", err)
	}

	cfgFile := ConfigFile{
		DefaultProfile: "default",
		Profiles:       make(map[string]ProfileConfig),
	}
	loadedCfgFile, err := loadConfig(configPath)
	if err == nil {
		cfgFile = loadedCfgFile
	} else if !os.IsNotExist(err) {
		log.Warnf("Configuration file found but failed to load (%s). Starting with defaults. Error: %v", configPath, err)
	}

	profileToUse := cfgFile.DefaultProfile
	if currentProfileName != "" {
		profileToUse = currentProfileName
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
	// We need to re-parse flags but track if they were explicitly set.
	// Since the flag package doesn't natively expose "was set," we use the parsed values.
	// This approach means if a flag is *not* passed, we use the profile config value.

	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "wavelog-url":
			currentProfileConfig.WavelogURL = *wavelogURL
		case "wavelog-key":
			currentProfileConfig.WavelogKey = *wavelogKey
		case "wavelog-key-v1":
			currentProfileConfig.WavelogKeyV1 = *wavelogKeyV1
		case "radio-name":
			currentProfileConfig.RadioName = *radioName
		case "max-power":
			currentProfileConfig.MaxPower = *maxPower
		case "flrig-host":
			currentProfileConfig.FlrigHost = *flrigHost
		case "flrig-port":
			currentProfileConfig.FlrigPort = *flrigPort
		case "hamlib-host":
			currentProfileConfig.HamlibHost = *hamlibHost
		case "hamlib-port":
			currentProfileConfig.HamlibPort = *hamlibPort
		case "interval":
			currentProfileConfig.Interval = *interval
		case "data-source":
			currentProfileConfig.DataSource = *dataSource
		case "log-level":
			currentProfileConfig.LogLevel = *logLevel
		case "websocket-enable":
			currentProfileConfig.WebSocketEnable = *websocketEnable
		case "websocket-port":
			currentProfileConfig.WebSocketPort = *websocketPort
		case "wss-enable":
			currentProfileConfig.WSSEnable = *wssEnable
		case "wss-port":
			currentProfileConfig.WSSPort = *wssPort
		case "qsy-enable":
			currentProfileConfig.QSYEnable = *qsyEnable
		case "qsy-port":
			currentProfileConfig.QSYPort = *qsyPort
		case "qsy-enable-ssl":
			currentProfileConfig.QSYEnableSSL = *qsyEnableSSL
		}
	})

	if setDefaultProfileName != "" {
		if _, ok := cfgFile.Profiles[setDefaultProfileName]; !ok {
			log.Fatalf("Fatal: Cannot set default profile. Profile '%s' does not exist in the configuration file.", setDefaultProfileName)
		}
		cfgFile.DefaultProfile = setDefaultProfileName
		if err := saveConfig(configPath, cfgFile); err != nil {
			log.Fatalf("Fatal: Failed to save configuration file: %v", err)
		}
		fmt.Printf("Default profile successfully set to '%s'.\n", setDefaultProfileName)
		return
	}

	if saveProfileName != "" {
		if saveProfileName == "" {
			log.Fatalf("Fatal: The -save-profile flag requires a profile name.")
		}
		cfgFile.Profiles[saveProfileName] = currentProfileConfig
		if err := saveConfig(configPath, cfgFile); err != nil {
			log.Fatalf("Fatal: Failed to save configuration file: %v", err)
		}
		fmt.Printf("Configuration saved successfully to profile '%s' in %s\n", saveProfileName, configPath)
		return
	}

	ctx := context.Background()

	setupLogging(currentProfileConfig.LogLevel)

	if currentProfileConfig.WavelogKey == "" || currentProfileConfig.WavelogKey == defaultConfig.WavelogKey {
		log.Fatalf("Fatal: Wavelog API key is required. Please set via -wavelog-key or in the config file.")
	}

	if currentProfileConfig.WavelogKeyV1 != "" {
		wavelogVersion, err := fetchWavelogVersion(currentProfileConfig)
		if err != nil {
			log.Errorf("Failed to fetch Wavelog version: %v", err)
		} else {
			log.Infof("Wavelog API version: %s", wavelogVersion)
		}
	} else {
		log.Infof("Wavelog V1 API key not provided; skipping version check. Add -wavelog-key-v1 to see version info.")
	}

	if currentProfileConfig.WavelogURL == "" {
		log.Fatalf("Fatal: Wavelog URL is required.")
	}

	var client RadioClient
	switch strings.ToLower(currentProfileConfig.DataSource) {
	case "flrig":
		client, err = NewFlrigRadioClient(ctx, currentProfileConfig.FlrigHost, currentProfileConfig.FlrigPort)
		if err != nil {
			log.Fatalf("Fatal: Failed to create flrig client: %v", err)
		}
		log.Infof("Using flrig client at %s:%d (Profile: %s)", currentProfileConfig.FlrigHost, currentProfileConfig.FlrigPort, profileToUse)
	case "hamlib":
		client = NewHamlibClient(ctx, currentProfileConfig.HamlibHost, currentProfileConfig.HamlibPort, currentProfileConfig.MaxPower, &defaultConnectionProvider{})
		log.Infof("Using Hamlib client at %s:%d (Profile: %s)", currentProfileConfig.HamlibHost, currentProfileConfig.HamlibPort, profileToUse)
		log.Warnf("Hamlib support is untested and presumed broken. Please report success or failure to debug or remove this message!")
	default:
		log.Fatalf("Fatal: Invalid data source specified: '%s'. Must be 'flrig' or 'hamlib'.", currentProfileConfig.DataSource)
	}

	intervalDuration, err := time.ParseDuration(currentProfileConfig.Interval)
	if err != nil {
		log.Fatalf("Fatal: Invalid interval duration format: %v", err)
	}

	// Load or create SSL certificates if SSL is enabled
	var certPath, keyPath string
	sslEnabled := currentProfileConfig.WSSEnable || currentProfileConfig.QSYEnableSSL
	if sslEnabled {
		certPath, keyPath, err = loadOrCreateCertificate()
		if err != nil {
			log.Fatalf("Fatal: Failed to load or create SSL certificates: %v", err)
		}
	}

	// Track both WebSocket servers for broadcasting
	type wsServerRef struct {
		server *WebSocketServer
		isWSS  bool
	}
	var wsServers []wsServerRef

	var webSocketServer *WebSocketServer
	if currentProfileConfig.WebSocketEnable {
		var err error
		webSocketServer, err = startWebSocketServer(currentProfileConfig.WebSocketPort)
		if err != nil {
			log.Errorf("Failed to start WebSocket server: %v", err)
		} else {
			wsServers = append(wsServers, wsServerRef{server: webSocketServer, isWSS: false})
		}
	}

	var wssServer *WebSocketServer
	if currentProfileConfig.WSSEnable && sslEnabled {
		var err error
		wssServer, err = startWSSServer(currentProfileConfig.WSSPort, certPath, keyPath)
		if err != nil {
			log.Errorf("Failed to start WebSocket Secure server: %v", err)
		} else {
			wsServers = append(wsServers, wsServerRef{server: wssServer, isWSS: true})
		}
	} else if currentProfileConfig.WSSEnable && !sslEnabled {
		log.Warnf("WSS enabled but SSL certificates not available. WSS server not started.")
	}

	if currentProfileConfig.QSYEnable {
		_, err := startQSYServer(client, currentProfileConfig.QSYPort, currentProfileConfig.QSYEnableSSL, certPath, keyPath)
		if err != nil {
			log.Errorf("Failed to start QSY server: %v", err)
		}
	}

	var lastData RigData

	lastUpdate := time.Time{}
	log.Infof("Starting WaveLogGoat polling every %s...", intervalDuration)
	if currentProfileConfig.WebSocketEnable {
		log.Infof("WebSocket server enabled on port %d", currentProfileConfig.WebSocketPort)
	}
	if currentProfileConfig.WSSEnable {
		log.Infof("WebSocket Secure server enabled on port %d", currentProfileConfig.WSSPort)
	}
	if currentProfileConfig.QSYEnable {
		log.Infof("QSY HTTP server enabled on port %d", currentProfileConfig.QSYPort)
		log.Infof("QSY endpoint: http://localhost:%d/{frequency}/{mode}", currentProfileConfig.QSYPort)
		if currentProfileConfig.QSYEnableSSL {
			log.Infof("QSY HTTPS server enabled on port %d", currentProfileConfig.QSYPort)
			log.Infof("QSY HTTPS endpoint: https://localhost:%d/{frequency}/{mode}", currentProfileConfig.QSYPort)
		}
	}

	for {
		time.Sleep(intervalDuration)

		currentData, err := client.GetData()
		if err != nil {
			// Do not be noisy about connection errors, because flrig or hamlib may not yet/currently be started.
			// Wait patiently.
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() || strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "dial tcp") {
				log.Debugf("Connection error fetching radio data: %v", err)
			} else {
				log.Errorf("Error fetching radio data: %v", err)
			}
			continue
		}

		sinceLast := time.Now().Sub(lastUpdate)
		if currentData == lastData && sinceLast < time.Minute {
			log.Debug("Radio data unchanged. Skipping update.")
			continue
		}

		log.Infof("Radio state changed; freq: %.0f Hz, mode: %s). Updating Wavelog...", currentData.FreqVFOA, currentData.Mode)

		if err := postToWavelog(currentProfileConfig, currentData, wavelogVersion); err != nil {
			log.Errorf("Error posting to Wavelog: %v", err)
			continue
		}

		lastData = currentData
		lastUpdate = time.Now()
		log.Debug("Successfully updated Wavelog.")

		// Broadcast to all WebSocket clients (both WS and WSS) if enabled
		for _, wsRef := range wsServers {
			wsMessage := WebSocketMessage{
				Type:      "radio_status",
				Frequency: int(currentData.FreqVFOA),
				Mode:      currentData.Mode,
				Power:     int(currentData.Power),
				Radio:     currentProfileConfig.RadioName,
				Timestamp: time.Now().Unix(),
			}
			// Include frequency_rx for split mode (exactly like WaveLogGate)
			if currentData.Split != 0 {
				wsMessage.FrequencyRX = int(currentData.FreqVFOA)
				wsMessage.Frequency = int(currentData.FreqVFOB)
			}
			broadcastToWavelog(wsRef.server, wsMessage)
		}
	}
}
