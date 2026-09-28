package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadConfig_AppliesDefaults(t *testing.T) {
	path := writeConfig(t, "server:\n  port: 9999\n")

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, "opencode", cfg.OpenCode.BinaryPath)
	assert.Equal(t, 1, cfg.Concurrency.MaxActivePerProject)
	assert.Equal(t, PhaseAgentDiscoveryDefault, cfg.OpenCode.Agents.Discovery)
}

func TestLoadConfig_MissingFileReturnsError(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	assert.Error(t, err)
}
