package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFlags(t *testing.T) {
	defaultConfig := GetDefaultConfig()

	t.Run("defaults", func(t *testing.T) {
		cfg, _, err := ParseFlags([]string{}, defaultConfig)
		assert.NoError(t, err)
		assert.Equal(t, defaultConfig.WavelogURL, *cfg.WavelogURL)
		assert.Equal(t, defaultConfig.FlrigHost, *cfg.FlrigHost)
		assert.False(t, *cfg.ShowVersion)
	})

	t.Run("override", func(t *testing.T) {
		args := []string{"-wavelog-url", "http://test.com", "-flrig-host", "192.168.1.1"}
		cfg, _, err := ParseFlags(args, defaultConfig)
		assert.NoError(t, err)
		assert.Equal(t, "http://test.com", *cfg.WavelogURL)
		assert.Equal(t, "192.168.1.1", *cfg.FlrigHost)
	})
}
