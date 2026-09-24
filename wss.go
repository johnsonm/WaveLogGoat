package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

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

type WebSocketServer struct {
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	upgrader  websocket.Upgrader
	port      int
}

func broadcastToWavelog(server *WebSocketServer, message WebSocketMessage) {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Errorf("Failed to marshal WebSocket message: %v", err)
		return
	}

	server.clientsMu.RLock()
	defer server.clientsMu.RUnlock()

	for client := range server.clients {
		if err := client.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
			log.Errorf("Failed to send message to Wavelog: %v", err)
			client.Close()
			delete(server.clients, client)
		}
	}
	log.Debugf("Broadcasted radio status to Wavelog: freq=%d, mode=%s", message.Frequency, message.Mode)
}

func startWebSocketServer(port int) (*WebSocketServer, error) {
	server := &WebSocketServer{
		clients: make(map[*websocket.Conn]bool),
		port:    port,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Upgrade HTTP connection to WebSocket
		conn, err := server.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Debugf("WebSocket upgrade failed: %v", err)
			return
		}

		server.clientsMu.Lock()
		server.clients[conn] = true
		server.clientsMu.Unlock()

		log.Infof("WebSocket client connected")

		welcomeMsg := WebSocketMessage{
			Type:    "welcome",
			Message: "Connected to WaveLogGoat WebSocket server",
		}
		welcomeBytes, err := json.Marshal(welcomeMsg)
		if err != nil {
			log.Errorf("Failed to marshal welcome message: %v", err)
			conn.Close()
			server.clientsMu.Lock()
			delete(server.clients, conn)
			server.clientsMu.Unlock()
			return
		}

		if err := conn.WriteMessage(websocket.TextMessage, welcomeBytes); err != nil {
			log.Errorf("Failed to send welcome message: %v", err)
			conn.Close()
			server.clientsMu.Lock()
			delete(server.clients, conn)
			server.clientsMu.Unlock()
			return
		}

		defer func() {
			conn.Close()
			server.clientsMu.Lock()
			delete(server.clients, conn)
			server.clientsMu.Unlock()
			log.Infof("WebSocket client disconnected")
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Errorf("WebSocket error: %v", err)
				}
				break
			}
			// WaveLogGate doesn't process incoming messages, just ignore them
		}
	})

	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}

	log.Infof("Starting WebSocket server on port %d", port)
	log.Infof("WebSocket endpoint: ws://localhost:%d/", port)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("WebSocket server error: %v", err)
		}
	}()

	return server, nil
}

// startWSSServer starts a WebSocket Secure server (WSS)
func startWSSServer(port int, certPath, keyPath string) (*WebSocketServer, error) {
	server := &WebSocketServer{
		clients: make(map[*websocket.Conn]bool),
		port:    port,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Upgrade HTTP connection to WebSocket
		conn, err := server.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Debugf("WebSocket upgrade failed: %v", err)
			return
		}

		server.clientsMu.Lock()
		server.clients[conn] = true
		server.clientsMu.Unlock()

		log.Infof("WebSocket Secure client connected")

		welcomeMsg := WebSocketMessage{
			Type:    "welcome",
			Message: "Connected to WaveLogGoat WebSocket Secure server",
		}
		welcomeBytes, err := json.Marshal(welcomeMsg)
		if err != nil {
			log.Errorf("Failed to marshal welcome message: %v", err)
			conn.Close()
			server.clientsMu.Lock()
			delete(server.clients, conn)
			server.clientsMu.Unlock()
			return
		}

		if err := conn.WriteMessage(websocket.TextMessage, welcomeBytes); err != nil {
			log.Errorf("Failed to send welcome message: %v", err)
			conn.Close()
			server.clientsMu.Lock()
			delete(server.clients, conn)
			server.clientsMu.Unlock()
			return
		}

		defer func() {
			conn.Close()
			server.clientsMu.Lock()
			delete(server.clients, conn)
			server.clientsMu.Unlock()
			log.Infof("WebSocket Secure client disconnected")
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Errorf("WebSocket Secure error: %v", err)
				}
				break
			}
			// WaveLogGate doesn't process incoming messages, just ignore them
		}
	})

	httpsServer := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}

	log.Infof("Starting WebSocket Secure server on port %d", port)
	log.Infof("WSS endpoint: wss://localhost:%d/", port)

	go func() {
		if err := httpsServer.ListenAndServeTLS(certPath, keyPath); err != nil && err != http.ErrServerClosed {
			log.Errorf("WebSocket Secure server error: %v", err)
		}
	}()

	return server, nil
}

// getCertDir returns the directory where SSL certificates are stored
func getCertDir() (string, error) {
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
	return configDir, nil
}

// generateCertificate generates a self-signed SSL certificate for localhost
func generateCertificate() ([]byte, []byte, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"WaveLogGoat"},
			CommonName:   "127.0.0.1",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour * 10), // 10 years
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "127.0.0.1", "::1"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privKeyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privKeyBytes})

	return certPEM, privKeyPEM, nil
}

// loadOrCreateCertificate loads existing certificates or generates new ones
func loadOrCreateCertificate() (certPath, keyPath string, err error) {
	certDir, err := getCertDir()
	if err != nil {
		return "", "", err
	}

	certPath = filepath.Join(certDir, "waveloggoat.crt")
	keyPath = filepath.Join(certDir, "waveloggoat.key")

	// Check if certificate already exists
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			log.Debugf("Using existing SSL certificates from %s", certDir)
			return certPath, keyPath, nil
		}
	}

	log.Infof("Generating new SSL certificates in %s", certDir)

	certPEM, keyPEM, err := generateCertificate()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate certificate: %w", err)
	}

	// Write certificate
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return "", "", fmt.Errorf("failed to write certificate file: %w", err)
	}

	// Write private key
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return "", "", fmt.Errorf("failed to write key file: %w", err)
	}

	log.Infof("SSL certificates generated successfully")
	log.Infof("Certificate: %s", certPath)
	log.Infof("Private key: %s", keyPath)

	// Print installation instructions
	printCertInstallInstructions(certPath)

	return certPath, keyPath, nil
}

// printCertInstallInstructions prints platform-specific certificate installation instructions
func printCertInstallInstructions(certPath string) {
	log.Infof("")
	log.Info("=" + strings.Repeat("=", 70))
	log.Infof("SSL Certificate Installation Required")
	log.Info("=" + strings.Repeat("=", 70))
	log.Infof("")
	log.Infof("WaveLogGoat has generated a self-signed SSL certificate for HTTPS support.")
	log.Infof("For browsers to trust this certificate, it must be installed in your")
	log.Infof("system's certificate trust store.")
	log.Infof("")
	log.Infof("Certificate location: %s", certPath)
	log.Infof("")
	log.Infof("Platform-specific installation instructions:")
	log.Infof("")

	switch runtime.GOOS {
	case "darwin":
		log.Infof("macOS:")
		log.Infof("  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s", certPath)
		log.Infof("")
		log.Infof("Alternative (via GUI):")
		log.Infof("  1. Open Keychain Access (Applications > Utilities > Keychain Access)")
		log.Infof("  2. Drag the certificate file into the 'System' keychain")
		log.Infof("  3. Find the certificate, double-click it, and expand 'Trust'")
		log.Infof("  4. Set 'When using this certificate' to 'Always Trust'")
		log.Infof("  5. Close the dialog and enter your password if prompted")
	case "windows":
		log.Infof("Windows:")
		log.Infof("  certutil -addstore -f Root %s", certPath)
		log.Infof("")
		log.Infof("Alternative (via GUI):")
		log.Infof("  1. Double-click the certificate file")
		log.Infof("  2. Click 'Install Certificate'")
		log.Infof("  3. Select 'Local Machine' > Next")
		log.Infof("  4. Select 'Place all certificates in the following store'")
		log.Infof("  5. Click 'Browse' and select 'Trusted Root Certification Authorities'")
		log.Infof("  6. Click Finish")
	case "linux":
		log.Infof("Linux (varies by distribution):")
		log.Infof("")
		log.Infof("Debian/Ubuntu:")
		log.Infof("  sudo cp %s /usr/local/share/ca-certificates/waveloggoat.crt", certPath)
		log.Infof("  sudo update-ca-certificates")
		log.Infof("")
		log.Infof("Fedora/RHEL/CentOS:")
		log.Infof("  sudo cp %s /etc/pki/ca-trust/source/anchors/waveloggoat.crt", certPath)
		log.Infof("  sudo update-ca-trust")
		log.Infof("")
		log.Infof("Arch Linux:")
		log.Infof("  sudo trust anchor %s", certPath)
		log.Infof("")
		log.Infof("Alternative (via browser):")
		log.Infof("  Some browsers (like Firefox) manage their own certificate store.")
		log.Infof("  You may need to import the certificate directly in your browser settings.")
	default:
		log.Infof("See your operating system's documentation for installing CA certificates.")
	}

	log.Infof("")
	log.Infof("After installation, restart your browser for changes to take effect.")
	log.Infof("")
	log.Info("=" + strings.Repeat("=", 70))
	log.Infof("")
}
