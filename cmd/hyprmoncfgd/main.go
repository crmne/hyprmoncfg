package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/crmne/hyprmoncfg/internal/buildinfo"
	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/daemon"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configDir string
	var debounce time.Duration
	var poll time.Duration
	var lidPoll time.Duration
	var forceProfile string
	var quiet bool
	var monitorsConf string
	var hyprConfig string

	cmd := &cobra.Command{
		Use:     "hyprmoncfgd",
		Short:   "Daemon for automatic monitor profile switching",
		Version: buildinfo.Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			base, err := config.EnsureBaseDir(configDir)
			if err != nil {
				return err
			}
			client, err := hypr.NewClient()
			if err != nil {
				return err
			}
			store := profile.NewStore(base)
			if err := store.Ensure(); err != nil {
				return err
			}

			logger := log.New(os.Stdout, "hyprmoncfgd: ", log.LstdFlags)
			logf := func(string, ...any) {}
			if !quiet {
				logf = logger.Printf
			}

			svc := daemon.New(client, store, daemon.Config{
				Debounce:        debounce,
				PollInterval:    poll,
				LidPollInterval: lidPoll,
				ForcedProfile:   forceProfile,
				MonitorsConf:    monitorsConf,
				HyprConfig:      hyprConfig,
				Logf:            logf,
			})

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			logf("starting daemon")
			err = svc.Run(ctx)
			if err != nil {
				return err
			}
			logf("stopped")
			return nil
		},
	}

	cmd.Flags().StringVar(&configDir, "config-dir", "", "Config directory (default: ~/.config/hyprmoncfg)")
	cmd.Flags().DurationVar(&debounce, "debounce", 1200*time.Millisecond, "Debounce duration before applying profile")
	cmd.Flags().DurationVar(&poll, "poll-interval", 5*time.Second, "Polling interval for monitor changes")
	cmd.Flags().DurationVar(&lidPoll, "lid-poll-interval", lid.DefaultPollInterval, "Polling interval for lid-state fallback checks")
	cmd.Flags().StringVar(&forceProfile, "profile", "", "Force this profile instead of auto-matching")
	cmd.Flags().StringVar(&monitorsConf, "monitors-conf", "", "Hyprland monitor config target to write and reload (default: ~/.config/hypr/monitors.conf)")
	cmd.Flags().StringVar(&hyprConfig, "hypr-config", "", "Hyprland root config to verify source directives against (default: ~/.config/hypr/hyprland.conf)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress logs")
	cmd.AddCommand(newVersionCmd("hyprmoncfgd"))

	return cmd
}

func newVersionCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show build version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Summary(name))
		},
	}
}
