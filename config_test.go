package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigLoadSave(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	cfg := ConfigFile{
		DefaultProfile: "default",
		Profiles: map[string]ProfileConfig{
			"default": GetDefaultConfig(),
		},
	}

	// Test Save
	err := saveConfig(configPath, cfg)
	assert.NoError(t, err)

	// Test Load
	loadedCfg, err := loadConfig(configPath)
	assert.NoError(t, err)
	assert.Equal(t, cfg, loadedCfg)
}

func TestGetDefaultConfig(t *testing.T) {
	cfg := GetDefaultConfig()
	assert.Equal(t, "flrig", cfg.DataSource)
	assert.Equal(t, 100.0, cfg.MaxPower)
	assert.Equal(t, "127.0.0.1", cfg.FlrigHost)
	assert.Equal(t, "127.0.0.1", cfg.HamlibHost)
	assert.True(t, cfg.WebSocketEnable)
}
