# WaveLogGoat

`WaveLogGoat` is a lightweight Go application that polls radio status from `flrig` (via XML-RPC) or `hamlib`'s `rigctld` (via TCP) and sends it to your [Wavelog](https://github.com/wavelog/wavelog) instance.

This tool replaces the JavaScript-based `WaveLogGate` with a single, statically compiled binary that runs as a background service with no runtime dependencies. It supports multiple configuration profiles for different radios or Wavelog instances.

## Features

- **Dual Data Source:** Supports both `flrig` and `hamlib` (`rigctld`).
    - The `flrig` support is tested and known to function with flrig running on Fedora and an IC-7300
    - hamlib/rigctld is reported working by mattmelling
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
- **Real-time Radio Status Broadcasting:**
    - **WebSocket Server (WS):** Broadcasts radio status changes to connected clients in real-time.
    - **WebSocket Secure Server (WSS):** Encrypted WebSocket connections for secure communication.
    - Compatible with WaveLogGate client format.
- **HTTP QSY Server:**
    - Remote frequency/mode control via HTTP API.
    - Supports both HTTP and HTTPS (dual listener on same port).
- **SSL/TLS Support:**
    - Automatic self-signed certificate generation for localhost.
    - Platform-specific certificate installation instructions.
    - Secure connections for WSS and HTTPS QSY endpoints.

## How to Use

### Downloading

Download the standalone binary for your operating system and CPU type, and if necessary make it executable:

```sh
chmod +x waveloggoat
```

### Building from Source

You must have the [Go](https://go.dev/doc/install) toolchain (version 1.21+) installed.

It is recommended to have [GoReleaser installed](https://goreleaser.com/install/)

```sh
# git clone https://github.com/johnsonm/WaveLogGoat
# cd WaveLogGoat

# Download dependencies
go mod tidy

# Build the binary with goreleaser
goreleaser build

# Or build the binary with go
go build
```

### 2. Configuration

`WaveLogGoat` is configured using a `config.json` file, command-line flags, or a combination of both. Flags will always override settings from the config file.

**Configuration File Location:**
- **Linux:** `~/.config/WaveLogGoat/config.json`
- **Windows:** `%APPDATA%\WaveLogGoat\config.json`
- **macOS:** `~/Library/Application Support/WaveLogGoat/config.json`

**Certificate Location (for SSL/TLS):**
- **Linux:** `~/.config/WaveLogGoat/waveloggoat.crt` and `waveloggoat.key`
- **Windows:** `%APPDATA%\WaveLogGoat\waveloggoat.crt` and `waveloggoat.key`
- **macOS:** `~/Library/Application Support/WaveLogGoat/waveloggoat.crt` and `waveloggoat.key`

#### Creating Your First Profile

The easiest way to get started is by using command-line flags to create and save your first profile.

```sh
# Example: Create a profile named "IC-7300" using flrig
./waveloggoat \
    -save-profile="IC-7300" \
    -wavelog-url="https://mywavelog.com/index.php" \
    -wavelog-key="wl2_MY-API-KEY" \
    -radio-name="IC-7300" \
    -data-source="flrig" \
    -flrig-host="127.0.0.1" \
    -flrig-port=12345 \
    -websocket-enable \
    -websocket-port=54322 \
    -qsy-enable \
    -qsy-port=54321
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

### Command-Line Options
```
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
  -qsy-enable
    	Enable QSY HTTP server.
  -qsy-enable-ssl
    	Enable HTTPS for QSY server (dual HTTP/HTTPS on same port).
  -qsy-port int
    	QSY HTTP server port (default: 54321). (default 54321)
  -radio-name string
    	Name of the radio (e.g., FT-891). (default "RIG")
  -save-profile string
    	Saves the current configuration flags (excluding this flag) to the specified profile name and exits.
  -set-default-profile string
    	Sets the default profile to the specified name and exits.
  -version
    	Print version information and exit
  -wavelog-key wl2_
    	Wavelog API Key, starting with wl2_. (default "wl2_YOUR_API_KEY")
  -wavelog-url string
    	Wavelog API URL for radio status. (default "http://localhost/index.php")
  -websocket-enable
    	Enable WebSocket server for real-time radio status. (default true)
  -websocket-port int
    	WebSocket server port (default: 54322). (default 54322)
  -wss-enable
    	Enable WebSocket Secure server (WSS) for encrypted connections.
  -wss-port int
    	WebSocket Secure server port (default: 54323). (default 54323)
```

### Wavelog API Format

This tool sends data to Wavelog using the new JSON format:

- **Endpoint:** `(your-wavelog-url)/api/v2/radio` (The `/api/v2/radio` path is added automatically; the old /api/radio API is not supported)
- **Method:** `POST`
- **Header:** `Authorization: Bearer wl2_YOUR_API_KEY`
- **Body (JSON):**
  ```json
  {
    "radio": "IC-7300",
    "power": 100,
    "frequency": 14074000,
    "mode": "DATA",
    "frequency_rx": 14076000, // Optional: Only sent when split
    "mode_rx": "DATA" // Optional: Only sent when split
  }
```

## WebSocket/WSS Server

WaveLogGoat includes a WebSocket server that broadcasts real-time radio status changes to connected clients. This is c
mpatible with the WaveLogGate client format.

### WebSocket (WS) - Default Port 54322

```javascript
// Example JavaScript client
const socket = new WebSocket('ws://localhost:54322/');
socket.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.type === 'radio_status') {
        console.log(`Frequency: ${data.frequency} Hz, Mode: ${data.mode}`);
    }
};
```

### WebSocket Secure (WSS) - Default Port 54323

When SSL is enabled, WaveLogGoat generates a self-signed certificate and starts a WSS server:

```sh
./waveloggoat -wss-enable
```

```javascript
// Example WSS client
const socket = new WebSocket('wss://localhost:54323/');
socket.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.type === 'radio_status') {
        console.log(`Frequency: ${data.frequency} Hz, Mode: ${data.mode}`);
    }
};
```

### Message Format

Radio status messages are sent in the following JSON format:

```json
{
  "type": "radio_status",
  "frequency": 14075000,
  "mode": "USB",
  "power": 100,
  "split": 0,
  "radio": "IC-7300",
  "timestamp": 1704067200000
}
```

For split mode operation:
```json
{
  "type": "radio_status",
  "frequency": 14075000,
  "frequency_rx": 14074000,
  "mode": "USB",
  "mode_rx": "USB",
  "power": 100,
  "split": 1,
  "radio": "IC-7300",
  "timestamp": 1704067200000
}
```

## HTTP QSY Server

WaveLogGoat includes an HTTP API for remote frequency and mode control (QSY). This allows WaveLog or other applications to change the radio frequency and mode.

### QSY Endpoint

- **HTTP:** `http://localhost:54321/{frequency}/{mode}`
- **HTTPS:** `https://localhost:54321/{frequency}/{mode}` (when `-qsy-enable-ssl` is set)

### Example Usage

```bash
# Set radio to 7.155 MHz LSB
curl http://localhost:54321/7155000/LSB

# Using HTTPS (requires SSL certificate to be installed)
curl -k https://localhost:54321/7155000/LSB
```

### Response Format

Success response:
```json
{
  "status": "success",
  "message": "QSY successful: frequency=7155000 Hz, mode=LSB",
  "frequency": 7155000,
  "mode": "LSB"
}
```

Error response (radio control software not available):
```json
{
  "error": "QSY failed: radio control software not available",
  "details": "connection refused"
}
```

### Supported Modes

USB, LSB, CW, AM, FM, PKTUSB, PKTLSB, RTTY, CWR, PKTFM, DIGI, DIGU, DIGL


## SSL/TLS Support

WaveLogGoat includes automatic SSL/TLS certificate generation for secure connections.

### Certificate Generation

When you enable SSL features (`-wss-enable` or `-qsy-enable-ssl`), WaveLogGoat automatically:

1. Generates a self-signed ECDSA P-256 certificate for localhost
2. Saves the certificate and private key to the config directory
3. Displays platform-specific installation instructions

The certificate is valid for 10 years and includes the following Subject Alternative Names:
- `localhost`
- `127.0.0.1`
- `::1`

### Certificate Installation

For browsers to trust the self-signed certificate, it must be installed in your system's certificate trust store.

#### macOS

```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ~/Library/Application\ Support/WaveLogGoat/waveloggoat.crt
```

**Alternative (via GUI):**
1. Open Keychain Access (Applications > Utilities > Keychain Access)
2. Drag the certificate file into the 'System' keychain
3. Find the certificate, double-click it, and expand 'Trust'
4. Set 'When using this certificate' to 'Always Trust'
5. Close the dialog and enter your password if prompted

#### Windows

```cmd
certutil -addstore -f Root %APPDATA%\WaveLogGoat\waveloggoat.crt
```

**Alternative (via GUI):**
1. Double-click the certificate file
2. Click 'Install Certificate'
3. Select 'Local Machine' > Next
4. Select 'Place all certificates in the following store'
5. Click 'Browse' and select 'Trusted Root Certification Authorities'
6. Click Finish

#### Linux

**Debian/Ubuntu:**
```bash
sudo cp ~/.config/WaveLogGoat/waveloggoat.crt /usr/local/share/ca-certificates/waveloggoat.crt
sudo update-ca-certificates
```

**Fedora/RHEL/CentOS:**
```bash
sudo cp ~/.config/WaveLogGoat/waveloggoat.crt /etc/pki/ca-trust/source/anchors/waveloggoat.crt
sudo update-ca-trust
```

**Arch Linux:**
```bash
sudo trust anchor ~/.config/WaveLogGoat/waveloggoat.crt
```

**Note:** Some browsers (like Firefox) manage their own certificate store. You may need to import the certificate directly in your browser settings.

### Enabling SSL Features

```bash
# Enable WebSocket Secure only
./waveloggoat -wss-enable

# Enable HTTPS for QSY server
./waveloggoat -qsy-enable-ssl

# Enable both
./waveloggoat -wss-enable -qsy-enable-ssl

# Save SSL settings to profile
./waveloggoat -save-profile="ssl-profile" -wss-enable -qsy-enable-ssl
```

## Network Ports

- **12345:** Default flrig XML-RPC port
- **4532:** Default hamlib rigctld port
- **54321:** HTTP/HTTPS QSY server port
- **54322:** WebSocket (WS) server port
- **54323:** WebSocket Secure (WSS) server port

## License

MIT License - See LICENSE file for details.
