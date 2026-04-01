package apply

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestCommandsForProfile(t *testing.T) {
	mon := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1"}
	p := profile.New("desk", []profile.OutputConfig{{
		Key:       mon.HardwareKey(),
		Name:      "DP-1",
		Enabled:   true,
		Width:     2560,
		Height:    1440,
		Refresh:   144,
		X:         0,
		Y:         0,
		Scale:     1,
		Transform: 0,
	}})

	cmds, err := CommandsForProfile(p, []hypr.Monitor{mon})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0], "DP-1,2560x1440@144") {
		t.Fatalf("unexpected command: %s", cmds[0])
	}
}

func TestCommandsForProfileDisable(t *testing.T) {
	mon := hypr.Monitor{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2"}
	p := profile.New("dock", []profile.OutputConfig{{
		Key:     mon.HardwareKey(),
		Name:    mon.Name,
		Enabled: false,
	}})

	cmds, err := CommandsForProfile(p, []hypr.Monitor{mon})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 || cmds[0] != "HDMI-A-1,disable" {
		t.Fatalf("unexpected disable command: %+v", cmds)
	}
}

func TestKeywordifyMonitorCommandsLeavesWorkspaceAndDispatchCommandsUntouched(t *testing.T) {
	commands := []string{
		"DP-1,2560x1440@144,0x0,1",
		"keyword workspace 1, monitor:desc:Dell U2720Q, default:true",
		"dispatch moveworkspacetomonitor 1 DP-1",
	}

	got := keywordifyMonitorCommands(commands)
	want := []string{
		"keyword monitor DP-1,2560x1440@144,0x0,1",
		"keyword workspace 1, monitor:desc:Dell U2720Q, default:true",
		"dispatch moveworkspacetomonitor 1 DP-1",
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d commands, got %d", len(want), len(got))
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("command %d mismatch: got %q want %q", idx, got[idx], want[idx])
		}
	}
}

func TestWorkspaceCommandsForProfileIncludeKeywordAndDispatch(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD", Serial: "A1"},
		{Name: "eDP-1", Description: "Samsung Display Corp. ATNA60CL10-0", Make: "Samsung Display Corp.", Model: "ATNA60CL10-0", Serial: "B2"},
	}
	p := profile.New("desk", []profile.OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Scale: 1},
	})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: monitors[0].HardwareKey(), OutputName: monitors[0].Name, Default: true, Persistent: true},
			{Workspace: "2", OutputKey: monitors[1].HardwareKey(), OutputName: monitors[1].Name, Default: true, Persistent: true},
		},
	}

	got := WorkspaceCommandsForProfile(p, monitors)
	want := []string{
		"keyword workspace 1, monitor:desc:Microstep MPG321UR-QD, default:true, persistent:true",
		"dispatch moveworkspacetomonitor 1 DP-1",
		"keyword workspace 2, monitor:desc:Samsung Display Corp. ATNA60CL10-0, default:true, persistent:true",
		"dispatch moveworkspacetomonitor 2 eDP-1",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d workspace commands, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("workspace command %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestValidateLayoutRejectsOverlaps(t *testing.T) {
	outputs := []profile.OutputConfig{
		{
			Key:     "left",
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			X:       0,
			Y:       0,
			Scale:   1,
		},
		{
			Key:     "right",
			Name:    "eDP-1",
			Enabled: true,
			Width:   1920,
			Height:  1200,
			X:       2500,
			Y:       0,
			Scale:   1,
		},
	}

	if err := ValidateLayout(outputs); err == nil {
		t.Fatal("expected overlap validation error")
	}
}

func TestRenderHyprlandConfigUsesMonitorV2WithWorkspaceRules(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD"},
	}
	p := profile.New("desk", []profile.OutputConfig{{
		Key:       monitors[0].HardwareKey(),
		Name:      "DP-1",
		Enabled:   true,
		Width:     3840,
		Height:    2160,
		Refresh:   143.99,
		X:         3720,
		Y:         951,
		Scale:     1.33,
		VRR:       1,
		Transform: 0,
	}})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:       true,
		Strategy:      profile.WorkspaceStrategySequential,
		MaxWorkspaces: 3,
		GroupSize:     3,
		MonitorOrder:  []string{monitors[0].HardwareKey()},
	}

	rendered, err := RenderHyprlandConfig(p, monitors, true)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	for _, want := range []string{
		"# Generated by hyprmoncfg",
		"monitorv2 {",
		"output = desc:Microstep MPG321UR-QD",
		"position = 3720x951",
		"scale = 1.33",
		"vrr = 1",
		"workspace = 1, monitor:desc:Microstep MPG321UR-QD, default:true, persistent:true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "persistent:true\n\nworkspace = 2") {
		t.Fatalf("expected workspace rules to be contiguous without blank lines, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "persistent:true\nworkspace = 2, monitor:desc:Microstep MPG321UR-QD") {
		t.Fatalf("expected workspace rules to be separated by a single newline, got:\n%s", rendered)
	}
}

func TestCommandsForProfileMirror(t *testing.T) {
	primary := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1"}
	secondary := hypr.Monitor{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2"}
	p := profile.New("mirror", []profile.OutputConfig{
		{
			Key:     primary.HardwareKey(),
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			Refresh: 144,
			Scale:   1,
		},
		{
			Key:      secondary.HardwareKey(),
			Name:     "HDMI-A-1",
			Enabled:  true,
			Width:    1920,
			Height:   1080,
			Refresh:  60,
			Scale:    1,
			MirrorOf: primary.HardwareKey(),
		},
	})

	cmds, err := CommandsForProfile(p, []hypr.Monitor{primary, secondary})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if !strings.Contains(cmds[1], "1920x1080@60") {
		t.Fatalf("expected mirror command to include resolution, got: %s", cmds[1])
	}
	if !strings.Contains(cmds[1], "mirror,DP-1") {
		t.Fatalf("expected mirror command targeting DP-1, got: %s", cmds[1])
	}
}

func TestRenderHyprlandConfigMirrorV1(t *testing.T) {
	primary := hypr.Monitor{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "A1"}
	secondary := hypr.Monitor{Name: "HDMI-A-1", Description: "LG 27GP850", Make: "LG", Model: "27GP850", Serial: "B2"}
	p := profile.New("mirror", []profile.OutputConfig{
		{
			Key:     primary.HardwareKey(),
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			Refresh: 144,
			Scale:   1,
		},
		{
			Key:      secondary.HardwareKey(),
			Name:     "HDMI-A-1",
			Enabled:  true,
			Width:    1920,
			Height:   1080,
			Refresh:  60,
			Scale:    1,
			MirrorOf: primary.HardwareKey(),
		},
	})

	rendered, err := RenderHyprlandConfig(p, []hypr.Monitor{primary, secondary}, false)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	for _, want := range []string{"1920x1080@60", "mirror,desc:Dell U2720Q"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected v1 mirror line to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestRenderHyprlandConfigMirrorV2(t *testing.T) {
	primary := hypr.Monitor{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "A1"}
	secondary := hypr.Monitor{Name: "HDMI-A-1", Description: "LG 27GP850", Make: "LG", Model: "27GP850", Serial: "B2"}
	p := profile.New("mirror", []profile.OutputConfig{
		{
			Key:     primary.HardwareKey(),
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			Refresh: 144,
			Scale:   1,
		},
		{
			Key:      secondary.HardwareKey(),
			Name:     "HDMI-A-1",
			Enabled:  true,
			Width:    1920,
			Height:   1080,
			Refresh:  60,
			Scale:    1,
			MirrorOf: primary.HardwareKey(),
		},
	})

	rendered, err := RenderHyprlandConfig(p, []hypr.Monitor{primary, secondary}, true)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	// The mirror block should contain mode, position, scale, AND mirror.
	mirrorIdx := strings.Index(rendered, "desc:LG 27GP850")
	if mirrorIdx < 0 {
		t.Fatalf("mirror monitor block not found in:\n%s", rendered)
	}
	mirrorBlock := rendered[mirrorIdx:]
	endIdx := strings.Index(mirrorBlock, "}")
	if endIdx >= 0 {
		mirrorBlock = mirrorBlock[:endIdx]
	}
	for _, want := range []string{"mode = 1920x1080@60", "position =", "scale =", "mirror = desc:Dell U2720Q"} {
		if !strings.Contains(mirrorBlock, want) {
			t.Fatalf("mirror v2 block should contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestValidateLayoutSkipsMirroredMonitors(t *testing.T) {
	outputs := []profile.OutputConfig{
		{
			Key:     "primary",
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			X:       0,
			Y:       0,
			Scale:   1,
		},
		{
			Key:      "secondary",
			Name:     "HDMI-A-1",
			Enabled:  true,
			Width:    2560,
			Height:   1440,
			X:        0,
			Y:        0,
			Scale:    1,
			MirrorOf: "primary",
		},
	}

	if err := ValidateLayout(outputs); err != nil {
		t.Fatalf("mirrored monitor should not trigger overlap: %v", err)
	}
}

func TestSnapshotCommandsMirror(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Width: 2560, Height: 1440, RefreshRate: 144, Scale: 1},
		{Name: "HDMI-A-1", Width: 1920, Height: 1080, RefreshRate: 60, MirrorOf: "DP-1", Scale: 1},
	}

	cmds := SnapshotCommands(monitors)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if !strings.Contains(cmds[1], "1920x1080@60") {
		t.Fatalf("expected snapshot mirror command to include resolution, got: %s", cmds[1])
	}
	if !strings.Contains(cmds[1], "mirror,DP-1") {
		t.Fatalf("expected snapshot mirror command, got: %s", cmds[1])
	}
}

func TestEngineApplyAndRevertReplayWorkspacePlacement(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "hyprctl.log")
	monitorsConfPath := filepath.Join(dir, "monitors.conf")
	hyprlandConfigPath := filepath.Join(dir, "hyprland.conf")
	hyprctlPath := filepath.Join(dir, "hyprctl")

	hyprctlScript := `#!/usr/bin/env bash
set -eu
cmd1="${1-}"
cmd2="${2-}"
cmd3="${3-}"
printf '%s\n' "$*" >> "$HYPRCTL_LOG"

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "version" ]; then
  printf '{"version":"0.54.0"}'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "monitors" ] && [ "$cmd3" = "all" ]; then
  printf '%s' '[{"id":1,"name":"DP-1","description":"Microstep MPG321UR-QD","make":"Microstep","model":"MPG321UR-QD","serial":"A1","width":3840,"height":2160,"refreshRate":143.99,"x":0,"y":0,"scale":1,"transform":0,"focused":true,"dpmsStatus":true,"vrr":true,"disabled":false,"mirrorOf":"","activeWorkspace":{"id":1,"name":"1"}},{"id":2,"name":"eDP-1","description":"Samsung Display Corp. ATNA60CL10-0","make":"Samsung Display Corp.","model":"ATNA60CL10-0","serial":"B2","width":2880,"height":1800,"refreshRate":120,"x":3840,"y":0,"scale":1,"transform":0,"focused":false,"dpmsStatus":true,"vrr":false,"disabled":false,"mirrorOf":"","activeWorkspace":{"id":2,"name":"2"}}]'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "workspacerules" ]; then
  printf '%s' '[{"workspaceString":"1","monitor":"desc:Samsung Display Corp. ATNA60CL10-0","default":true,"persistent":true},{"workspaceString":"2","monitor":"desc:Microstep MPG321UR-QD","default":true,"persistent":true}]'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "workspaces" ]; then
  printf '%s' '[{"id":1,"name":"1","monitor":"eDP-1","monitorID":2,"windows":1,"ispersistent":true},{"id":2,"name":"2","monitor":"DP-1","monitorID":1,"windows":1,"ispersistent":true}]'
  exit 0
fi

if [ "$cmd1" = "reload" ]; then
  exit 0
fi

if [ "$cmd1" = "--batch" ]; then
  printf 'BATCH:%s\n' "$2" >> "$HYPRCTL_LOG"
  exit 0
fi

echo "unexpected args: $*" >&2
exit 1
`

	if err := os.WriteFile(monitorsConfPath, []byte("# initial\n"), 0o644); err != nil {
		t.Fatalf("write monitors.conf: %v", err)
	}
	if err := os.WriteFile(hyprlandConfigPath, []byte("source = "+monitorsConfPath+"\n"), 0o644); err != nil {
		t.Fatalf("write hyprland.conf: %v", err)
	}
	if err := os.WriteFile(hyprctlPath, []byte(hyprctlScript), 0o755); err != nil {
		t.Fatalf("write fake hyprctl: %v", err)
	}

	t.Setenv("HYPRCTL_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	client, err := hypr.NewClient()
	if err != nil {
		t.Fatalf("new hypr client: %v", err)
	}

	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD", Serial: "A1", Width: 3840, Height: 2160, RefreshRate: 143.99, Scale: 1, VRR: true},
		{Name: "eDP-1", Description: "Samsung Display Corp. ATNA60CL10-0", Make: "Samsung Display Corp.", Model: "ATNA60CL10-0", Serial: "B2", Width: 2880, Height: 1800, RefreshRate: 120, X: 3840, Scale: 1},
	}
	p := profile.New("desk", []profile.OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Width: 3840, Height: 2160, Refresh: 143.99, X: 0, Y: 0, Scale: 1, VRR: 1},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Width: 2880, Height: 1800, Refresh: 120, X: 3840, Y: 0, Scale: 1},
	})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: monitors[0].HardwareKey(), OutputName: monitors[0].Name, Default: true, Persistent: true},
			{Workspace: "2", OutputKey: monitors[1].HardwareKey(), OutputName: monitors[1].Name, Default: true, Persistent: true},
		},
	}

	engine := Engine{
		Client:             client,
		MonitorsConfPath:   monitorsConfPath,
		HyprlandConfigPath: hyprlandConfigPath,
	}

	ctx := context.Background()
	snapshot, err := engine.Apply(ctx, p, monitors)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if err := engine.Revert(ctx, snapshot); err != nil {
		t.Fatalf("revert failed: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read hyprctl log: %v", err)
	}
	log := string(logBytes)

	for _, want := range []string{
		"reload",
		"BATCH:keyword workspace 1, monitor:desc:Microstep MPG321UR-QD, default:true, persistent:true ; dispatch moveworkspacetomonitor 1 DP-1 ; keyword workspace 2, monitor:desc:Samsung Display Corp. ATNA60CL10-0, default:true, persistent:true ; dispatch moveworkspacetomonitor 2 eDP-1",
		"BATCH:keyword monitor DP-1,3840x2160@143.99,0x0,1,transform,0,vrr,1 ; keyword monitor eDP-1,2880x1800@120.00,3840x0,1,transform,0,vrr,0 ; keyword workspace 1, monitor:desc:Samsung Display Corp. ATNA60CL10-0, default:true, persistent:true ; keyword workspace 2, monitor:desc:Microstep MPG321UR-QD, default:true, persistent:true ; dispatch moveworkspacetomonitor 1 eDP-1 ; dispatch moveworkspacetomonitor 2 DP-1",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("expected hyprctl log to contain %q, got:\n%s", want, log)
		}
	}
}
