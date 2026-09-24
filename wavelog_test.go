package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchWavelogVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/version", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var payload map[string]string
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)
		assert.Equal(t, "v1key", payload["key"])

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"version": "3.2.0"}`))
	}))
	defer ts.Close()

	config := ProfileConfig{
		WavelogURL:   ts.URL,
		WavelogKeyV1: "v1key",
	}

	version, err := fetchWavelogVersion(config)
	assert.NoError(t, err)
	assert.Equal(t, "3.2.0", version)
}

func TestPostToWavelog(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/radio", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer v2key", r.Header.Get("Authorization"))

		var payload WavelogJSONRequest
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)
		assert.Equal(t, "RIG", payload.Radio)
		assert.Equal(t, 14074000, payload.Frequency)
		assert.Equal(t, "USB", payload.Mode)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := ProfileConfig{
		WavelogURL: ts.URL,
		WavelogKey: "v2key",
		RadioName:  "RIG",
	}

	data := RigData{
		FreqVFOA: 14074000,
		Mode:     "USB",
	}

	err := postToWavelog(config, data, "3.2.0")
	assert.NoError(t, err)
}
