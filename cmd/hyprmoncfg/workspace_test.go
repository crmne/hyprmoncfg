package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

type fakeWorkspaceClient struct {
	monitors   []hypr.Monitor
	rules      []hypr.WorkspaceRule
	dispatched []string
}

func (f *fakeWorkspaceClient) Monitors(context.Context) ([]hypr.Monitor, error) {
	return f.monitors, nil
}

func (f *fakeWorkspaceClient) WorkspaceRules(context.Context) ([]hypr.WorkspaceRule, error) {
	return f.rules, nil
}

func (f *fakeWorkspaceClient) Dispatch(_ context.Context, dispatcher string, args ...string) error {
	call := strings.Join(append([]string{dispatcher}, args...), " ")
	f.dispatched = append(f.dispatched, call)
	return nil
}

func TestWorkspaceCommandGoesToTheFocusedDisplaysSlot(t *testing.T) {
	client := &fakeWorkspaceClient{
		monitors: []hypr.Monitor{
			{Name: "eDP-1", Description: "Laptop panel"},
			{Name: "DP-1", Description: "Desk display", Focused: true},
		},
		rules: []hypr.WorkspaceRule{
			{WorkspaceString: "1", Monitor: "desc:Laptop panel", Default: true},
			{WorkspaceString: "11", Monitor: "desc:Desk display", Default: true},
		},
	}
	for _, tc := range []struct{ action, slot, want string }{
		{"focus", "3", "workspace 13"},
		{"move", "10", "movetoworkspace 20"},
		{"move-silent", "1", "movetoworkspacesilent 11"},
	} {
		if err := runWorkspace(context.Background(), client, tc.action, tc.slot); err != nil {
			t.Fatal(err)
		}
		if got := client.dispatched[len(client.dispatched)-1]; got != tc.want {
			t.Fatalf("%s %s dispatched %q, want %q", tc.action, tc.slot, got, tc.want)
		}
	}

	for _, bad := range [][2]string{{"focus", "0"}, {"focus", "11"}, {"focus", "x"}, {"swap", "1"}} {
		if err := runWorkspace(context.Background(), client, bad[0], bad[1]); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}

// Under a Lua config Hyprland rejects classic dispatchers, so the command
// retries in Lua.
func TestWorkspaceCommandFallsBackToLuaDispatch(t *testing.T) {
	client := &fakeWorkspaceClient{monitors: []hypr.Monitor{{Name: "DP-1", Focused: true}}}
	wrapped := &luaOnlyClient{fakeWorkspaceClient: client}
	if err := runWorkspace(context.Background(), wrapped, "move", "4"); err != nil {
		t.Fatal(err)
	}
	if got := client.dispatched; len(got) != 2 || got[1] != `hl.dsp.window.move({ workspace = "4", follow = true })` {
		t.Fatalf("dispatched %q", got)
	}
}

type luaOnlyClient struct{ *fakeWorkspaceClient }

func (l *luaOnlyClient) Dispatch(ctx context.Context, dispatcher string, args ...string) error {
	_ = l.fakeWorkspaceClient.Dispatch(ctx, dispatcher, args...)
	if strings.HasPrefix(dispatcher, "hl.") {
		return nil
	}
	return errors.New("lua config")
}
