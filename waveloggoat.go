package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// version is set at build time using ldflags
var version = "dev"

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

// interface for interacting with a radio source (flrig or hamlib)
type RadioClient interface {
	GetData() (RigData, error)
	SetData(freq float64, mode string) error
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

func main() {
	defaultConfig := GetDefaultConfig()

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
		fmt.Printf("WaveLogGoat version: %s\nGo version: %s\n", version, runtime.Version())
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
