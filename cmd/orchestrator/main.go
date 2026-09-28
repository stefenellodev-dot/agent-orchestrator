package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/stefenello/agent-orchestrator/internal/api"
	"github.com/stefenello/agent-orchestrator/internal/config"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "orchestrator",
		Version: version,
		Short:   "Human-in-the-Loop Agent Orchestrator for OpenCode",
	}
	rootCmd.PersistentFlags().String("config", "configs/config.example.yaml", "path to config file")

	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(webCmd())
	rootCmd.AddCommand(opencodeRunCmd())
	rootCmd.AddCommand(migrateCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func configPath(cmd *cobra.Command) string {
	p, _ := cmd.Flags().GetString("config")
	return p
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start orchestrator API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath(cmd))
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			return runServe(cfg)
		},
	}
}

func runServe(cfg *config.Config) error {
	if cfg.Store.Kind != "memory" {
		return fmt.Errorf("store kind %q not yet supported (only \"memory\")", cfg.Store.Kind)
	}

	adapter := service.NewCLIAdapter(cfg.OpenCode.BinaryPath)
	if caps, err := adapter.ValidateCLI(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: opencode CLI probe failed: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "opencode %s detected (agents: %v)\n", caps.Version, caps.SupportedAgents)
	}

	store := memory.New()
	worktrees := service.NewGitWorktreeManager(cfg.Worktree.RootPath)
	orch := service.New(store, worktrees, adapter)

	handler := api.NewServer(orch, api.Options{
		AuthEnabled:  cfg.Auth.Enabled,
		AuthUsername: cfg.Auth.Username,
		AuthPassword: cfg.Auth.Password,
		StaticDir:    "",
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "orchestrator listening on %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

func webCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "web",
		Short: "Start web dashboard server",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
}

func opencodeRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "opencode-run",
		Short: "Run a single OpenCode phase (invoked by systemd/Quadlet)",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
}

func migrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
}
