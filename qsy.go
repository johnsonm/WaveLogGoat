package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type QSYServer struct {
	client RadioClient
	port   int
}

// qsyHandler creates the QSY HTTP handler function
func qsyHandler(client RadioClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight OPTIONS requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Path: /{freq} or /{freq}/{mode}
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		if parts[0] == "" {
			http.Error(w, "Frequency required", http.StatusBadRequest)
			return
		}

		hz, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "Invalid frequency", http.StatusBadRequest)
			return
		}
		mode := ""
		if len(parts) > 1 {
			mode = parts[1]
		}

		// Set frequency and mode
		if err := client.SetData(float64(hz), strings.ToUpper(mode)); err != nil {
			log.Warnf("QSY failed - radio control software may not be running: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			response := map[string]string{
				"error":   "QSY failed: radio control software not available",
				"details": err.Error(),
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		log.Infof("QSY successful: frequency=%d Hz, mode=%s", hz, strings.ToUpper(mode))

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"status":    "success",
			"message":   fmt.Sprintf("QSY successful: frequency=%d Hz, mode=%s", hz, strings.ToUpper(mode)),
			"frequency": hz,
			"mode":      strings.ToUpper(mode),
		}
		json.NewEncoder(w).Encode(response)
	}
}

func startQSYServer(client RadioClient, port int, enableSSL bool, certPath, keyPath string) (*http.Server, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", qsyHandler(client))

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}

	log.Infof("Starting QSY HTTP server on port %d", port)
	log.Infof("QSY endpoint: http://localhost:%d/{frequency}/{mode}", port)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("QSY HTTP server error: %v", err)
		}
	}()

	// Start HTTPS server if SSL is enabled
	if enableSSL {
		httpsServer := &http.Server{
			Addr:    ":" + strconv.Itoa(port),
			Handler: mux,
		}

		log.Infof("Starting QSY HTTPS server on port %d", port)
		log.Infof("QSY HTTPS endpoint: https://localhost:%d/{frequency}/{mode}", port)
		log.Infof("Example: curl -k https://localhost:%d/7155000/LSB", port)

		go func() {
			if err := httpsServer.ListenAndServeTLS(certPath, keyPath); err != nil && err != http.ErrServerClosed {
				log.Errorf("QSY HTTPS server error: %v", err)
			}
		}()
	}

	return httpServer, nil
}
