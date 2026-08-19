package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestDisplayPowerState(t *testing.T) {
	tests := []struct {
		name     string
		monitors []hypr.Monitor
		want     displayPower
	}{
		{name: "none", want: displayPowerUnknown},
		{name: "disabled", monitors: []hypr.Monitor{{Name: "eDP-1", Disabled: true}}, want: displayPowerUnknown},
		{name: "asleep", monitors: []hypr.Monitor{{Name: "eDP-1"}}, want: displayPowerAsleep},
		{name: "awake", monitors: []hypr.Monitor{{Name: "eDP-1", DPMSStatus: true}}, want: displayPowerAwake},
		{name: "one output awake", monitors: []hypr.Monitor{{Name: "eDP-1", DPMSStatus: true}, {Name: "DP-1"}}, want: displayPowerAwake},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayPowerState(tt.monitors); got != tt.want {
				t.Fatalf("displayPowerState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDisplaySleepGuardPreservesSleepAcrossMissingOutputs(t *testing.T) {
	guard := displaySleepGuard{}

	if got := guard.Observe([]hypr.Monitor{{Name: "eDP-1", DPMSStatus: true}}); got != displaySleepUnchanged {
		t.Fatalf("initial awake transition = %v", got)
	}
	if got := guard.Observe([]hypr.Monitor{{Name: "eDP-1"}}); got != displaySleepEntered || !guard.sleeping {
		t.Fatalf("sleep transition = %v, sleeping = %t", got, guard.sleeping)
	}
	if got := guard.Observe(nil); got != displaySleepUnchanged || !guard.sleeping {
		t.Fatalf("missing-output transition = %v, sleeping = %t", got, guard.sleeping)
	}
	if got := guard.Observe([]hypr.Monitor{{Name: "eDP-1", DPMSStatus: true}}); got != displaySleepExited || guard.sleeping {
		t.Fatalf("wake transition = %v, sleeping = %t", got, guard.sleeping)
	}
}

func TestInternalOnlyFallbackProfileEnablesInternalWhenAllOutputsDisabled(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, X: 3840, Scale: 1.5, Disabled: true},
	}

	got, ok := internalOnlyFallbackProfile(monitors)
	if !ok {
		t.Fatal("expected fallback profile")
	}
	if len(got.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(got.Outputs))
	}

	output := got.Outputs[0]
	if !output.Enabled {
		t.Fatal("expected internal output to be enabled")
	}
	if output.Mode != "2880x1800@120.00Hz" || output.Width != 2880 || output.Height != 1800 || output.Refresh != 120 {
		t.Fatalf("unexpected fallback mode: %+v", output)
	}
	if output.X != 0 || output.Y != 0 || output.Scale != 1.5 || output.MirrorOf != "" {
		t.Fatalf("unexpected fallback placement: %+v", output)
	}
	if got.Workspaces.Enabled {
		t.Fatalf("expected fallback workspace settings to be disabled: %+v", got.Workspaces)
	}
}

func TestInternalOnlyFallbackProfileLeavesExternalOutputsDisabled(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "B1", Disabled: true},
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1", Disabled: true},
	}

	got, ok := internalOnlyFallbackProfile(monitors)
	if !ok {
		t.Fatal("expected fallback profile")
	}

	for _, output := range got.Outputs {
		if output.Name == "DP-1" && output.Enabled {
			t.Fatalf("expected external output to stay disabled: %+v", output)
		}
		if output.Name == "eDP-1" && !output.Enabled {
			t.Fatalf("expected internal output to be enabled: %+v", output)
		}
	}
}

func TestInternalOnlyFallbackProfileDoesNotOverrideEnabledOutput(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "B1"},
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1", Disabled: true},
	}

	if _, ok := internalOnlyFallbackProfile(monitors); ok {
		t.Fatal("did not expect fallback while an output is enabled")
	}
}

func TestInternalOnlyFallbackProfileRequiresInternalOutput(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "B1", Disabled: true},
	}

	if _, ok := internalOnlyFallbackProfile(monitors); ok {
		t.Fatal("did not expect fallback without an internal output")
	}
}

func TestManualOverrideLastsUntilMonitorSetChanges(t *testing.T) {
	svc := &Service{}
	svc.setManualOverride("desk-set", profile.Profile{Name: "desk"})

	chosen, ok := svc.manualOverride("desk-set")
	if !ok || chosen.Name != "desk" {
		t.Fatalf("expected the chosen profile to remain in force, got %+v (ok=%v)", chosen, ok)
	}
	if _, ok := svc.manualOverride("projector-set"); ok {
		t.Fatal("expected monitor hotplug to clear the manual profile override")
	}
	if _, ok := svc.manualOverride("desk-set"); ok {
		t.Fatal("expected cleared manual profile override to stay cleared")
	}
}

func TestClearManualOverride(t *testing.T) {
	svc := &Service{}
	svc.setManualOverride("desk-set", profile.Profile{Name: "desk"})
	svc.clearManualOverride()

	if _, ok := svc.manualOverride("desk-set"); ok {
		t.Fatal("expected explicit clear to remove manual profile override")
	}
}

func TestApplyBestUsesInternalFallbackWhenNoProfilesAndAllOutputsDisabled(t *testing.T) {
	env := newApplyBestTestEnv(t, `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":0,"y":0,"scale":1.5,"transform":0,"disabled":false,"mirrorOf":""}]`)

	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
	})
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest returned error: %v", err)
	}

	rendered := readMonitorsConf(t, env)
	for _, want := range []string{
		"output = desc:Framework Panel",
		"mode = 2880x1800@120.00",
		"position = 0x0",
		"scale = 1.5",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}

	logBytes, err := os.ReadFile(env.logPath)
	if err != nil {
		t.Fatalf("read hyprctl log: %v", err)
	}
	if !strings.Contains(string(logBytes), "reload") {
		t.Fatalf("expected applyBest to reload Hyprland, got:\n%s", logBytes)
	}
}

func TestApplyBestUsesSavedProfileBeforeInternalFallback(t *testing.T) {
	env := newApplyBestTestEnv(t, `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":100,"y":200,"scale":2,"transform":0,"disabled":false,"mirrorOf":""}]`)
	mon := hypr.Monitor{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1"}

	if err := env.store.Save(profile.New("saved-internal", []profile.OutputConfig{{
		Key:     mon.HardwareKey(),
		Name:    mon.Name,
		Enabled: true,
		Width:   2880,
		Height:  1800,
		Refresh: 120,
		X:       100,
		Y:       200,
		Scale:   2,
	}})); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
	})
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest returned error: %v", err)
	}

	rendered := readMonitorsConf(t, env)
	for _, want := range []string{
		"output = desc:Framework Panel",
		"mode = 2880x1800@120.00",
		"position = 100x200",
		"scale = 2",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}

func TestApplyBestDefersWhileDisplaysSleep(t *testing.T) {
	sleeping := `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":0,"y":0,"scale":1.5,"transform":0,"dpmsStatus":false,"disabled":false,"mirrorOf":""}]`
	env := newApplyBestTestEnvWithMonitors(t, sleeping, sleeping)

	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
	})
	err := svc.applyBest(context.Background())
	if !errors.Is(err, errDisplaysSleeping) {
		t.Fatalf("applyBest error = %v, want displays-sleeping sentinel", err)
	}

	rendered := readMonitorsConf(t, env)
	if !strings.Contains(rendered, "# initial") {
		t.Fatalf("sleeping apply changed monitor config:\n%s", rendered)
	}
	logBytes, err := os.ReadFile(env.logPath)
	if err != nil {
		t.Fatalf("read hyprctl log: %v", err)
	}
	if strings.Contains(string(logBytes), "reload") {
		t.Fatalf("sleeping apply reloaded Hyprland:\n%s", logBytes)
	}
}

func TestRunDefersTransientHotplugWhileDisplaysSleep(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, "runtime")
	socketDir := filepath.Join(runtimeDir, "hypr", "sig-test")
	if err := os.MkdirAll(socketDir, 0o755); err != nil {
		t.Fatalf("create socket directory: %v", err)
	}

	listener, err := net.Listen("unix", filepath.Join(socketDir, ".socket2.sock"))
	if err != nil {
		t.Fatalf("listen on fake socket2: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	monitorStatePath := filepath.Join(dir, "monitors.json")
	logPath := filepath.Join(dir, "hyprctl.log")
	monitorsConfPath := filepath.Join(dir, "monitors.conf")
	hyprlandConfigPath := filepath.Join(dir, "hyprland.conf")
	hyprctlPath := filepath.Join(dir, "hyprctl")
	fakeHyprctlScript := `#!/bin/bash
set -eu

if [[ "${1-}" == "--instance" ]]; then
  shift 2
fi

printf '%s\n' "$*" >> "$HYPRCTL_LOG"

if [[ "${1-}" == "-j" && "${2-}" == "version" ]]; then
  printf '{"version":"0.54.0"}'
  exit 0
fi
if [[ "${1-}" == "-j" && "${2-}" == "monitors" && "${3-}" == "all" ]]; then
  cat "$HYPRCTL_MONITORS"
  exit 0
fi
if [[ "${1-}" == "-j" && "${2-}" == "workspacerules" ]]; then
  printf '[]'
  exit 0
fi
if [[ "${1-}" == "-j" && "${2-}" == "workspaces" ]]; then
  printf '[]'
  exit 0
fi
if [[ "${1-}" == "reload" ]]; then
  exit 0
fi

echo "unexpected args: $*" >&2
exit 1
`

	if err := os.WriteFile(monitorsConfPath, []byte(config.GeneratedLegacyHeader+"\n# initial\n"), 0o644); err != nil {
		t.Fatalf("write monitors.conf: %v", err)
	}
	if err := os.WriteFile(hyprlandConfigPath, []byte("source = "+monitorsConfPath+"\n"), 0o644); err != nil {
		t.Fatalf("write hyprland.conf: %v", err)
	}
	if err := os.WriteFile(hyprctlPath, []byte(fakeHyprctlScript), 0o755); err != nil {
		t.Fatalf("write fake hyprctl: %v", err)
	}

	t.Setenv("HYPRCTL_LOG", logPath)
	t.Setenv("HYPRCTL_MONITORS", monitorStatePath)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	homeMonitors := []hypr.Monitor{
		{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true},
		{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "B1", Width: 3840, Height: 2160, RefreshRate: 60, X: 2880, Scale: 2, DPMSStatus: true},
	}
	laptopMonitors := []hypr.Monitor{homeMonitors[0]}
	if err := writeMonitorState(monitorStatePath, homeMonitors); err != nil {
		t.Fatalf("write initial monitor state: %v", err)
	}

	store := profile.NewStore(dir)
	if err := store.Save(profile.FromMonitors("Home", homeMonitors)); err != nil {
		t.Fatalf("save home profile: %v", err)
	}
	if err := store.Save(profile.FromMonitors("Laptop", laptopMonitors)); err != nil {
		t.Fatalf("save laptop profile: %v", err)
	}
	client, err := hypr.NewClient()
	if err != nil {
		t.Fatalf("new hypr client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	svc := New(client, store, Config{
		Debounce:        20 * time.Millisecond,
		WakeSettle:      80 * time.Millisecond,
		PollInterval:    time.Hour,
		LidPollInterval: time.Hour,
		EventRetry:      time.Hour,
		MonitorsConf:    monitorsConfPath,
		HyprConfig:      hyprlandConfigPath,
	})
	go func() { done <- svc.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case conn := <-accepted:
			_ = conn.Close()
		default:
		}
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("daemon returned error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("daemon did not stop")
		}
	}()

	var eventConn net.Conn
	select {
	case eventConn = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("daemon did not connect to fake socket2")
	}
	defer eventConn.Close()

	waitFor(t, time.Second, func() bool { return reloadCount(logPath) == 1 }, "startup profile apply")

	sleepingLaptop := append([]hypr.Monitor(nil), laptopMonitors...)
	sleepingLaptop[0].DPMSStatus = false
	if err := writeMonitorState(monitorStatePath, sleepingLaptop); err != nil {
		t.Fatalf("write sleeping monitor state: %v", err)
	}
	if _, err := fmt.Fprintln(eventConn, "monitorremoved>>DP-1"); err != nil {
		t.Fatalf("send sleeping removal event: %v", err)
	}
	time.Sleep(4 * svc.cfg.Debounce)
	if got := reloadCount(logPath); got != 1 {
		t.Fatalf("sleeping removal applied a profile: reload count = %d, want 1", got)
	}

	wakingHome := append([]hypr.Monitor(nil), homeMonitors...)
	wakingHome[1].Scale = 1
	if err := writeMonitorState(monitorStatePath, wakingHome); err != nil {
		t.Fatalf("write waking monitor state: %v", err)
	}
	if _, err := fmt.Fprintln(eventConn, "monitoradded>>DP-1"); err != nil {
		t.Fatalf("send wake event: %v", err)
	}
	time.Sleep(svc.cfg.WakeSettle / 2)
	if got := reloadCount(logPath); got != 1 {
		t.Fatalf("wake reconciled before settle period: reload count = %d, want 1", got)
	}
	waitFor(t, time.Second, func() bool { return reloadCount(logPath) == 2 }, "post-wake profile reconciliation")
}

type runTestEnv struct {
	svc              *Service
	logPath          string
	monitorStatePath string
	monitorsConfPath string
	logs             *logRecorder
	setLid           func(lid.State)
	lidStates        chan lid.State
	suspendEvents    chan bool
	stop             func()
}

type logRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *logRecorder) logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

func (r *logRecorder) contains(substring string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, line := range r.lines {
		if strings.Contains(line, substring) {
			return true
		}
	}
	return false
}

func (r *logRecorder) all() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

// newRunTestEnv starts a daemon whose lid and suspend sources are test
// channels, against a fake hyprctl serving monitor state from a file.
func newRunTestEnv(t *testing.T, monitors []hypr.Monitor) runTestEnv {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	logPath := filepath.Join(dir, "hyprctl.log")
	monitorStatePath := filepath.Join(dir, "monitors.json")
	monitorsConfPath := filepath.Join(dir, "monitors.conf")
	hyprlandConfigPath := filepath.Join(dir, "hyprland.conf")
	hyprctlPath := filepath.Join(dir, "hyprctl")
	fakeHyprctlScript := `#!/bin/bash
set -eu

if [[ "${1-}" == "--instance" ]]; then
  shift 2
fi

printf '%s\n' "$*" >> "$HYPRCTL_LOG"

if [[ "${1-}" == "-j" && "${2-}" == "version" ]]; then
  printf '{"version":"0.54.0"}'
  exit 0
fi
if [[ "${1-}" == "-j" && "${2-}" == "monitors" && "${3-}" == "all" ]]; then
  cat "$HYPRCTL_MONITORS"
  exit 0
fi
if [[ "${1-}" == "-j" && "${2-}" == "workspacerules" ]]; then
  printf '[]'
  exit 0
fi
if [[ "${1-}" == "-j" && "${2-}" == "workspaces" ]]; then
  printf '[]'
  exit 0
fi
if [[ "${1-}" == "reload" ]]; then
  if [[ -f "$HYPRCTL_MONITORS_NEXT" ]]; then
    mv "$HYPRCTL_MONITORS_NEXT" "$HYPRCTL_MONITORS"
  fi
  exit 0
fi
if [[ "${1-}" == "dispatch" ]]; then
  exit 0
fi

echo "unexpected args: $*" >&2
exit 1
`

	if err := os.WriteFile(monitorsConfPath, []byte(config.GeneratedLegacyHeader+"\n# initial\n"), 0o644); err != nil {
		t.Fatalf("write monitors.conf: %v", err)
	}
	if err := os.WriteFile(hyprlandConfigPath, []byte("source = "+monitorsConfPath+"\n"), 0o644); err != nil {
		t.Fatalf("write hyprland.conf: %v", err)
	}
	if err := os.WriteFile(hyprctlPath, []byte(fakeHyprctlScript), 0o755); err != nil {
		t.Fatalf("write fake hyprctl: %v", err)
	}
	if err := writeMonitorState(monitorStatePath, monitors); err != nil {
		t.Fatalf("write initial monitor state: %v", err)
	}

	t.Setenv("HYPRCTL_LOG", logPath)
	t.Setenv("HYPRCTL_MONITORS", monitorStatePath)
	t.Setenv("HYPRCTL_MONITORS_NEXT", monitorStatePath+".next")
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	store := profile.NewStore(dir)
	if err := store.Save(profile.FromMonitors("Home", monitors)); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	client, err := hypr.NewClient()
	if err != nil {
		t.Fatalf("new hypr client: %v", err)
	}

	logs := &logRecorder{}
	svc := New(client, store, Config{
		Debounce:        50 * time.Millisecond,
		WakeSettle:      80 * time.Millisecond,
		PollInterval:    time.Hour,
		LidPollInterval: time.Hour,
		EventRetry:      time.Hour,
		MonitorsConf:    monitorsConfPath,
		HyprConfig:      hyprlandConfigPath,
		Logf:            logs.logf,
	})

	var lidMu sync.Mutex
	lidNow := lid.Open
	lidStates := make(chan lid.State, 4)
	suspendEvents := make(chan bool, 4)
	svc.readLid = func(context.Context) (lid.State, error) {
		lidMu.Lock()
		defer lidMu.Unlock()
		return lidNow, nil
	}
	svc.watchLid = func(context.Context, time.Duration) (<-chan lid.State, <-chan error) {
		return lidStates, nil
	}
	svc.watchSuspend = func(context.Context) <-chan bool {
		return suspendEvents
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	stop := func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("daemon returned error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("daemon did not stop")
		}
	}

	return runTestEnv{
		svc:              svc,
		logPath:          logPath,
		monitorStatePath: monitorStatePath,
		monitorsConfPath: monitorsConfPath,
		logs:             logs,
		setLid: func(state lid.State) {
			lidMu.Lock()
			lidNow = state
			lidMu.Unlock()
		},
		lidStates:     lidStates,
		suspendEvents: suspendEvents,
		stop:          stop,
	}
}

func hyprctlLogContains(path string, substring string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substring)
}

// The user's laptop suspends when the lid closes. The pending lid:closed
// trigger must not survive into the resume: applying it there disables the
// internal panel right as the user opens the laptop, and nothing has told the
// displays to light up yet.
func TestRunDropsStaleLidCloseAcrossSuspendAndWakesDisplaysOnResume(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true},
		{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "B1", Width: 3840, Height: 2160, RefreshRate: 60, X: 2880, Scale: 2, DPMSStatus: true},
	}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()

	waitFor(t, time.Second, func() bool { return reloadCount(env.logPath) == 1 }, "startup profile apply")

	// The lid closes; before the debounce elapses, the machine suspends.
	env.setLid(lid.Closed)
	env.lidStates <- lid.Closed
	waitFor(t, time.Second, func() bool { return env.logs.contains("triggered: lid:closed") }, "lid close trigger")
	env.suspendEvents <- true
	waitFor(t, time.Second, func() bool { return env.logs.contains("suspending; dropped the pending trigger") }, "suspend drop")

	// The user opens the lid, which resumes the machine.
	env.setLid(lid.Open)
	env.suspendEvents <- false
	waitFor(t, time.Second, func() bool { return env.logs.contains("resumed from sleep; waking displays") }, "resume handling")
	waitFor(t, time.Second, func() bool { return hyprctlLogContains(env.logPath, "dispatch dpms on") }, "display wake dispatch")
	waitFor(t, time.Second, func() bool { return env.logs.contains("triggered: resume") }, "resume trigger")

	// Nothing changed while the machine slept, so the resume settles without a
	// reload, and the stale close never turned the panel off.
	time.Sleep(3 * env.svc.cfg.WakeSettle)
	if env.logs.contains("lid closed: forced internal outputs off") {
		t.Fatalf("stale lid close was applied after resume:\n%s", env.logs.all())
	}
	if got := reloadCount(env.logPath); got != 1 {
		t.Fatalf("resume reloaded Hyprland without a change: reload count = %d, want 1\nlogs:\n%s", got, env.logs.all())
	}
	rendered, err := os.ReadFile(env.monitorsConfPath)
	if err != nil {
		t.Fatalf("read monitors.conf: %v", err)
	}
	if strings.Contains(string(rendered), "disable") {
		t.Fatalf("resume left an output disabled:\n%s", rendered)
	}
}

// A lid close that does not suspend the machine is clamshell mode: the closed
// policy must still apply and turn the internal panel off.
func TestRunAppliesClosedLidPolicyWhenLidCloseDoesNotSuspend(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true},
		{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "B1", Width: 3840, Height: 2160, RefreshRate: 60, X: 2880, Scale: 2, DPMSStatus: true},
	}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()

	waitFor(t, time.Second, func() bool { return reloadCount(env.logPath) == 1 }, "startup profile apply")

	// The panel is still on when the lid closes; the reload the daemon issues
	// is what turns it off, so stage that state as the post-reload result.
	clamshell := append([]hypr.Monitor(nil), monitors...)
	clamshell[0].Disabled = true
	if err := writeMonitorState(env.monitorStatePath+".next", clamshell); err != nil {
		t.Fatalf("stage clamshell monitor state: %v", err)
	}
	env.setLid(lid.Closed)
	env.lidStates <- lid.Closed

	waitFor(t, time.Second, func() bool { return reloadCount(env.logPath) == 2 }, "closed-lid apply")
	waitFor(t, time.Second, func() bool { return env.logs.contains("lid closed: forced internal outputs off") }, "closed-lid policy")

	rendered, err := os.ReadFile(env.monitorsConfPath)
	if err != nil {
		t.Fatalf("read monitors.conf: %v", err)
	}
	if !strings.Contains(string(rendered), "disable") {
		t.Fatalf("closed-lid apply left the internal panel enabled:\n%s", rendered)
	}
}

func writeMonitorState(path string, monitors []hypr.Monitor) error {
	data, err := json.Marshal(monitors)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func reloadCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if line == "reload" {
			count++
		}
	}
	return count
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

type applyBestTestEnv struct {
	client             *hypr.Client
	store              *profile.Store
	logPath            string
	monitorsConfPath   string
	hyprlandConfigPath string
}

func newApplyBestTestEnv(t *testing.T, afterApplyMonitorsJSON string) applyBestTestEnv {
	t.Helper()
	beforeApplyMonitorsJSON := `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":3840,"y":0,"scale":1.5,"transform":0,"disabled":true,"mirrorOf":""}]`
	return newApplyBestTestEnvWithMonitors(t, beforeApplyMonitorsJSON, afterApplyMonitorsJSON)
}

func newApplyBestTestEnvWithMonitors(t *testing.T, beforeApplyMonitorsJSON, afterApplyMonitorsJSON string) applyBestTestEnv {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	logPath := filepath.Join(dir, "hyprctl.log")
	statePath := filepath.Join(dir, "applied")
	monitorsConfPath := filepath.Join(dir, "monitors.conf")
	hyprlandConfigPath := filepath.Join(dir, "hyprland.conf")
	hyprctlPath := filepath.Join(dir, "hyprctl")
	fakeHyprctlScript := `#!/bin/bash
set -eu

if [[ "${1-}" == "--instance" ]]; then
  shift 2
fi

printf '%s\n' "$*" >> "$HYPRCTL_LOG"

if [[ "${1-}" == "-j" && "${2-}" == "version" ]]; then
  printf '{"version":"0.54.0"}'
  exit 0
fi

if [[ "${1-}" == "-j" && "${2-}" == "monitors" && "${3-}" == "all" ]]; then
  if [[ -f "$HYPRCTL_STATE" ]]; then
    printf '%s' '` + afterApplyMonitorsJSON + `'
  else
    printf '%s' '` + beforeApplyMonitorsJSON + `'
  fi
  exit 0
fi

if [[ "${1-}" == "-j" && "${2-}" == "workspacerules" ]]; then
  printf '[]'
  exit 0
fi

if [[ "${1-}" == "-j" && "${2-}" == "workspaces" ]]; then
  printf '[]'
  exit 0
fi

if [[ "${1-}" == "reload" ]]; then
  touch "$HYPRCTL_STATE"
  exit 0
fi

echo "unexpected args: $*" >&2
exit 1
`

	if err := os.WriteFile(monitorsConfPath, []byte(config.GeneratedLegacyHeader+"\n# initial\n"), 0o644); err != nil {
		t.Fatalf("write monitors.conf: %v", err)
	}
	if err := os.WriteFile(hyprlandConfigPath, []byte("source = "+monitorsConfPath+"\n"), 0o644); err != nil {
		t.Fatalf("write hyprland.conf: %v", err)
	}
	if err := os.WriteFile(hyprctlPath, []byte(fakeHyprctlScript), 0o755); err != nil {
		t.Fatalf("write fake hyprctl: %v", err)
	}

	t.Setenv("HYPRCTL_LOG", logPath)
	t.Setenv("HYPRCTL_STATE", statePath)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	client, err := hypr.NewClient()
	if err != nil {
		t.Fatalf("new hypr client: %v", err)
	}

	return applyBestTestEnv{
		client:             client,
		store:              profile.NewStore(dir),
		logPath:            logPath,
		monitorsConfPath:   monitorsConfPath,
		hyprlandConfigPath: hyprlandConfigPath,
	}
}

func readMonitorsConf(t *testing.T, env applyBestTestEnv) string {
	t.Helper()

	rendered, err := os.ReadFile(env.monitorsConfPath)
	if err != nil {
		t.Fatalf("read monitors.conf: %v", err)
	}
	return string(rendered)
}

func TestApplyBestRestoresTheManuallyChosenProfileAfterAnExternalChange(t *testing.T) {
	// Something outside hyprmoncfg moved the panel: the monitor set is the same
	// hardware, but its scale and position are no longer what was applied.
	drifted := `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":0,"y":0,"scale":2,"transform":0,"disabled":false,"dpmsStatus":true,"mirrorOf":""}]`
	restored := `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":100,"y":200,"scale":1.5,"transform":0,"disabled":false,"dpmsStatus":true,"mirrorOf":""}]`
	env := newApplyBestTestEnvWithMonitors(t, drifted, restored)
	mon := hypr.Monitor{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1"}

	chosen := profile.New("hand-picked", []profile.OutputConfig{{
		Key:     mon.HardwareKey(),
		Name:    mon.Name,
		Enabled: true,
		Width:   2880,
		Height:  1800,
		Refresh: 120,
		X:       100,
		Y:       200,
		Scale:   1.5,
	}})
	decoy := profile.New("aaa-auto-match", []profile.OutputConfig{{
		Key:     mon.HardwareKey(),
		Name:    mon.Name,
		Enabled: true,
		Width:   2880,
		Height:  1800,
		Refresh: 120,
		X:       0,
		Y:       0,
		Scale:   3,
	}})
	for _, saved := range []profile.Profile{chosen, decoy} {
		if err := env.store.Save(saved); err != nil {
			t.Fatalf("save profile %q: %v", saved.Name, err)
		}
	}

	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
	})
	svc.setManualOverride(profile.MonitorSetHash([]hypr.Monitor{mon}), chosen)

	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest returned error: %v", err)
	}

	rendered := readMonitorsConf(t, env)
	for _, want := range []string{"position = 100x200", "scale = 1.5"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected the hand-picked profile to be restored, missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "scale = 3") {
		t.Fatalf("expected the manual choice to outrank automatic matching:\n%s", rendered)
	}
}

func TestApplyBestPersistsOmarchyDisplayPanelScale(t *testing.T) {
	// Omarchy's Display panel changed scale via hyprctl and left position=auto.
	// Keep the new scale, put the saved layout back.
	drifted := `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":0,"y":0,"scale":1.25,"transform":0,"disabled":false,"dpmsStatus":true,"mirrorOf":""}]`
	restored := `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":100,"y":200,"scale":1.25,"transform":0,"disabled":false,"dpmsStatus":true,"mirrorOf":""}]`
	env := newApplyBestTestEnvWithMonitors(t, drifted, restored)
	mon := hypr.Monitor{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1"}

	if err := os.MkdirAll(filepath.Join(os.Getenv("XDG_STATE_HOME"), "omarchy"), 0o755); err != nil {
		t.Fatalf("mkdir omarchy state: %v", err)
	}
	line := fmt.Sprintf("at=%s\trequested=1.25\tcurrent=1.5\tnew=1.25\tmonitor=eDP-1\n", time.Now().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "omarchy", "monitor-scaling.log"), []byte(line), 0o644); err != nil {
		t.Fatalf("write scaling log: %v", err)
	}

	chosen := profile.New("hand-picked", []profile.OutputConfig{{
		Key:     mon.HardwareKey(),
		Name:    mon.Name,
		Enabled: true,
		Width:   2880,
		Height:  1800,
		Refresh: 120,
		X:       100,
		Y:       200,
		Scale:   1.5,
	}})
	if err := env.store.Save(chosen); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	logs := &logRecorder{}
	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
		Logf:         logs.logf,
	})
	svc.setManualOverride(profile.MonitorSetHash([]hypr.Monitor{mon}), chosen)

	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest returned error: %v", err)
	}

	rendered := readMonitorsConf(t, env)
	for _, want := range []string{"position = 100x200", "scale = 1.25"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected saved layout with Omarchy scale, missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "scale = 1.5") {
		t.Fatalf("Omarchy display scale was reverted:\n%s", rendered)
	}
	if !logs.contains("persisted Omarchy display scale for eDP-1 into profile \"hand-picked\"") {
		t.Fatalf("expected persist log, got:\n%s", logs.all())
	}

	saved, err := env.store.Load("hand-picked")
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	got, ok := saved.OutputByKey(mon.HardwareKey())
	if !ok || got.Scale != 1.25 || got.X != 100 || got.Y != 200 {
		t.Fatalf("saved output = %+v", got)
	}
}

func TestApplyBestLeavesTheManualChoiceAloneWhenNothingMoved(t *testing.T) {
	monitorsJSON := `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":100,"y":200,"scale":1.5,"transform":0,"disabled":false,"dpmsStatus":true,"mirrorOf":""}]`
	env := newApplyBestTestEnvWithMonitors(t, monitorsJSON, monitorsJSON)
	mon := hypr.Monitor{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1"}

	chosen := profile.New("hand-picked", []profile.OutputConfig{{
		Key:     mon.HardwareKey(),
		Name:    mon.Name,
		Enabled: true,
		Width:   2880,
		Height:  1800,
		Refresh: 120,
		X:       100,
		Y:       200,
		Scale:   1.5,
	}})
	if err := env.store.Save(chosen); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
	})
	svc.setManualOverride(profile.MonitorSetHash([]hypr.Monitor{mon}), chosen)
	svc.applied = chosen.Name + "|" + profile.MonitorStateHash([]hypr.Monitor{{
		Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1",
		Width: 2880, Height: 1800, RefreshRate: 120, X: 100, Y: 200, Scale: 1.5,
	}}) + "|lid=" + string(svc.lidState)

	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest returned error: %v", err)
	}

	logBytes, err := os.ReadFile(env.logPath)
	if err != nil {
		t.Fatalf("read hyprctl log: %v", err)
	}
	if strings.Contains(string(logBytes), "reload") {
		t.Fatalf("expected no reload when the manual choice is already on screen, got:\n%s", logBytes)
	}
}

const applyBestDualBeforeJSON = `[{"id":0,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":0,"y":0,"scale":1.5,"transform":0,"disabled":false,"dpmsStatus":true,"mirrorOf":""},{"id":1,"name":"DP-1","description":"Dell U2720Q","make":"Dell","model":"U2720Q","serial":"B1","width":3840,"height":2160,"refreshRate":60,"x":1920,"y":0,"scale":2,"transform":0,"disabled":false,"dpmsStatus":true,"mirrorOf":""}]`

func applyBestDualMonitors() []hypr.Monitor {
	return []hypr.Monitor{
		{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true},
		{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "B1", Width: 3840, Height: 2160, RefreshRate: 60, X: 1920, Scale: 2, DPMSStatus: true},
	}
}

// The cached lid state says closed, but the lid is open by the time the apply
// runs; this is exactly the resume race. The fresh read must win.
func TestApplyBestRefreshesStaleClosedLidBeforeApplying(t *testing.T) {
	env := newApplyBestTestEnvWithMonitors(t, applyBestDualBeforeJSON, applyBestDualBeforeJSON)
	if err := env.store.Save(profile.FromMonitors("Home", applyBestDualMonitors())); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	logs := &logRecorder{}
	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
		Logf:         logs.logf,
	})
	svc.lidSupported = true
	svc.lidState = lid.Closed
	svc.readLid = func(context.Context) (lid.State, error) { return lid.Open, nil }

	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest returned error: %v", err)
	}

	if svc.lidState != lid.Open {
		t.Fatalf("lid state not refreshed: %s", svc.lidState)
	}
	if logs.contains("lid closed: forced internal outputs off") {
		t.Fatalf("stale closed lid state disabled the internal panel:\n%s", logs.all())
	}
	rendered := readMonitorsConf(t, env)
	if strings.Contains(rendered, "disable") {
		t.Fatalf("expected every output enabled with the lid open:\n%s", rendered)
	}
}

// The mirror image: the cache says open but the lid has just closed. The fresh
// read must apply the closed-lid policy without waiting for the watcher.
func TestApplyBestRefreshesStaleOpenLidBeforeApplying(t *testing.T) {
	closedAfterJSON := `[{"id":0,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":0,"y":0,"scale":1.5,"transform":0,"disabled":true,"dpmsStatus":true,"mirrorOf":""},{"id":1,"name":"DP-1","description":"Dell U2720Q","make":"Dell","model":"U2720Q","serial":"B1","width":3840,"height":2160,"refreshRate":60,"x":1920,"y":0,"scale":2,"transform":0,"disabled":false,"dpmsStatus":true,"mirrorOf":""}]`
	env := newApplyBestTestEnvWithMonitors(t, applyBestDualBeforeJSON, closedAfterJSON)
	if err := env.store.Save(profile.FromMonitors("Home", applyBestDualMonitors())); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	logs := &logRecorder{}
	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
		Logf:         logs.logf,
	})
	svc.lidSupported = true
	svc.lidState = lid.Open
	svc.readLid = func(context.Context) (lid.State, error) { return lid.Closed, nil }

	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest returned error: %v", err)
	}

	if !logs.contains("lid closed: forced internal outputs off") {
		t.Fatalf("closed-lid policy did not run:\n%s", logs.all())
	}
	rendered := readMonitorsConf(t, env)
	if !strings.Contains(rendered, "disable") {
		t.Fatalf("expected the internal panel disabled with the lid closed:\n%s", rendered)
	}
}
