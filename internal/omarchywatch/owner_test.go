package omarchywatch

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMatchingUnitsSelectsOnlyActiveExactWatcherScopes(t *testing.T) {
	data := []byte(`[
  {"unit":"app-Hyprland-omarchy\\x2dmonitor\\x2dwatch.scope","active":"active","description":"omarchy-hyprland-monitor-watch"},
  {"unit":"inactive.scope","active":"inactive","description":"omarchy-hyprland-monitor-watch"},
  {"unit":"lookalike.scope","active":"active","description":"omarchy-hyprland-monitor-watch-extra"},
  {"unit":"watcher.service","active":"active","description":"omarchy-hyprland-monitor-watch"}
]`)

	got, err := matchingUnits(data)
	if err != nil {
		t.Fatalf("matchingUnits: %v", err)
	}
	want := []string{`app-Hyprland-omarchy\x2dmonitor\x2dwatch.scope`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matchingUnits = %v, want %v", got, want)
	}
}

func TestMatchingUnitsRejectsInvalidJSON(t *testing.T) {
	if _, err := matchingUnits([]byte(`not-json`)); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestOwnerStopsWatcherScopeAndRestartsItOnRelease(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "watcher-active")
	logPath := filepath.Join(dir, "commands.log")
	systemctlPath := writeExecutable(t, dir, "systemctl", `#!/bin/sh
set -eu
printf 'systemctl %s\n' "$*" >> "$TEST_COMMAND_LOG"
case "$*" in
  *list-units*)
    if [ -f "$TEST_WATCHER_STATE" ]; then
      printf '[{"unit":"app-omarchy.scope","active":"active","description":"omarchy-hyprland-monitor-watch"}]'
    else
      printf '[]'
    fi
    ;;
  *stop*) rm -f "$TEST_WATCHER_STATE" ;;
esac
`)
	launcherPath := writeExecutable(t, dir, "uwsm-app", `#!/bin/sh
set -eu
printf 'launcher %s\n' "$*" >> "$TEST_COMMAND_LOG"
touch "$TEST_WATCHER_STATE"
`)
	watcherPath := writeExecutable(t, dir, watcherCommand, "#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(statePath, nil, 0o644); err != nil {
		t.Fatalf("create watcher state: %v", err)
	}
	t.Setenv("TEST_COMMAND_LOG", logPath)
	t.Setenv("TEST_WATCHER_STATE", statePath)

	owner := &Owner{
		systemctl:    systemctlPath,
		launcher:     launcherPath,
		watcher:      watcherPath,
		pollInterval: time.Hour,
		logf:         func(string, ...any) {},
		sessionAlive: func() bool { return true },
	}
	ctx, cancel := context.WithCancel(context.Background())
	owner.Start(ctx)
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("watcher state still exists after Start: %v", err)
	}

	cancel()
	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer releaseCancel()
	if err := owner.Release(releaseCtx); err != nil {
		t.Fatalf("Release: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(statePath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watcher was not restarted")
		}
		time.Sleep(10 * time.Millisecond)
	}

	commands, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	if !strings.Contains(string(commands), "systemctl --user stop app-omarchy.scope") {
		t.Fatalf("watcher scope was not stopped:\n%s", commands)
	}
	if !strings.Contains(string(commands), "launcher -- "+watcherPath) {
		t.Fatalf("watcher was not launched:\n%s", commands)
	}
}

func TestOwnerDoesNotStartWatcherItDidNotStop(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "commands.log")
	systemctlPath := writeExecutable(t, dir, "systemctl", `#!/bin/sh
set -eu
printf '[]'
`)
	launcherPath := writeExecutable(t, dir, "uwsm-app", `#!/bin/sh
printf 'launched\n' >> "$TEST_COMMAND_LOG"
`)
	watcherPath := writeExecutable(t, dir, watcherCommand, "#!/bin/sh\nexit 0\n")
	t.Setenv("TEST_COMMAND_LOG", logPath)

	owner := &Owner{
		systemctl:    systemctlPath,
		launcher:     launcherPath,
		watcher:      watcherPath,
		pollInterval: time.Hour,
		logf:         func(string, ...any) {},
		sessionAlive: func() bool { return true },
	}
	ctx, cancel := context.WithCancel(context.Background())
	owner.Start(ctx)
	cancel()
	if err := owner.Release(context.Background()); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("unexpected launcher invocation: %v", err)
	}
}

func TestOwnerSuppressesWatcherStartedAfterDaemon(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "watcher-active")
	systemctlPath := writeExecutable(t, dir, "systemctl", `#!/bin/sh
set -eu
case "$*" in
  *list-units*)
    if [ -f "$TEST_WATCHER_STATE" ]; then
      printf '[{"unit":"app-omarchy.scope","active":"active","description":"omarchy-hyprland-monitor-watch"}]'
    else
      printf '[]'
    fi
    ;;
  *stop*) rm -f "$TEST_WATCHER_STATE" ;;
esac
`)
	launcherPath := writeExecutable(t, dir, "uwsm-app", "#!/bin/sh\nexit 0\n")
	watcherPath := writeExecutable(t, dir, watcherCommand, "#!/bin/sh\nexit 0\n")
	t.Setenv("TEST_WATCHER_STATE", statePath)

	owner := &Owner{
		systemctl:    systemctlPath,
		launcher:     launcherPath,
		watcher:      watcherPath,
		pollInterval: 10 * time.Millisecond,
		logf:         func(string, ...any) {},
		sessionAlive: func() bool { return false },
	}
	ctx, cancel := context.WithCancel(context.Background())
	owner.Start(ctx)
	// Repeating manage and ending its request must not leak or stop the owner.
	cancel()
	owner.Start(context.Background())
	if err := os.WriteFile(statePath, nil, 0o644); err != nil {
		t.Fatalf("start late watcher: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(statePath); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("late watcher was not suppressed")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	if err := owner.Release(context.Background()); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestOwnerDoesNotRestartWatcherAfterHyprlandStops(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "watcher-active")
	logPath := filepath.Join(dir, "commands.log")
	systemctlPath := writeExecutable(t, dir, "systemctl", `#!/bin/sh
set -eu
case "$*" in
  *list-units*)
    if [ -f "$TEST_WATCHER_STATE" ]; then
      printf '[{"unit":"app-omarchy.scope","active":"active","description":"omarchy-hyprland-monitor-watch"}]'
    else
      printf '[]'
    fi
    ;;
  *stop*) rm -f "$TEST_WATCHER_STATE" ;;
esac
`)
	launcherPath := writeExecutable(t, dir, "uwsm-app", `#!/bin/sh
printf 'launched\n' >> "$TEST_COMMAND_LOG"
`)
	watcherPath := writeExecutable(t, dir, watcherCommand, "#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(statePath, nil, 0o644); err != nil {
		t.Fatalf("create watcher state: %v", err)
	}
	t.Setenv("TEST_COMMAND_LOG", logPath)
	t.Setenv("TEST_WATCHER_STATE", statePath)

	owner := &Owner{
		systemctl:    systemctlPath,
		launcher:     launcherPath,
		watcher:      watcherPath,
		pollInterval: time.Hour,
		logf:         func(string, ...any) {},
		sessionAlive: func() bool { return false },
	}
	ctx, cancel := context.WithCancel(context.Background())
	owner.Start(ctx)
	cancel()
	if err := owner.Release(context.Background()); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("unexpected launcher invocation: %v", err)
	}
}

func writeExecutable(t *testing.T, dir string, name string, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}
