package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/buildinfo"
	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/omarchywatch"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/profileio"
	"github.com/crmne/hyprmoncfg/internal/tui"
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
	var monitorsConf string
	var hyprConfig string

	root := &cobra.Command{
		Use:     "hyprmoncfg",
		Short:   "Monitor profile manager for Hyprland",
		Version: buildinfo.Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(configDir, monitorsConf, hyprConfig)
		},
	}
	root.PersistentFlags().StringVar(&configDir, "config-dir", "", "Config directory (default: ~/.config/hyprmoncfg)")
	root.PersistentFlags().StringVar(&monitorsConf, "monitors-conf", "", "Generated monitor config target to write and reload (overrides HYPRMONCFG_MONITORS_CONF)")
	root.PersistentFlags().StringVar(&hyprConfig, "hypr-config", "", "Hyprland root config for include verification (overrides HYPRLAND_CONFIG)")

	root.AddCommand(newTUICmd(&configDir, &monitorsConf, &hyprConfig))
	root.AddCommand(newMonitorsCmd(&configDir))
	root.AddCommand(newProfilesCmd(&configDir))
	root.AddCommand(newStatusCmd(&configDir))
	root.AddCommand(newSaveCmd(&configDir))
	root.AddCommand(newApplyCmd(&configDir, &monitorsConf, &hyprConfig))
	root.AddCommand(newDeleteCmd(&configDir))
	root.AddCommand(newDoctorCmd(&monitorsConf, &hyprConfig))
	root.AddCommand(newManageCmd(&configDir, &monitorsConf, &hyprConfig))
	root.AddCommand(newUnmanageCmd(&configDir, &monitorsConf, &hyprConfig))
	root.AddCommand(newVersionCmd("hyprmoncfg"))

	return root
}

func newStatusCmd(configDir *string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current profile and daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			document, remote, err := daemonStatus(ctx)
			if err != nil {
				return err
			}
			if !remote {
				client, store, err := bootstrap(*configDir)
				if err != nil {
					return err
				}
				profiles, err := store.List()
				if err != nil {
					return err
				}
				monitors, err := client.Monitors(ctx)
				if err != nil {
					return err
				}
				rules, err := client.WorkspaceRules(ctx)
				if err != nil {
					return err
				}
				document = appstatus.Build(buildinfo.Version, false, profiles, monitors, rules)
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(document)
			}

			activeProfile := "custom layout"
			if document.ActiveProfile != nil {
				activeProfile = document.ActiveProfile.Name
			}
			daemonState := "stopped"
			if document.Daemon.Running {
				daemonState = "running"
			}
			enabledMonitors := 0
			for _, monitor := range document.Monitors {
				if monitor.Enabled {
					enabledMonitors++
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Active profile: %s\n", activeProfile)
			if document.RecommendedProfile != nil && document.RecommendedProfile.Name != activeProfile {
				fmt.Fprintf(cmd.OutOrStdout(), "Recommended profile: %s\n", document.RecommendedProfile.Name)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Daemon: %s\n", daemonState)
			if running := strings.TrimSpace(document.Version); running != "" && document.Daemon.Running {
				if installed := strings.TrimSpace(buildinfo.Version); installed != "" && installed != running {
					fmt.Fprintf(cmd.OutOrStdout(),
						"Daemon is still running %s while %s is installed; restart it with `systemctl --user restart hyprmoncfgd`\n",
						running, installed)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Displays: %d enabled, %d connected\n", enabledMonitors, len(document.Monitors))
			fmt.Fprintf(cmd.OutOrStdout(), "Saved profiles: %d\n", len(document.Profiles))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print machine-readable JSON")
	return cmd
}

func newTUICmd(configDir *string, monitorsConf *string, hyprConfig *string) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive terminal UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(*configDir, *monitorsConf, *hyprConfig)
		},
	}
}

func newMonitorsCmd(configDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "monitors",
		Short: "List current monitors from Hyprland",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := bootstrap(*configDir)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			monitors, err := client.Monitors(ctx)
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATE\tMODE\tPOSITION\tSCALE\tKEY")
			for _, m := range monitors {
				state := "on"
				if m.Disabled {
					state = "off"
				}
				mode := fmt.Sprintf("%dx%d@%.2f", m.Width, m.Height, m.RefreshRate)
				if m.Width == 0 || m.Height == 0 {
					mode = "preferred"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%dx%d\t%.2f\t%s\n", m.Name, state, mode, m.X, m.Y, m.Scale, m.HardwareKey())
			}
			return w.Flush()
		},
	}
}

func newProfilesCmd(configDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List saved profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := bootstrap(*configDir)
			if err != nil {
				return err
			}
			profiles, err := store.List()
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				fmt.Println("No saved profiles")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tOUTPUTS\tUPDATED")
			for _, p := range profiles {
				fmt.Fprintf(w, "%s\t%d\t%s\n", p.Name, len(p.Outputs), p.UpdatedAt.Local().Format(time.RFC3339))
			}
			return w.Flush()
		},
	}
}

func newSaveCmd(configDir *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Save current monitor state as profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			client, store, err := bootstrap(*configDir)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			monitors, err := client.Monitors(ctx)
			if err != nil {
				return err
			}
			rules, err := client.WorkspaceRules(ctx)
			if err != nil {
				return err
			}
			p := profile.FromState(name, monitors, rules)
			existing, err := store.Load(name)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			p.Exec = existing.Exec
			session, err := openWriterSession(cmd.Context())
			if err != nil {
				return err
			}
			defer session.Close()
			if session.ipc != nil {
				saveCtx, saveCancel := context.WithTimeout(cmd.Context(), 10*time.Second)
				defer saveCancel()
				if err := session.ipc.Save(saveCtx, ipc.SaveParams{Profile: p}); err != nil {
					return err
				}
			} else if err := profileio.SaveWithSidecars(store, p); err != nil {
				return err
			}
			fmt.Printf("Saved profile %q\n", p.Name)
			return nil
		},
	}
	return cmd
}

func newApplyCmd(configDir *string, monitorsConf *string, hyprConfig *string) *cobra.Command {
	var confirmTimeout int

	cmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Apply a saved profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, store, err := bootstrap(*configDir)
			if err != nil {
				return err
			}
			p, err := store.Load(args[0])
			if err != nil {
				return err
			}
			session, err := openWriterSession(cmd.Context())
			if err != nil {
				return err
			}
			defer session.Close()
			if session.ipc != nil {
				return runRemoteApply(cmd, session.ipc, p, confirmTimeout)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			monitors, err := client.Monitors(ctx)
			if err != nil {
				return err
			}
			applyProfile := p
			if state, err := lid.ReadState(ctx); err == nil && state == lid.Closed {
				applyProfile, _ = profile.ApplyClosedLidPolicy(p, monitors)
			}

			isInteractive := confirmTimeout > 0
			var applySignals chan os.Signal
			if isInteractive {
				applySignals = make(chan os.Signal, 1)
				signal.Notify(applySignals, os.Interrupt, syscall.SIGTERM)
				defer signal.Stop(applySignals)
			}

			engine := apply.Engine{
				Client:             client,
				WakeConfig:         omarchywatch.NewWakeConfig(),
				MonitorsConfPath:   *monitorsConf,
				HyprlandConfigPath: *hyprConfig,
				Logf: func(format string, args ...any) {
					fmt.Printf(format, args...)
				},
			}
			snapshot, err := engine.Apply(ctx, applyProfile, monitors, apply.ApplyModeInteractive)
			if err != nil {
				return err
			}
			fmt.Printf("Applied profile %q\n", p.Name)

			if !isInteractive {
				postApplyCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				if err := engine.PostApply(postApplyCtx, applyProfile); err != nil {
					fmt.Printf("Post-apply failed for %s: %v\n", p.Name, err)
				}
				return nil
			}

			keep, err := confirmApplyWithInput(confirmTimeout, cmd.InOrStdin(), cmd.OutOrStdout(), applySignals)
			if keep {
				fmt.Println("Configuration kept")

				postApplyCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()

				err = engine.PostApply(postApplyCtx, applyProfile)
				if err != nil {
					fmt.Printf("Post-apply failed for %s: %v\n", p.Name, err)
				}

				return nil
			}

			revertCtx, revertCancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer revertCancel()
			revertErr := engine.Revert(revertCtx, snapshot)
			if revertErr != nil {
				revertErr = fmt.Errorf("failed to revert unconfirmed configuration: %w", revertErr)
				if err != nil {
					return errors.Join(err, revertErr)
				}
				return revertErr
			}
			fmt.Println("Configuration reverted")
			return err
		},
	}
	cmd.Flags().IntVar(&confirmTimeout, "confirm-timeout", 10, "Seconds to confirm configuration before reverting; set 0 to disable")
	return cmd
}

func newDeleteCmd(configDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete saved profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := bootstrap(*configDir)
			if err != nil {
				return err
			}
			session, err := openWriterSession(cmd.Context())
			if err != nil {
				return err
			}
			defer session.Close()
			if session.ipc != nil {
				ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
				defer cancel()
				if err := session.ipc.Delete(ctx, args[0]); err != nil {
					return err
				}
			} else if err := store.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted profile %q\n", args[0])
			return nil
		},
	}
}

func newDoctorCmd(monitorsConf *string, hyprConfig *string) *cobra.Command {
	var fix bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that Hyprland reads hyprmoncfg's monitor config last",
		Long: "Report setups where another tool's monitor rules load after hyprmoncfg's " +
			"and override the applied layout on every Hyprland reload.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd, *monitorsConf, *hyprConfig, fix)
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Rewrite the Hyprland config to load hyprmoncfg's monitors last")
	return cmd
}

func runDoctor(cmd *cobra.Command, monitorsConf string, hyprConfig string, fix bool) error {
	out := cmd.OutOrStdout()

	version := ""
	if client, err := hypr.NewClient(); err == nil {
		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
		defer cancel()
		if info, err := client.Version(ctx); err == nil {
			version = info.Version
		}
	}

	resolved, err := config.ResolveHyprlandConfig(version, monitorsConf, hyprConfig)
	if err != nil {
		return err
	}

	line := config.IncludeLine(resolved.Format, resolved.MonitorsPath)
	if err := config.VerifyGeneratedMonitors(resolved.MonitorsPath); err != nil {
		fmt.Fprintf(out, "PROBLEM  %v.\n", err)
		fmt.Fprintln(out, "         Hyprland has no generated hyprmoncfg monitor rules to load.")
		fmt.Fprintln(out, "         Save and apply a profile to recreate the file.")
		return nil
	}
	if err := config.VerifyLoadedLast(resolved.RootPath, resolved.Format, resolved.MonitorsPath); err == nil {
		fmt.Fprintf(out, "OK  %s loads %s last\n", resolved.RootPath, resolved.MonitorsPath)
		return nil
	}

	fmt.Fprintf(out, "PROBLEM  %s does not load %s last.\n", resolved.RootPath, resolved.MonitorsPath)
	fmt.Fprintln(out, "         Anything loaded after it can override the layout hyprmoncfg applies.")
	if !fix {
		fmt.Fprintf(out, "         The daemon and every apply fix this. To do it now: hyprmoncfg doctor --fix\n")
		fmt.Fprintf(out, "         Or add this line at the end of %s yourself:\n\n           %s\n", resolved.RootPath, line)
		return nil
	}

	result, err := config.EnsureIncluded(resolved.RootPath, resolved.Format, resolved.MonitorsPath)
	if err != nil {
		return err
	}
	action := "Moved"
	if result.Added {
		action = "Added"
	}
	fmt.Fprintf(out, "FIXED    %s the include at the end of %s:\n\n           %s\n", action, result.RootPath, result.Line)
	fmt.Fprintln(out, "\n         If your dotfiles are managed elsewhere, keep that line in your source copy.")
	return nil
}
func newManageCmd(configDir *string, monitorsConf *string, hyprConfig *string) *cobra.Command {
	return &cobra.Command{
		Use:   "manage",
		Short: "Let hyprmoncfg manage monitor configuration",
		Long: "Put hyprmoncfg's include back in the Hyprland config and let automatic " +
			"switching resume. Where Omarchy is installed, its monitor watcher steps aside again.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetManaged(cmd, true, *configDir, *monitorsConf, *hyprConfig)
		},
	}
}

func newUnmanageCmd(configDir *string, monitorsConf *string, hyprConfig *string) *cobra.Command {
	return &cobra.Command{
		Use:   "unmanage",
		Short: "Hand monitor configuration back to Hyprland",
		Long: "Stop automatic switching and take hyprmoncfg's include out of the Hyprland " +
			"config, so whatever you or your distro configured has the last word again. " +
			"Where Omarchy is installed, its monitor watcher resumes.\n\n" +
			"Stopping the daemon does not do this on its own: the generated rules keep " +
			"loading on every reload, so anything else that writes monitor config still " +
			"loses to a hyprmoncfg that is no longer running.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetManaged(cmd, false, *configDir, *monitorsConf, *hyprConfig)
		},
	}
}

// runSetManaged prefers the running daemon, which can also hand Omarchy's
// watcher back and reload Hyprland. Without one it does the part that lives on
// disk, so the choice still holds the next time the daemon starts.
func runSetManaged(cmd *cobra.Command, managed bool, configDir string, monitorsConf string, hyprConfig string) error {
	out := cmd.OutOrStdout()

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	if handled, err := setManagedViaDaemon(ctx, managed); err != nil {
		return err
	} else if handled {
		fmt.Fprintln(out, managedSummary(managed))
		return nil
	}

	base, err := config.EnsureBaseDir(configDir)
	if err != nil {
		return err
	}
	if err := config.SetManaged(base, managed); err != nil {
		return err
	}

	resolved, err := resolveHyprConfigForCLI(cmd, monitorsConf, hyprConfig)
	if err != nil {
		return err
	}

	if managed {
		result, err := config.EnsureIncluded(resolved.RootPath, resolved.Format, resolved.MonitorsPath)
		if err != nil {
			return err
		}
		if result.ReadOnly {
			fmt.Fprintf(out, "%s is read-only, so add this line at its end yourself:\n\n  %s\n\n", result.RootPath, result.Line)
		}
	} else {
		result, err := config.RemoveInclude(resolved.RootPath, resolved.Format)
		if err != nil {
			return err
		}
		if result.ReadOnly {
			fmt.Fprintf(out, "%s is read-only, so remove hyprmoncfg's include from it yourself.\n\n", result.RootPath)
		}
	}

	if client, err := hypr.NewClient(); err == nil {
		if err := client.Reload(ctx); err != nil {
			fmt.Fprintf(out, "Could not reload Hyprland, so this takes effect on its next reload: %v\n", err)
		}
	}

	fmt.Fprintln(out, managedSummary(managed))
	fmt.Fprintln(out, "\nThe daemon is not running. Start it to pick this up:")
	fmt.Fprintln(out, "  systemctl --user start hyprmoncfgd.service")
	return nil
}

func managedSummary(managed bool) string {
	if managed {
		return "hyprmoncfg manages monitor configuration."
	}
	return "Monitor configuration is Hyprland's again. Run `hyprmoncfg manage` to hand it back."
}

// setManagedViaDaemon reports whether a running daemon took the request.
func setManagedViaDaemon(ctx context.Context, managed bool) (bool, error) {
	path, err := ipc.SocketPath()
	if err != nil {
		return false, nil
	}
	client, err := ipc.Dial(ctx, path)
	if err != nil {
		return false, nil
	}
	defer client.Close()

	if managed {
		return true, client.Manage(ctx)
	}
	return true, client.Unmanage(ctx)
}

func resolveHyprConfigForCLI(cmd *cobra.Command, monitorsConf string, hyprConfig string) (config.ResolvedHyprConfig, error) {
	version := ""
	if client, err := hypr.NewClient(); err == nil {
		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
		defer cancel()
		if info, err := client.Version(ctx); err == nil {
			version = info.Version
		}
	}
	return config.ResolveHyprlandConfig(version, monitorsConf, hyprConfig)
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

func runTUI(configDir string, monitorsConf string, hyprConfig string) error {
	client, store, err := bootstrap(configDir)
	if err != nil {
		return err
	}

	sessionCtx, sessionCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sessionCancel()
	session, err := openWriterSession(sessionCtx)
	if err != nil {
		return err
	}
	defer session.Close()

	model := tui.NewModel(client, store, monitorsConf, hyprConfig)
	if session.ipc != nil {
		model = tui.NewModelWithIPC(client, store, monitorsConf, hyprConfig, session.ipc)
	}
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, runErr := p.Run()

	revertCtx, revertCancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer revertCancel()
	if revertErr := model.RevertPending(revertCtx); revertErr != nil {
		revertErr = fmt.Errorf("failed to revert unconfirmed configuration while quitting: %w", revertErr)
		if runErr != nil {
			return errors.Join(runErr, revertErr)
		}
		return revertErr
	}
	return runErr
}

func bootstrap(explicitConfigDir string) (*hypr.Client, *profile.Store, error) {
	base, err := config.EnsureBaseDir(explicitConfigDir)
	if err != nil {
		return nil, nil, err
	}
	client, err := hypr.NewClient()
	if err != nil {
		return nil, nil, err
	}
	store := profile.NewStore(base)
	if err := store.Ensure(); err != nil {
		return nil, nil, err
	}
	return client, store, nil
}

type writerSession struct {
	ipc  *ipc.Client
	lock *writerlock.Lock
}

func (s *writerSession) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.ipc != nil {
		errs = append(errs, s.ipc.Close())
		s.ipc = nil
	}
	if s.lock != nil {
		errs = append(errs, s.lock.Close())
		s.lock = nil
	}
	return errors.Join(errs...)
}

func daemonStatus(ctx context.Context) (appstatus.Document, bool, error) {
	path, err := ipc.SocketPath()
	if err != nil {
		return appstatus.Document{}, false, nil
	}
	client, err := ipc.Dial(ctx, path)
	if err != nil {
		return appstatus.Document{}, false, nil
	}
	defer client.Close()
	document, err := client.Status(ctx)
	if err != nil {
		return appstatus.Document{}, true, err
	}
	return document, true, nil
}

func openWriterSession(ctx context.Context) (*writerSession, error) {
	path, err := ipc.SocketPath()
	if err != nil {
		// Direct mode predates the daemon socket and remains the recovery path
		// on sessions without an XDG runtime directory.
		return &writerSession{}, nil
	}

	waitCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
	}

	for {
		dialCtx, cancel := context.WithTimeout(waitCtx, 150*time.Millisecond)
		client, dialErr := ipc.Dial(dialCtx, path)
		cancel()
		if dialErr == nil {
			healthCtx, healthCancel := context.WithTimeout(waitCtx, 750*time.Millisecond)
			_, statusErr := client.Status(healthCtx)
			healthCancel()
			if statusErr == nil {
				return &writerSession{ipc: client}, nil
			}
			_ = client.Close()
		}

		lock, lockErr := writerlock.TryAcquire()
		if lockErr == nil {
			return &writerSession{lock: lock}, nil
		}
		if !errors.Is(lockErr, writerlock.ErrBusy) {
			return nil, lockErr
		}

		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return nil, fmt.Errorf("daemon owns monitor configuration but its IPC socket is unavailable: %w", waitCtx.Err())
		case <-timer.C:
		}
	}
}

func runRemoteApply(cmd *cobra.Command, client *ipc.Client, target profile.Profile, confirmTimeout int) error {
	isInteractive := confirmTimeout > 0
	var applySignals chan os.Signal
	if isInteractive {
		applySignals = make(chan os.Signal, 1)
		signal.Notify(applySignals, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(applySignals)
	}

	preview := func() (ipc.Transaction, error) {
		timeout := confirmTimeout
		if timeout <= 0 {
			timeout = 10
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()
		return client.Preview(ctx, ipc.PreviewParams{
			Profile:        &target,
			TimeoutSeconds: timeout,
		})
	}

	transaction, err := preview()
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Applied profile %q\n", target.Name)
	if !isInteractive {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return client.Confirm(ctx, transaction.ID)
	}

	keep, confirmErr := confirmApplyWithInput(confirmTimeout, cmd.InOrStdin(), cmd.OutOrStdout(), applySignals)
	if keep {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Confirm(ctx, transaction.ID); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Configuration kept")
		return nil
	}

	revertCtx, revertCancel := context.WithTimeout(context.Background(), 10*time.Second)
	revertErr := client.Revert(revertCtx, transaction.ID)
	revertCancel()
	if errors.Is(revertErr, ipc.ErrTransactionUnavailable) {
		revertErr = nil
	}
	if revertErr != nil {
		revertErr = fmt.Errorf("failed to revert unconfirmed configuration: %w", revertErr)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Configuration reverted")
	return errors.Join(confirmErr, revertErr)
}

func confirmApplyWithInput(timeoutSec int, input io.Reader, output io.Writer, signals <-chan os.Signal) (bool, error) {
	fmt.Fprintf(output, "Keep this configuration? [y/N] (auto-revert in %ds): ", timeoutSec)
	inputCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		reader := bufio.NewReader(input)
		line, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		inputCh <- strings.TrimSpace(strings.ToLower(line))
	}()

	select {
	case line := <-inputCh:
		return line == "y" || line == "yes", nil
	case err := <-errCh:
		return false, err
	case <-signals:
		fmt.Fprintln(output)
		return false, nil
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		return false, nil
	}
}
