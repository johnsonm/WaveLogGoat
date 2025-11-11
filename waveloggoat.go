package main

import (
	"bufio"
	"bytes"
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
	"strconv"
	"strings"
	"time"

	"github.com/kolo/xmlrpc"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// version is set at build time using ldflags
var version = "dev"

// RigData holds the radio state as provided by flrig or hamlib.
type RigData struct {
	FreqVFOA float64
	FreqVFOB float64
	Mode   string
	ModeB    string
	Split    int
	Power    float64
}

// WavelogJSONRequest matches the required JSON payload for the Wavelog API update.
type WavelogJSONRequest struct {
	Key         string  `json:"key"`
	Radio       string  `json:"radio"`
	Power       float64 `json:"power"`
	Frequency   int     `json:"frequency"`
	Mode        string  `json:"mode"`
	FrequencyRX int     `json:"frequency_rx,omitempty"`
	ModeRX      string  `json:"mode_rx,omitempty"`
	// Split may come in a later WaveLog version
	// PTT may come in a a later WaveLog version
}

type ProfileConfig struct {
	WavelogURL string `json:"wavelog_url"`
	WavelogKey string `json:"wavelog_key"`
	RadioName  string `json:"radio_name"`
	FlrigHost  string `json:"flrig_host"`
	FlrigPort  int    `json:"flrig_port"`
	HamlibHost string `json:"hamlib_host"`
	HamlibPort int    `json:"hamlib_port"`
	Interval   string `json:"interval"`
	DataSource string `json:"data_source"` // "flrig" or "hamlib"
	LogLevel   string `json:"log_level"`   // "error", "warn", "info", "debug"
}

type ConfigFile struct {
	DefaultProfile string                   `json:"default_profile"`
	Profiles       map[string]ProfileConfig `json:"profiles"`
}

// interface for interacting with a radio source (flrig or hamlib)
type RadioClient interface {
	GetData() (RigData, error)
}

// implements RadioClient for XML-RPC communication with flrig
type FlrigClient struct {
	Host string
	Port int
}

// implements RadioClient for TCP communication with rigctld / hamlib
type HamlibClient struct {
	Host string
	Port int
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

func (f *FlrigClient) GetData() (RigData, error) {
	var data RigData
	var vfoA string
	var power int
	var vfoB string

	client, err := xmlrpc.NewClient(fmt.Sprintf("http://%s:%d/", f.Host, f.Port), nil)
	if err != nil {
		return data, err
	}
	defer client.Close()

	if err := client.Call("rig.get_vfo", nil, &vfoA); err != nil {
		return RigData{}, fmt.Errorf("call failed to rig.get_vfo: %w", err)
	}
	if data.FreqVFOA, err = strconv.ParseFloat(vfoA, 64); err != nil {
		log.Errorf("Failed to parse vfo frequency %s: %s", vfoA, err)
		return RigData{}, err
	}

	if err := client.Call("rig.get_mode", nil, &data.Mode); err != nil {
		return RigData{}, fmt.Errorf("call failed to rig.get_mode: %w", err)
	}

	if err := client.Call("rig.get_power", nil, &power); err != nil {
		log.Debugf("call failed to rig.get_power (flrig): %v. Sending 0 power.", err)
		power = 0
	}
	data.Power = float64(power)

	if err := client.Call("rig.get_split", nil, &data.Split); err != nil {
		log.Warnf("call failed to rig.get_split (flrig): %v. Sending Split=0.", err)
		data.Split = 0
	}

	if err := client.Call("rig.get_vfoB", nil, &vfoB); err != nil {
		log.Debugf("call failed to rig.get_vfoB (flrig): %v. Sending vfoA %s.", err, vfoA)
		vfoB = vfoA
	}
	if data.FreqVFOB, err = strconv.ParseFloat(vfoB, 64); err != nil {
		log.Errorf("Failed to parse vfoB frequency %s: %s", vfoB, err)
		return RigData{}, err
	}

	if err := client.Call("rig.get_modeB", nil, &data.ModeB); err != nil {
		log.Debugf("call failed to rig.get_modeB (flrig): %v. Sending ModeA.", err)
		data.ModeB = data.Mode
	}

	log.Debugf("Got data %#v", data)
	return data, nil
}

// Hamlib support is UNTESTED and was partially confabulated ("hallucinated") by Gemini, so it
// is very unlikely to actually work. Please report errors in order to fix it.

func (h *HamlibClient) GetData() (RigData, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", h.Host, h.Port))
	if err != nil {
		return RigData{}, fmt.Errorf("hamlib connection error: %w", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	data := RigData{}

	// Query Frequency (VFO A)
	if _, err := fmt.Fprintf(conn, "f\n"); err != nil {
		return RigData{}, fmt.Errorf("failed to send 'f' command to hamlib: %w", err)
	}
	freqStr, _, err := reader.ReadLine()
	if err != nil {
		return RigData{}, fmt.Errorf("failed to read frequency response from hamlib: %w", err)
	}
	data.FreqVFOA, err = strconv.ParseFloat(string(freqStr), 64)
	if err != nil {
		return RigData{}, fmt.Errorf("failed to parse frequency '%s': %w", freqStr, err)
	}

	// Query Mode (TX/RX mode is assumed to be the same, and no separate RX mode is readily available)
	if _, err := fmt.Fprintf(conn, "m\n"); err != nil {
		return RigData{}, fmt.Errorf("failed to send 'm' command to hamlib: %w", err)
	}
	modeResp, _, err := reader.ReadLine() // e.g., "USB 2400"
	if err != nil {
		return RigData{}, fmt.Errorf("failed to read mode response from hamlib: %w", err)
	}
	modeParts := strings.Fields(string(modeResp))
	if len(modeParts) > 0 {
		data.Mode = modeParts[0]
		data.ModeB = modeParts[0] // Default modeB to Mode/RX for simplicity
	} else {
		return RigData{}, fmt.Errorf("invalid mode response format from hamlib: '%s'", modeResp)
	}

	// Query Power (P)
	if _, err := fmt.Fprintf(conn, "P\n"); err != nil {
		log.Warnf("Failed to send 'P' (power) command to hamlib: %v. Sending 0 W.", err)
		data.Power = 0.0
	} else {
		powerStr, _, err := reader.ReadLine()
		if err != nil {
			log.Warnf("Failed to read power response from hamlib: %v. Sending 0 W.", err)
			data.Power = 0.0
		} else {
			// Hamlib returns 0-100 float percentage
			powerPercent, err := strconv.ParseFloat(string(powerStr), 64)
			if err != nil {
				log.Warnf("Failed to parse power '%s': %v. Sending 0 W.", powerStr, err)
				data.Power = 0.0
			} else {
				// Convert percentage to 100W max for simple display (Wavelog typically expects watts)
				data.Power = powerPercent
			}
		}
	}

	// WaveLogGate doesn't try either
	data.Split = 0
	data.FreqVFOB = data.FreqVFOA

	return data, nil
}

func postToWavelog(config ProfileConfig, data RigData) error {
	payload := WavelogJSONRequest{
		Key:       config.WavelogKey,
		Radio:     config.RadioName,
		Power:     data.Power,
		Frequency: int(data.FreqVFOA),
		Mode:      data.Mode,
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
	url := config.WavelogURL + "/api/radio"
	log.Infof("Sending to %s: %s", url, string(jsonPayload))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("wavelog API returned non-200 status code: %d. Body: %s", resp.StatusCode, string(body))
	}

	return nil
}

func main() {
	defaultConfig := ProfileConfig{
		WavelogURL: "http://localhost/index.php",
		WavelogKey: "YOUR_API_KEY",
		RadioName:  "RIG",
		FlrigHost:  "127.0.0.1",
		FlrigPort:  12345,
		HamlibHost: "127.0.0.1",
		HamlibPort: 4532,
		Interval:   "1s",
		DataSource: "flrig",
		LogLevel:   "error",
	}

	var currentProfileName string
	var saveProfileName string
	var setDefaultProfileName string

	showVersion := flag.Bool("version", false, "Print version information and exit")

	flag.StringVar(&currentProfileName, "profile", "", "Select a named configuration profile to run (overrides default).")
	flag.StringVar(&saveProfileName, "save-profile", "", "Saves the current configuration flags (excluding this flag) to the specified profile name and exits.")
	flag.StringVar(&setDefaultProfileName, "set-default-profile", "", "Sets the default profile to the specified name and exits.")

	wavelogURL := flag.String("wavelog-url", defaultConfig.WavelogURL, "Wavelog API URL for radio status.")
	wavelogKey := flag.String("wavelog-key", defaultConfig.WavelogKey, "Wavelog API Key.")
	radioName := flag.String("radio-name", defaultConfig.RadioName, "Name of the radio (e.g., FT-891).")
	flrigHost := flag.String("flrig-host", defaultConfig.FlrigHost, "flrig XML-RPC host address.")
	flrigPort := flag.Int("flrig-port", defaultConfig.FlrigPort, "flrig XML-RPC port.")
	hamlibHost := flag.String("hamlib-host", defaultConfig.HamlibHost, "Hamlib rigctld host address.")
	hamlibPort := flag.Int("hamlib-port", defaultConfig.HamlibPort, "Hamlib rigctld port.")
	interval := flag.String("interval", defaultConfig.Interval, "Polling interval (e.g., 1s, 1500ms).")
	dataSource := flag.String("data-source", defaultConfig.DataSource, "Data source: 'flrig' or 'hamlib'.")
	logLevel := flag.String("log-level", defaultConfig.LogLevel, "Logging level: 'debug', 'info', 'warn', or 'error'.")

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
		case "radio-name":
			currentProfileConfig.RadioName = *radioName
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
			log.Fatalf("Fatal: The --save-profile flag requires a profile name.")
		}
		cfgFile.Profiles[saveProfileName] = currentProfileConfig
		if err := saveConfig(configPath, cfgFile); err != nil {
			log.Fatalf("Fatal: Failed to save configuration file: %v", err)
		}
		fmt.Printf("Configuration saved successfully to profile '%s' in %s\n", saveProfileName, configPath)
		return
	}

	setupLogging(currentProfileConfig.LogLevel)

	if currentProfileConfig.WavelogKey == "" || currentProfileConfig.WavelogKey == defaultConfig.WavelogKey {
		log.Fatalf("Fatal: Wavelog API key is required. Please set via --wavelog-key or in the config file.")
	}
	if currentProfileConfig.WavelogURL == "" {
		log.Fatalf("Fatal: Wavelog URL is required.")
	}

	var client RadioClient
	switch strings.ToLower(currentProfileConfig.DataSource) {
	case "flrig":
		client = &FlrigClient{Host: currentProfileConfig.FlrigHost, Port: currentProfileConfig.FlrigPort}
		log.Infof("Using flrig client at %s:%d (Profile: %s)", currentProfileConfig.FlrigHost, currentProfileConfig.FlrigPort, profileToUse)
	case "hamlib":
		client = &HamlibClient{Host: currentProfileConfig.HamlibHost, Port: currentProfileConfig.HamlibPort}
		log.Infof("Using Hamlib client at %s:%d (Profile: %s)", currentProfileConfig.HamlibHost, currentProfileConfig.HamlibPort, profileToUse)
		log.Warnf("Hamlib support is untested and presumed broken. Please report success or failure to debug or remove this message!")
	default:
		log.Fatalf("Fatal: Invalid data source specified: '%s'. Must be 'flrig' or 'hamlib'.", currentProfileConfig.DataSource)
	}

	intervalDuration, err := time.ParseDuration(currentProfileConfig.Interval)
	if err != nil {
		log.Fatalf("Fatal: Invalid interval duration format: %v", err)
	}

	var lastData RigData
	log.Infof("Starting WaveLogGoat polling every %s...", intervalDuration)

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

		if currentData == lastData {
			log.Debug("Radio data unchanged. Skipping update.")
			continue
		}

		log.Infof("Radio state changed; freq: %.0f Hz, mode: %s). Updating Wavelog...", currentData.FreqVFOA, currentData.Mode)

		if err := postToWavelog(currentProfileConfig, currentData); err != nil {
			log.Errorf("Error posting to Wavelog: %v", err)
			continue
		}

		lastData = currentData
		log.Debug("Successfully updated Wavelog.")
	}
}
