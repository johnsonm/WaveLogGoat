package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WavelogJSONRequest matches the required JSON payload for the Wavelog API update.
type WavelogJSONRequest struct {
	Radio       string `json:"radio"`
	Power       any    `json:"power,omitempty"`
	Frequency   int    `json:"frequency"`
	Mode        string `json:"mode"`
	FrequencyRX int    `json:"frequency_rx,omitempty"`
	ModeRX      string `json:"mode_rx,omitempty"`
}

type WavelogErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
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
