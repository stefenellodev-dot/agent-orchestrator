package config

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	PhaseAgentDiscoveryDefault      = "explore"
	PhaseAgentDecisionDefault       = "plan"
	PhaseAgentImplementationDefault = "build"
	PhaseAgentValidationDefault     = "build"
)

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Store       StoreConfig       `mapstructure:"store"`
	Worktree    WorktreeConfig    `mapstructure:"worktree"`
	OpenCode    OpenCodeConfig    `mapstructure:"opencode"`
	Concurrency ConcurrencyConfig `mapstructure:"concurrency"`
	Auth        AuthConfig        `mapstructure:"auth"`
	Web         WebConfig         `mapstructure:"web"`
	Feature     FeatureConfig     `mapstructure:"feature"`
	Projects    []ProjectConfig   `mapstructure:"projects"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type DatabaseConfig struct {
	DSN             string `mapstructure:"dsn"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime string `mapstructure:"conn_max_lifetime"`
}

type StoreConfig struct {
	Kind string `mapstructure:"kind"`
}

type WorktreeConfig struct {
	RootPath         string `mapstructure:"root_path"`
	RetainDays       int    `mapstructure:"retain_days"`
	RetainFailedDays int    `mapstructure:"retain_failed_days"`
}

type OpenCodeConfig struct {
	BinaryPath     string        `mapstructure:"binary_path"`
	Model          string        `mapstructure:"model"`
	DefaultTimeout time.Duration `mapstructure:"default_timeout"`
	Agents         AgentsConfig  `mapstructure:"agents"`
}

type AgentsConfig struct {
	Discovery      string `mapstructure:"discovery"`
	Decision       string `mapstructure:"decision"`
	Implementation string `mapstructure:"implementation"`
	Validation     string `mapstructure:"validation"`
}

type ConcurrencyConfig struct {
	MaxActivePerProject int `mapstructure:"max_active_per_project"`
}

type AuthConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type WebConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	StaticPath string `mapstructure:"static_path"`
}

type FeatureConfig struct {
	AutoApproveForTests bool `mapstructure:"auto_approve_for_tests"`
}

type ProjectConfig struct {
	Name           string   `mapstructure:"name"`
	RepoPath       string   `mapstructure:"repo_path"`
	BaseBranch     string   `mapstructure:"base_branch"`
	ValidationCmds []string `mapstructure:"validation_commands"`
	LintCmds       []string `mapstructure:"lint_commands"`
	TypecheckCmds  []string `mapstructure:"typecheck_commands"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("ORCHESTRATOR")
	// Map dotted keys to underscore env names (auth.username -> AUTH_USERNAME).
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	applyEnvOverrides(&cfg)
	return &cfg, nil
}

// applyEnvOverrides lets a small, explicit set of settings be supplied from the
// environment (e.g. systemd EnvironmentFile), overriding the YAML. This is used
// for credentials so real secrets can live outside Git. Environment wins only
// when the variable is present and non-empty; the YAML remains the fallback.
func applyEnvOverrides(cfg *Config) {
	if v, ok := os.LookupEnv("ORCHESTRATOR_AUTH_USERNAME"); ok && v != "" {
		cfg.Auth.Username = v
	}
	if v, ok := os.LookupEnv("ORCHESTRATOR_AUTH_PASSWORD"); ok && v != "" {
		cfg.Auth.Password = v
	}
	if v, ok := os.LookupEnv("ORCHESTRATOR_AUTH_ENABLED"); ok && v != "" {
		cfg.Auth.Enabled = v == "true" || v == "1"
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)

	v.SetDefault("database.max_open_conns", 10)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "1h")
	v.SetDefault("store.kind", "memory")

	v.SetDefault("worktree.root_path", "/var/lib/agent-orchestrator/worktrees")
	v.SetDefault("worktree.retain_days", 7)
	v.SetDefault("worktree.retain_failed_days", 30)

	v.SetDefault("opencode.binary_path", "opencode")
	v.SetDefault("opencode.default_timeout", 10*time.Minute)
	v.SetDefault("opencode.agents.discovery", PhaseAgentDiscoveryDefault)
	v.SetDefault("opencode.agents.decision", PhaseAgentDecisionDefault)
	v.SetDefault("opencode.agents.implementation", PhaseAgentImplementationDefault)
	v.SetDefault("opencode.agents.validation", PhaseAgentValidationDefault)

	v.SetDefault("concurrency.max_active_per_project", 1)

	v.SetDefault("auth.enabled", true)
	v.SetDefault("auth.username", "admin")

	v.SetDefault("web.enabled", true)
	v.SetDefault("web.static_path", "./web/dist")

	v.SetDefault("feature.auto_approve_for_tests", false)
}

func (c *Config) AgentForPhase(phase string) string {
	switch phase {
	case "discovery":
		return c.OpenCode.Agents.Discovery
	case "decision":
		return c.OpenCode.Agents.Decision
	case "implementation":
		return c.OpenCode.Agents.Implementation
	case "validation":
		return c.OpenCode.Agents.Validation
	default:
		return ""
	}
}
