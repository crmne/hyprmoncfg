package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/buildinfo"
	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/tui"
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
	root.PersistentFlags().StringVar(&monitorsConf, "monitors-conf", "", "Hyprland monitor config target to write and reload (default: ~/.config/hypr/monitors.conf)")
	root.PersistentFlags().StringVar(&hyprConfig, "hypr-config", "", "Hyprland root config to verify source directives against (default: ~/.config/hypr/hyprland.conf)")

	root.AddCommand(newTUICmd(&configDir, &monitorsConf, &hyprConfig))
	root.AddCommand(newMonitorsCmd(&configDir))
	root.AddCommand(newProfilesCmd(&configDir))
	root.AddCommand(newSaveCmd(&configDir))
	root.AddCommand(newApplyCmd(&configDir, &monitorsConf, &hyprConfig))
	root.AddCommand(newDeleteCmd(&configDir))
	root.AddCommand(newVersionCmd("hyprmoncfg"))

	return root
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
			if err := store.Save(p); err != nil {
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

			engine := apply.Engine{
				Client:             client,
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

			keep, err := confirmApply(confirmTimeout)
			if err != nil {
				return err
			}
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
			if err := engine.Revert(revertCtx, snapshot); err != nil {
				return fmt.Errorf("failed to revert after denied confirmation: %w", err)
			}
			fmt.Println("Configuration reverted")
			return nil
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
			if err := store.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted profile %q\n", args[0])
			return nil
		},
	}
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

	model := tui.NewModel(client, store, monitorsConf, hyprConfig)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
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

func confirmApply(timeoutSec int) (bool, error) {
	fmt.Printf("Keep this configuration? [y/N] (auto-revert in %ds): ", timeoutSec)
	inputCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		reader := bufio.NewReader(os.Stdin)
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
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		return false, nil
	}
}
