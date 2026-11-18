package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// workspaceDispatcher is the subset of the Hyprland client the workspace
// command needs.
type workspaceDispatcher interface {
	Monitors(ctx context.Context) ([]hypr.Monitor, error)
	WorkspaceRules(ctx context.Context) ([]hypr.WorkspaceRule, error)
	Dispatch(ctx context.Context, dispatcher string, args ...string) error
}

func newWorkspaceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "workspace [focus|move|move-silent] <1-10>",
		Short: "Go to, or move the window to, a workspace of the focused display",
		Long: "With the Per display workspace strategy, each display has workspaces 1-10 of its own.\n" +
			"focus goes to that workspace on the focused display, move takes the focused window there\n" +
			"and follows it, and move-silent sends the window without following.\n" +
			"hyprmoncfg binds Super + number and Super + Shift + number to these itself.\n" +
			"Bind move-silent, or other keys, to them in your own Hyprland config.",
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{"focus", "move", "move-silent"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := hypr.NewClient()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			return runWorkspace(ctx, client, args[0], args[1])
		},
	}
}

func runWorkspace(ctx context.Context, client workspaceDispatcher, action, slotArg string) error {
	slot, err := strconv.Atoi(slotArg)
	if err != nil || slot < 1 || slot > profile.PerDisplaySlots {
		return fmt.Errorf("workspace must be 1-%d, got %q", profile.PerDisplaySlots, slotArg)
	}
	legacy, lua := "", ""
	switch action {
	case "focus":
		legacy, lua = "workspace", `hl.dsp.focus({ workspace = %q })`
	case "move":
		legacy, lua = "movetoworkspace", `hl.dsp.window.move({ workspace = %q, follow = true })`
	case "move-silent":
		legacy, lua = "movetoworkspacesilent", `hl.dsp.window.move({ workspace = %q, follow = false })`
	default:
		return fmt.Errorf("unknown action %q: use focus, move or move-silent", action)
	}

	monitors, err := client.Monitors(ctx)
	if err != nil {
		return err
	}
	var focused *hypr.Monitor
	for i := range monitors {
		if monitors[i].Focused {
			focused = &monitors[i]
			break
		}
	}
	workspace := strconv.Itoa(slot)
	if focused != nil {
		rules, err := client.WorkspaceRules(ctx)
		if err != nil {
			return err
		}
		workspace = profile.PerDisplayWorkspace(slot, *focused, rules)
	}

	// The key bindings that run this live in hyprlang configs, but someone may
	// call it under a Lua config too, which only takes Lua dispatchers.
	if err := client.Dispatch(ctx, legacy, workspace); err != nil {
		if luaErr := client.Dispatch(ctx, fmt.Sprintf(lua, workspace)); luaErr != nil {
			return err
		}
	}
	return nil
}
