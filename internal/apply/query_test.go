package apply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLuaProbeReadTimeoutRestoresConfig(t *testing.T) {
	shortenApplyValidation(t, 300*time.Millisecond)
	initial := []byte("-- previous generated layout\n")
	engine, target, logPath := initLuaProbeTestEngine(t, "ok", initial)
	engine.QueryTimeout = 40 * time.Millisecond
	stallAfterReload(t, logPath, "eval", 1000)
	started := time.Now()
	_, err := engine.Apply(context.Background(), newTestProfile(), monitors, ApplyModeInteractive)
	if !errors.Is(err, ErrQueryTimeout) || time.Since(started) > 2*time.Second {
		t.Fatalf("Lua verification was not bounded: elapsed=%s error=%v", time.Since(started), err)
	}
	if restored, err := os.ReadFile(target); err != nil || string(restored) != string(initial) {
		t.Fatalf("timed-out Lua read did not restore config: %s (%v)", restored, err)
	}
}

// Hyprland stalls reads while it removes or adds a head during the reload
// (issue #72). A slow answer inside the validation window is not a failure,
// and rolling back would re-add the head and start the next apply.
func TestSlowReadsAfterReloadDoNotRollBack(t *testing.T) {
	for _, command := range []string{"eval", "monitors"} {
		t.Run(command, func(t *testing.T) {
			initial := []byte("-- previous generated layout\n")
			engine, target, logPath := initLuaProbeTestEngine(t, "ok", initial)
			engine.QueryTimeout = 40 * time.Millisecond
			stallAfterReload(t, logPath, command, 2)

			if _, err := engine.Apply(context.Background(), newTestProfile(), monitors, ApplyModeNonInteractive); err != nil {
				t.Fatalf("apply after slow %s reads: %v", command, err)
			}
			written, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(written) == string(initial) || !strings.Contains(string(written), "hl.monitor({") {
				t.Fatalf("slow %s reads rolled back the applied layout:\n%s", command, written)
			}
			logBytes, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if reloads := strings.Count(string(logBytes), "\nreload\n"); reloads != 1 {
				t.Fatalf("expected one reload and no rollback, got %d:\n%s", reloads, logBytes)
			}
		})
	}
}

// An answered mismatch is a definitive rejection. A compositor that then
// stops answering must not turn it back into a busy, retryable timeout.
func TestQueryTimeoutAfterMismatchKeepsVerificationFailure(t *testing.T) {
	shortenApplyValidation(t, 400*time.Millisecond)
	engine, _, logPath := initLuaProbeTestEngine(t, "ok", []byte("-- previous generated layout\n"))
	engine.QueryTimeout = 40 * time.Millisecond
	dir := filepath.Dir(logPath)
	helper := filepath.Join(dir, "hyprctl")
	source, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := filepath.Join(dir, "reloaded")
	counter := filepath.Join(dir, "reads")
	reloadBranch := `if [ "$cmd1" = "reload" ]; then` + "\n"
	monitorsBranch := `if [ "$cmd1" = "-j" ] && [ "$cmd2" = "monitors" ] && [ "$cmd3" = "all" ]; then` + "\n"
	// Answer the first read after the reload, then stall every later one.
	stall := fmt.Sprintf(`if [ -e %[1]q ]; then
  n=$(cat %[2]q 2>/dev/null || echo 0)
  echo $((n + 1)) > %[2]q
  if [ "$n" -ge 1 ]; then
    exec sleep 20
  fi
fi
`, reloaded, counter)
	script := string(source)
	if !strings.Contains(script, reloadBranch) || !strings.Contains(script, monitorsBranch) {
		t.Fatalf("fake hyprctl no longer has the expected branches:\n%s", script)
	}
	script = strings.Replace(script, reloadBranch, reloadBranch+fmt.Sprintf("  touch %q\n", reloaded), 1)
	script = strings.Replace(script, monitorsBranch, monitorsBranch+stall, 1)
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	p := newTestProfile()
	p.Outputs[1].X = 4000 // The fake compositor keeps reporting x=3840.
	_, err = engine.Apply(context.Background(), p, monitors, ApplyModeNonInteractive)
	if !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("a later timeout hid the verification failure: %v", err)
	}
}

func shortenApplyValidation(t *testing.T, timeout time.Duration) {
	t.Helper()
	previous, previousGrace := applyValidationTimeout, applyValidationGrace
	applyValidationTimeout, applyValidationGrace = timeout, timeout
	t.Cleanup(func() { applyValidationTimeout, applyValidationGrace = previous, previousGrace })
}

// A panel Hyprland is still connecting can report its preferred mode, then
// stall reads while it switches to the requested one (#118). The apply must
// wait for the answer instead of rolling back a change that is about to land.
func TestMismatchThenStallWaitsForTheModeset(t *testing.T) {
	shortenApplyValidation(t, 400*time.Millisecond)
	applyValidationGrace = time.Second
	initial := []byte("-- previous generated layout\n")
	engine, target, logPath := initLuaProbeTestEngine(t, "ok", initial)
	engine.QueryTimeout = 40 * time.Millisecond
	dir := filepath.Dir(logPath)
	helper := filepath.Join(dir, "hyprctl")
	source, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := filepath.Join(dir, "reloaded")
	counter := filepath.Join(dir, "reads")
	reloadBranch := `if [ "$cmd1" = "reload" ]; then` + "\n"
	monitorsBranch := `if [ "$cmd1" = "-j" ] && [ "$cmd2" = "monitors" ] && [ "$cmd3" = "all" ]; then` + "\n"
	// After the reload: the panel at its preferred 240 Hz, then reads stall
	// until after the 400 ms window, then the requested 120 Hz.
	transient := fmt.Sprintf(`if [ -e %[1]q ]; then
  n=$(cat %[2]q 2>/dev/null || echo 0)
  echo $((n + 1)) > %[2]q
  if [ "$n" -eq 0 ]; then
    printf '%%s' '[{"id":1,"name":"DP-1","description":"Microstep MPG321UR-QD","make":"Microstep","model":"MPG321UR-QD","serial":"A1","width":3840,"height":2160,"refreshRate":143.99,"x":0,"y":0,"scale":1,"transform":0,"focused":true,"dpmsStatus":true,"vrr":true,"disabled":false,"mirrorOf":"","activeWorkspace":{"id":1,"name":"1"}},{"id":2,"name":"eDP-1","description":"Samsung Display Corp. ATNA60CL10-0","make":"Samsung Display Corp.","model":"ATNA60CL10-0","serial":"B2","width":2880,"height":1800,"refreshRate":240,"x":3840,"y":0,"scale":1,"transform":0,"focused":false,"dpmsStatus":true,"vrr":false,"disabled":false,"mirrorOf":"","activeWorkspace":{"id":2,"name":"2"}}]'
    exit 0
  fi
  if [ "$n" -le 4 ]; then
    exec sleep 20
  fi
fi
`, reloaded, counter)
	script := string(source)
	if !strings.Contains(script, reloadBranch) || !strings.Contains(script, monitorsBranch) {
		t.Fatalf("fake hyprctl no longer has the expected branches:\n%s", script)
	}
	script = strings.Replace(script, reloadBranch, reloadBranch+fmt.Sprintf("  touch %q\n", reloaded), 1)
	script = strings.Replace(script, monitorsBranch, monitorsBranch+transient, 1)
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := engine.Apply(context.Background(), newTestProfile(), monitors, ApplyModeNonInteractive); err != nil {
		t.Fatalf("a mismatch read mid-modeset failed the apply: %v", err)
	}
	if written, err := os.ReadFile(target); err != nil || string(written) == string(initial) {
		t.Fatalf("the apply was rolled back: %s (%v)", written, err)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloads := strings.Count(string(logBytes), "\nreload\n"); reloads != 1 {
		t.Fatalf("expected one reload and no rollback, got %d:\n%s", reloads, logBytes)
	}
}

// stallAfterReload makes the fake hyprctl hang on the first stalls calls of
// command made after the first reload, like Hyprland mid-way through one.
func stallAfterReload(t *testing.T, logPath string, command string, stalls int) {
	t.Helper()
	dir := filepath.Dir(logPath)
	helper := filepath.Join(dir, "hyprctl")
	source, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	script := string(source)
	reloaded := filepath.Join(dir, "reloaded")
	counter := filepath.Join(dir, "stalls")
	stall := fmt.Sprintf(`if [ -e %[1]q ]; then
  n=$(cat %[2]q 2>/dev/null || echo 0)
  if [ "$n" -lt %[3]d ]; then
    echo $((n + 1)) > %[2]q
    exec sleep 20
  fi
fi
`, reloaded, counter, stalls)
	reloadBranch := `if [ "$cmd1" = "reload" ]; then` + "\n"
	var branch string
	switch command {
	case "eval":
		branch = `if [ "$cmd1" = "eval" ]; then` + "\n"
	case "monitors":
		branch = `if [ "$cmd1" = "-j" ] && [ "$cmd2" = "monitors" ] && [ "$cmd3" = "all" ]; then` + "\n"
	default:
		t.Fatalf("unsupported stalled command %q", command)
	}
	if !strings.Contains(script, reloadBranch) || !strings.Contains(script, branch) {
		t.Fatalf("fake hyprctl no longer has the expected branches:\n%s", script)
	}
	script = strings.Replace(script, reloadBranch, reloadBranch+fmt.Sprintf("  touch %q\n", reloaded), 1)
	script = strings.Replace(script, branch, branch+stall, 1)
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
