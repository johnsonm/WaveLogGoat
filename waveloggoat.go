package main

import (
	"context"
	"errors"
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
	var wavelogVersion string

	cliConfig, fs, err := ParseFlags(os.Args[1:], defaultConfig)
	if err != nil {
		log.Fatalf("Fatal: Failed to parse flags: %v", err)
	}

	if *cliConfig.ShowVersion {
		fmt.Printf("WaveLogGoat version: %s\nGo version: %s\n", version, runtime.Version())
		return
	}

	cfgFile, currentProfileConfig, profileToUse, err := LoadAndMergeConfig(cliConfig, fs)
	if err != nil {
		log.Fatalf("Fatal: %v", err)
	}

	profileSet := setDefaultProfile(cliConfig, cfgFile)
	if profileSet {
		fmt.Printf("Default profile successfully set to '%s'.\n", *cliConfig.SetDefaultProfile)
		return
	}

	configPath := saveProfile(currentProfileConfig, cliConfig, cfgFile)
	if configPath != "" {
		fmt.Printf("Configuration saved successfully to profile '%s' in %s\n", *cliConfig.SaveProfile, configPath)
		return
	}

	ctx := context.Background()

	setupLogging(currentProfileConfig.LogLevel)
	validateWavelogConfig(currentProfileConfig, defaultConfig)

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
