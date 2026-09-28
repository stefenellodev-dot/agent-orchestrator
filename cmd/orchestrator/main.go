package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/stefenello/agent-orchestrator/internal/api"
	"github.com/stefenello/agent-orchestrator/internal/buildinfo"
	"github.com/stefenello/agent-orchestrator/internal/config"
	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
	"github.com/stefenello/agent-orchestrator/internal/store/postgres"
	"github.com/stefenello/agent-orchestrator/internal/web"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "orchestrator",
		Version: buildinfo.Version,
		Short:   "Human-in-the-Loop Agent Orchestrator for OpenCode",
	}
	rootCmd.PersistentFlags().String("config", "configs/config.example.yaml", "path to config file")

	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(webCmd())
	rootCmd.AddCommand(opencodeRunCmd())
	rootCmd.AddCommand(migrateCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func configPath(cmd *cobra.Command) string {
	p, _ := cmd.Flags().GetString("config")
	return p
}

// versionCmd prints the full build identity, including the exact Git commit, so
// a downloaded or running artifact can be verified before/after deployment.
func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build identity (version, commit, build time)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := json.MarshalIndent(buildinfo.Current(), "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		},
	}
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
	id := buildinfo.Current()
	fmt.Fprintf(os.Stderr, "orchestrator starting: %s\n", id.String())

	adapter := service.NewCLIAdapter(cfg.OpenCode.BinaryPath)
	if caps, err := adapter.ValidateCLI(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: opencode CLI probe failed: %v\n", err)
	} else {
		id.OpenCode = caps.Version
		id.Agents = strings.Join(caps.SupportedAgents, ",")
		fmt.Fprintf(os.Stderr, "opencode %s detected (agents: %v)\n", caps.Version, caps.SupportedAgents)
	}

	st, cleanup, err := buildStore(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	worktrees := service.NewGitWorktreeManager(cfg.Worktree.RootPath)
	orch := service.New(st, worktrees, adapter)
	orch.SetAutoDrive(true)
	orch.SetAutoApprove(cfg.Feature.AutoApproveForTests)
	orch.SetPhaseTimeout(cfg.OpenCode.DefaultTimeout)
	orch.SetModel(cfg.OpenCode.Model)
	orch.SetAgents(
		cfg.OpenCode.Agents.Discovery,
		cfg.OpenCode.Agents.Decision,
		cfg.OpenCode.Agents.Implementation,
		cfg.OpenCode.Agents.Validation,
	)
	if cfg.Feature.AutoApproveForTests {
		fmt.Fprintln(os.Stderr, "WARNING: auto-approve for tests is ENABLED; the human gate is bypassed")
	}
	for _, p := range cfg.Projects {
		orch.RegisterProject(service.ProjectConfig{
			Name:       p.Name,
			RepoPath:   p.RepoPath,
			BaseBranch: p.BaseBranch,
			Validation: service.ProjectValidation{
				TestCommands:      p.ValidationCmds,
				LintCommands:      p.LintCmds,
				TypecheckCommands: p.TypecheckCmds,
			},
		})
	}

	handler := api.NewServer(orch, api.Options{
		AuthEnabled:  cfg.Auth.Enabled,
		AuthUsername: cfg.Auth.Username,
		AuthPassword: cfg.Auth.Password,
		StaticFS:     staticFS(cfg),
		BuildInfo:    id,
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
		Short: "Start API server with the web dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath(cmd))
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			return runServe(cfg)
		},
	}
}

// staticFS returns the embedded dashboard when the web UI is enabled.
func staticFS(cfg *config.Config) fs.FS {
	if !cfg.Web.Enabled {
		return nil
	}
	return web.Dist()
}

// buildStore selects the persistence backend from config. The returned cleanup
// must be called on shutdown.
func buildStore(ctx context.Context, cfg *config.Config) (store.Store, func(), error) {
	switch cfg.Store.Kind {
	case "", "memory":
		return memory.New(), func() {}, nil
	case "postgres":
		if cfg.Database.DSN == "" {
			return nil, nil, errors.New("database.dsn is required for postgres store")
		}
		pool, err := pgxpool.New(ctx, cfg.Database.DSN)
		if err != nil {
			return nil, nil, fmt.Errorf("connect database: %w", err)
		}
		if err := postgres.Migrate(ctx, pool); err != nil {
			pool.Close()
			return nil, nil, fmt.Errorf("migrate: %w", err)
		}
		return postgres.NewStore(pool), pool.Close, nil
	default:
		return nil, nil, fmt.Errorf("unknown store kind %q", cfg.Store.Kind)
	}
}

func opencodeRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "opencode-run",
		Short: "Run a single OpenCode phase in a worktree (worker entrypoint)",
		Long: "Runs exactly one OpenCode phase against the WorkItem's worktree using only\n" +
			"verified CLI capabilities. Prints the objective RunResult as JSON and exits\n" +
			"with the OpenCode process's exit code. Used by the Piave systemd/Quadlet worker.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath(cmd))
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			worktree, _ := cmd.Flags().GetString("worktree")
			agent, _ := cmd.Flags().GetString("agent")
			phase, _ := cmd.Flags().GetString("phase")
			prompt, _ := cmd.Flags().GetString("prompt")
			model, _ := cmd.Flags().GetString("model")
			if worktree == "" || prompt == "" {
				return errors.New("--worktree and --prompt are required")
			}
			if model == "" {
				model = cfg.OpenCode.Model
			}

			adapter := service.NewCLIAdapter(cfg.OpenCode.BinaryPath)
			if _, err := adapter.ValidateCLI(cmd.Context()); err != nil {
				return err
			}
			res, err := adapter.Run(cmd.Context(), domain.RunRequest{
				WorktreePath: worktree,
				Phase:        domain.Phase(phase),
				Agent:        agent,
				Model:        model,
				Prompt:       prompt,
				Timeout:      cfg.OpenCode.DefaultTimeout,
			})
			if err != nil {
				return err
			}
			out, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			if res.ExitCode != 0 {
				os.Exit(res.ExitCode)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.String("worktree", "", "worktree path (required)")
	f.String("agent", "", "OpenCode agent name")
	f.String("phase", "", "phase name (discovery|decision|implementation|validation)")
	f.String("prompt", "", "prompt to send (required)")
	f.String("model", "", "provider/model (defaults to config)")
	return cmd
}

func migrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath(cmd))
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if cfg.Database.DSN == "" {
				return errors.New("database.dsn is required")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			pool, err := pgxpool.New(ctx, cfg.Database.DSN)
			if err != nil {
				return fmt.Errorf("connect database: %w", err)
			}
			defer pool.Close()

			if err := postgres.Migrate(ctx, pool); err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			fmt.Fprintln(os.Stderr, "migrations applied")
			return nil
		},
	}
}
