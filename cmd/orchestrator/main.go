package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
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

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start orchestrator API server",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
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
