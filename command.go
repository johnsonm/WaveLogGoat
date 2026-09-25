package main

import (
	"flag"
)

// CLIConfig holds pointers to all command-line flag values.
type CLIConfig struct {
	Profile           *string
	SaveProfile       *string
	SetDefaultProfile *string
	ShowVersion       *bool
	WavelogURL        *string
	WavelogKey        *string
	WavelogKeyV1      *string
	RadioName         *string
	MaxPower          *float64
	FlrigHost         *string
	FlrigPort         *int
	HamlibHost        *string
	HamlibPort        *int
	Interval          *string
	DataSource        *string
	LogLevel          *string
	WebSocketEnable   *bool
	WebSocketPort     *int
	WSSEnable         *bool
	WSSPort           *int
	QSYEnable         *bool
	QSYPort           *int
	QSYEnableSSL      *bool
}

// ParseFlags registers and parses all CLI flags.
func ParseFlags(args []string, defaultConfig ProfileConfig) (*CLIConfig, *flag.FlagSet, error) {
	fs := flag.NewFlagSet("WaveLogGoat", flag.ExitOnError)
	cfg := &CLIConfig{}

	cfg.Profile = fs.String("profile", "", "Select a named configuration profile to run (overrides default).")
	cfg.SaveProfile = fs.String("save-profile", "", "Saves the current configuration flags (excluding this flag) to the specified profile name and exits.")
	cfg.SetDefaultProfile = fs.String("set-default-profile", "", "Sets the default profile to the specified name and exits.")
	cfg.ShowVersion = fs.Bool("version", false, "Print version information and exit")

	cfg.WavelogURL = fs.String("wavelog-url", defaultConfig.WavelogURL, "Wavelog API URL for radio status.")
	cfg.WavelogKey = fs.String("wavelog-key", defaultConfig.WavelogKey, "Wavelog API Key, starting with `wl2_`.")
	cfg.WavelogKeyV1 = fs.String("wavelog-key-v1", defaultConfig.WavelogKeyV1, "Wavelog V1 API Key.")
	cfg.RadioName = fs.String("radio-name", defaultConfig.RadioName, "Name of the radio (e.g., FT-891).")
	cfg.MaxPower = fs.Float64("max-power", defaultConfig.MaxPower, "Maximum RF power in watts (default 100).")
	cfg.FlrigHost = fs.String("flrig-host", defaultConfig.FlrigHost, "flrig XML-RPC host address.")
	cfg.FlrigPort = fs.Int("flrig-port", defaultConfig.FlrigPort, "flrig XML-RPC port.")
	cfg.HamlibHost = fs.String("hamlib-host", defaultConfig.HamlibHost, "Hamlib rigctld host address.")
	cfg.HamlibPort = fs.Int("hamlib-port", defaultConfig.HamlibPort, "Hamlib rigctld port.")
	cfg.Interval = fs.String("interval", defaultConfig.Interval, "Polling interval (e.g., 1s, 1500ms).")
	cfg.DataSource = fs.String("data-source", defaultConfig.DataSource, "Data source: 'flrig' or 'hamlib'.")
	cfg.LogLevel = fs.String("log-level", defaultConfig.LogLevel, "Logging level: 'debug', 'info', 'warn', or 'error'.")
	cfg.WebSocketEnable = fs.Bool("websocket-enable", defaultConfig.WebSocketEnable, "Enable WebSocket server for real-time radio status.")
	cfg.WebSocketPort = fs.Int("websocket-port", defaultConfig.WebSocketPort, "WebSocket server port (default: 54322).")
	cfg.WSSEnable = fs.Bool("wss-enable", defaultConfig.WSSEnable, "Enable WebSocket Secure server (WSS) for encrypted connections.")
	cfg.WSSPort = fs.Int("wss-port", defaultConfig.WSSPort, "WebSocket Secure server port (default: 54323).")
	cfg.QSYEnable = fs.Bool("qsy-enable", defaultConfig.QSYEnable, "Enable QSY HTTP server.")
	cfg.QSYPort = fs.Int("qsy-port", defaultConfig.QSYPort, "QSY HTTP server port (default: 54321).")
	cfg.QSYEnableSSL = fs.Bool("qsy-enable-ssl", defaultConfig.QSYEnableSSL, "Enable HTTPS for QSY server (dual HTTP/HTTPS on same port).")

	err := fs.Parse(args)
	return cfg, fs, err
}
