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

// Credentials may be supplied from the environment (e.g. systemd
// EnvironmentFile) so real secrets live outside Git. Env overrides the YAML.
func TestLoadConfig_AuthEnvOverridesYAML(t *testing.T) {
	path := writeConfig(t, "auth:\n  enabled: true\n  username: \"admin\"\n  password: \"yaml-dev\"\n")
	t.Setenv("ORCHESTRATOR_AUTH_USERNAME", "stefenello")
	t.Setenv("ORCHESTRATOR_AUTH_PASSWORD", "env-secret")

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "stefenello", cfg.Auth.Username)
	assert.Equal(t, "env-secret", cfg.Auth.Password)
}

func TestLoadConfig_AuthYAMLUsedWhenEnvAbsent(t *testing.T) {
	path := writeConfig(t, "auth:\n  enabled: true\n  username: \"admin\"\n  password: \"changeme\"\n")

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "admin", cfg.Auth.Username)
	assert.Equal(t, "changeme", cfg.Auth.Password)
}

func TestLoadConfig_EmptyEnvDoesNotOverride(t *testing.T) {
	path := writeConfig(t, "auth:\n  username: \"admin\"\n  password: \"changeme\"\n")
	t.Setenv("ORCHESTRATOR_AUTH_USERNAME", "")
	t.Setenv("ORCHESTRATOR_AUTH_PASSWORD", "")

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "admin", cfg.Auth.Username, "empty env must not blank the YAML value")
	assert.Equal(t, "changeme", cfg.Auth.Password)
}

func TestLoadConfig_AuthEnabledEnvOverride(t *testing.T) {
	path := writeConfig(t, "auth:\n  enabled: true\n  username: \"admin\"\n")
	t.Setenv("ORCHESTRATOR_AUTH_ENABLED", "false")

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.False(t, cfg.Auth.Enabled)
}
