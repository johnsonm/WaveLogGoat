# WaveLogGoat

`WaveLogGoat` is a lightweight Go application that polls radio status from `flrig` (via XML-RPC) or `hamlib`'s `rigctld` (via TCP) and sends it to your [Wavelog](https://github.com/wavelog/wavelog) instance.

This tool replaces the JavaScript-based `WaveLogGate` with a single, statically compiled binary that runs as a background service with no runtime dependencies. It supports multiple configuration profiles for different radios or Wavelog instances.

## Features

- **Dual Data Source:** Supports both `flrig` and `hamlib` (`rigctld`).
    - The `flrig` support is tested and known to function with flrig running on Fedora and an IC-7300
    - **Warning:** hamlib/rigctld was confabulated by an LLM and may be functional or fictional. Please report either success or failure.
- **Configuration Profiles:** Manage multiple radio/Wavelog setups within a single `config.json` file.
- **Command-Line Control:** All configuration options can be set via command-line flags, which override file settings.
- **Easy Configuration:** Persist your settings using the `-save-profile` flag.
- **Cross-Platform:** Runs on Linux, Windows, and macOS.
    - Tested on Fedora Linux on amd64
    - Intended to work on macOS and Windows. Please report success or failure.
- **Lightweight:** Low CPU and memory usage, ideal for a Raspberry Pi, laptop, or generally conserving resources.
- **Leveled Logging:**
    - `-log-level=error` (default): Only shows fatal errors.
    - `-log-level=warn`: Shows non-critical errors (e.g., failed to get one data point).
    - `-log-level=info`: Shows successful updates and configuration loading.
    - `-log-level=debug`: Shows connection errors and unchanged data polls.

## How to Use

### Downloading

Download the standalone binary for your operating system and CPU type, and if necessary make it executable:

```sh
chmod +x waveloggoat
```

### Building from Source

You must have the [Go](https://go.dev/doc/install) toolchain (version 1.21+) installed.

```sh
# git clone https://github.com/johnsonm/WaveLogGoat
# cd WaveLogGoat

# Download dependencies
go mod tidy

# Build the binary
go build
```

### 2. Configuration

`WaveLogGoat` is configured using a `config.json` file, command-line flags, or a combination of both. Flags will always override settings from the config file.

**Configuration File Location:**
- **Linux:** `~/.config/WaveLogGoat/config.json`
- **Windows:** `%APPDATA%\WaveLogGoat\config.json`
- **macOS:** `~/Library/Application Support/WaveLogGoat/config.json`

#### Creating Your First Profile

The easiest way to get started is by using command-line flags to create and save your first profile.

```sh
# Example: Create a profile named "IC-7300" using flrig
./waveloggoat \
    -save-profile="IC-7300" \
    -wavelog-url="https://mywavelog.com/index.php" \
    -wavelog-key="MY-API-KEY" \
    -radio-name="IC-7300" \
    -data-source="flrig" \
    -flrig-host="127.0.0.1" \
    -flrig-port=12345
```

This command creates the `config.json` file (if it doesn't exist) and saves these settings.

#### Setting the Default Profile

After saving a profile, you can set it as the default.

```sh
./waveloggoat -set-default-profile="IC-7300"
```

### 3. Running the Program

Once you have a default profile set, you can run the program with no arguments:

```sh
# This will load the settings for the default profile
./waveloggoat
```

To run with a *different* profile:

```sh
./waveloggoat -profile="My-Other-Radio"
```

To override one setting (like the log level) for a single run:

```sh
# Run the default profile, but with debug logging
./waveloggoat -log-level=debug
```

### Command-Line Flags

```sh
Usage of ./waveloggoat:
  -data-source string
    	Data source: 'flrig' or 'hamlib'. (default "flrig")
  -flrig-host string
    	flrig XML-RPC host address. (default "127.0.0.1")
  -flrig-port int
    	flrig XML-RPC port. (default 12345)
  -hamlib-host string
    	Hamlib rigctld host address. (default "127.0.0.1")
  -hamlib-port int
    	Hamlib rigctld port. (default 4532)
  -interval string
    	Polling interval (e.g., 1s, 1500ms). (default "1s")
  -log-level string
    	Logging level: 'debug', 'info', 'warn', or 'error'. (default "error")
  -profile string
    	Select a named configuration profile to run (overrides default).
  -radio-name string
    	Name of the radio (e.g., FT-891). (default "RIG")
  -save-profile string
    	Saves the current configuration flags (excluding this flag) to the specified profile name and exits.
  -set-default-profile string
    	Sets the default profile to the specified name and exits.
  -version
    	Print version information and exit
  -wavelog-key string
    	Wavelog API Key. (default "YOUR_API_KEY")
  -wavelog-url string
    	Wavelog API URL for radio status. (default "http://localhost/index.php")
```

### Wavelog API Format

This tool sends data to Wavelog using the new JSON format:

- **Endpoint:** `(your-wavelog-url)/api/radio` (The `/api/radio` path is added automatically)
- **Method:** `POST`
- **Body (JSON):**
  ```json
  {
    "key": "YOUR_API_KEY",
    "radio": "IC-7300",
    "power": 100,
    "frequency": 14074000,
    "mode": "DATA",
    "frequency_rx": 14076000, // Optional: Only sent when split
    "mode_rx": "DATA" // Optional: Only sent when split
  }
  ```
