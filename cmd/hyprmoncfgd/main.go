package main

import (
	"context"
	"errors"
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
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/omarchywatch"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/writerlock"
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
	var wakeSettle time.Duration
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
			signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stopSignals()
			ctx, cancel := context.WithCancel(signalCtx)
			defer cancel()

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

			// Built before the service so the service can hand the watcher back
			// when someone turns monitor management off.
			watcherOwner := omarchywatch.New(logf)

			svc := daemon.New(client, store, daemon.Config{
				Debounce:        debounce,
				WakeSettle:      wakeSettle,
				PollInterval:    poll,
				LidPollInterval: lidPoll,
				ForcedProfile:   forceProfile,
				MonitorsConf:    monitorsConf,
				HyprConfig:      hyprConfig,
				ConfigDir:       base,
				ClaimWatcher:    watcherOwner.Start,
				ReleaseWatcher:  watcherOwner.Release,
				LaptopToggle:    omarchywatch.NewLaptopToggle(),
				WakeConfig:      omarchywatch.NewWakeConfig(),
				Logf:            logf,
			})
			socketPath, err := ipc.SocketPath()
			if err != nil {
				return err
			}
			ownership, err := writerlock.TryAcquire()
			if err != nil {
				return fmt.Errorf("claim monitor writer ownership: %w", err)
			}
			defer ownership.Close()
			server := &ipc.Server{Path: socketPath, Handler: svc, Logf: logf}
			svc.SetNotifier(server.Notify)

			logf("starting daemon")
			// Someone who handed monitor configuration back to Hyprland does not
			// want Omarchy's watcher stopped on their behalf either.
			if config.IsManaged(base) {
				watcherOwner.Start(ctx)
			} else {
				logf("monitor management is off; not claiming displays")
			}

			errCh := make(chan error, 2)
			go func() { errCh <- server.Run(ctx) }()
			go func() { errCh <- svc.Run(ctx) }()
			firstErr := <-errCh
			cancel()
			secondErr := <-errCh
			err = errors.Join(firstErr, secondErr, svc.Shutdown())

			restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 5*time.Second)
			restoreErr := watcherOwner.Release(restoreCtx)
			restoreCancel()
			if restoreErr != nil {
				logf("could not restore Omarchy monitor watcher: %v", restoreErr)
			}
			if err != nil {
				return err
			}
			logf("stopped")
			return nil
		},
	}

	cmd.Flags().StringVar(&configDir, "config-dir", "", "Config directory (default: ~/.config/hyprmoncfg)")
	cmd.Flags().DurationVar(&debounce, "debounce", 1200*time.Millisecond, "Debounce duration before applying profile")
	cmd.Flags().DurationVar(&wakeSettle, "wake-settle", 2*time.Second, "Quiet period after display wake before applying profile")
	cmd.Flags().DurationVar(&poll, "poll-interval", 5*time.Second, "Polling interval for monitor changes")
	cmd.Flags().DurationVar(&lidPoll, "lid-poll-interval", lid.DefaultPollInterval, "Polling interval for lid-state fallback checks")
	cmd.Flags().StringVar(&forceProfile, "profile", "", "Force this profile instead of auto-matching")
	cmd.Flags().StringVar(&monitorsConf, "monitors-conf", "", "Generated monitor config target to write and reload (overrides HYPRMONCFG_MONITORS_CONF)")
	cmd.Flags().StringVar(&hyprConfig, "hypr-config", "", "Hyprland root config for include verification (overrides HYPRLAND_CONFIG)")
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
