package main

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
)

// implements RadioClient for TCP communication with rigctld / hamlib
type HamlibClient struct {
	ctx      context.Context
	Host     string
	Port     int
	MaxPower float64
	dialer   ConnectionProvider // Injected dialer
}

// NewHamlibClient creates a new HamlibClient with the given ConnectionProvider.
func NewHamlibClient(ctx context.Context, host string, port int, maxPower float64, dialer ConnectionProvider) *HamlibClient {
	return &HamlibClient{
		ctx:      ctx,
		Host:     host,
		Port:     port,
		MaxPower: maxPower,
		dialer:   dialer,
	}
}

func (h *HamlibClient) SetData(freq float64, mode string) error {
	conn, err := h.dialer.Dial(h.ctx, "tcp", fmt.Sprintf("%s:%d", h.Host, h.Port))
	if err != nil {
		return fmt.Errorf("hamlib connection error: %w", err)
	}
	defer conn.Close()

	if freq > 0 {
		if _, err := fmt.Fprintf(conn, "F %.0f\n", freq); err != nil {
			return fmt.Errorf("failed to send 'F' command to hamlib: %w", err)
		}
	}
	if mode != "" {
		// Hamlib mode command: M <mode> <passband>.
		// Passband is optional, using 0 for default.
		if _, err := fmt.Fprintf(conn, "M %s 0\n", mode); err != nil {
			return fmt.Errorf("failed to send 'M' command to hamlib: %w", err)
		}
	}
	return nil
}

// readReply reads exactly n lines from a rigctld connection. Each rigctld command
// returns a fixed number of response lines; reading fewer leaves stale data in the
// bufio buffer that pollutes the next read.
func readReply(reader *bufio.Reader, n int) ([]string, error) {
	lines := make([]string, 0, n)
	for range n {
		line, _, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		lines = append(lines, string(line))
	}
	return lines, nil
}

func (h *HamlibClient) GetData() (RigData, error) {
	conn, err := h.dialer.Dial(h.ctx, "tcp", fmt.Sprintf("%s:%d", h.Host, h.Port))
	if err != nil {
		return RigData{}, fmt.Errorf("hamlib connection error: %w", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	data := RigData{}

	// Query Frequency (VFO A) — 1 line
	if _, err := fmt.Fprintf(conn, "f\n"); err != nil {
		return RigData{}, fmt.Errorf("failed to send 'f' command to hamlib: %w", err)
	}
	freqLines, err := readReply(reader, 1)
	if err != nil {
		return RigData{}, fmt.Errorf("failed to read frequency response from hamlib: %w", err)
	}
	data.FreqVFOA, err = strconv.ParseFloat(freqLines[0], 64)
	if err != nil {
		return RigData{}, fmt.Errorf("failed to parse frequency '%s': %w", freqLines[0], err)
	}

	// Query Mode (TX/RX mode is assumed to be the same, and no separate RX mode is readily available)
	// Returns Mode and Passband (2 lines)
	if _, err := fmt.Fprintf(conn, "m\n"); err != nil {
		return RigData{}, fmt.Errorf("failed to send 'm' command to hamlib: %w", err)
	}
	modeLines, err := readReply(reader, 2)
	if err != nil {
		return RigData{}, fmt.Errorf("failed to read mode response from hamlib: %w", err)
	}
	data.Mode = modeLines[0]
	data.ModeB = modeLines[0]

	// Query Power (P) — 1 line
	if _, err := fmt.Fprintf(conn, "l RFPOWER\n"); err != nil {
		log.Warnf("Failed to send 'l RFPOWER' (power) command to hamlib: %v.", err)
		data.Power = 0.0
		data.PowerValid = false
	} else {
		powerLines, err := readReply(reader, 1)
		if err != nil {
			log.Warnf("Failed to read power response from hamlib: %v.", err)
			data.Power = 0.0
			data.PowerValid = false
		} else {
			powerPercent, err := strconv.ParseFloat(powerLines[0], 64)
			if err != nil {
				log.Warnf("Failed to parse power '%s': %v.", powerLines[0], err)
				data.Power = 0.0
				data.PowerValid = false
			} else {
				if h.MaxPower > 0 {
					data.Power = powerPercent // Already in watts
				} else {
					data.Power = powerPercent * 100 // Will be normalized by ProfileConfig.MaxPower on send
				}
				data.PowerValid = true
			}
		}
	}

	// WaveLogGate doesn't try either
	data.Split = 0
	data.FreqVFOB = data.FreqVFOA

	return data, nil
}
