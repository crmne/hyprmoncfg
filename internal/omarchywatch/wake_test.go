package omarchywatch

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

const numericDefaults = "-- My monitor defaults\nlocal omarchy_monitor_scale = 1.6\n" +
	"hl.monitor({ output = \"\", mode = \"preferred\", position = \"auto\", scale = omarchy_monitor_scale })\n"

func withFakeClamshell(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	writeWakeTestFile(t, filepath.Join(bin, clamshellCommand), "#!/bin/sh\nexit 0\n", 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	return bin
}

func wakeFixture(t *testing.T, content string) (*WakeConfig, profile.Profile, []hypr.Monitor) {
	t.Helper()
	withFakeClamshell(t)
	w := NewWakeConfig()
	writeWakeTestFile(t, w.monitorsPath, content, 0o640)
	monitors := []hypr.Monitor{{Name: "eDP-1", Make: "BOE", Model: "Panel", Scale: 1.33333, X: 4096, Y: 576, Width: 1920, Height: 1080, RefreshRate: 60}}
	return w, profile.FromMonitors("Laptop", monitors), monitors
}

func writeWakeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readWakeTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestWakeConfigUsesOmarchyPathsAndPreservesDefaults(t *testing.T) {
	w, p, monitors := wakeFixture(t, numericDefaults)
	if w.monitorsPath != filepath.Join(os.Getenv("HOME"), ".config/hypr/monitors.lua") ||
		w.scalePath != filepath.Join(os.Getenv("HOME"), ".local/state/omarchy/toggles/hypr/internal-monitor-scale") {
		t.Fatalf("not Omarchy's paths: %+v", w)
	}
	snapshot, err := w.Sync(p, monitors)
	if err != nil {
		t.Fatal(err)
	}
	content := readWakeTestFile(t, w.monitorsPath)
	block, body, err := splitWakeBlock(content)
	if err != nil || body != numericDefaults || !strings.Contains(block, `position = "4096x576", scale = 1.33333`) {
		t.Fatalf("wrong bridge or modified defaults: %s (%v)", content, err)
	}
	if readWakeTestFile(t, w.scalePath) != "1.33333\n" {
		t.Fatal("wrong remembered scale")
	}
	before, _ := os.Stat(w.monitorsPath)
	same, err := w.Sync(p, monitors)
	if err != nil || same.monitorsPath != "" || same.scale.Path != "" {
		t.Fatal("identical apply should write nothing")
	}
	after, _ := os.Stat(w.monitorsPath)
	if !os.SameFile(before, after) || after.Mode().Perm() != 0o640 {
		t.Fatal("rewrote a settled file or changed permissions")
	}
	// A user edits another rule while the preview is open.
	edit := "-- A new user rule\n"
	writeWakeTestFile(t, w.monitorsPath, content+edit, 0o640)
	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}
	if got := readWakeTestFile(t, w.monitorsPath); got != numericDefaults+edit {
		t.Fatalf("rollback lost the user's edit: %s", got)
	}
	if _, err := os.Stat(w.scalePath); !os.IsNotExist(err) {
		t.Fatal("rollback left a new scale behind")
	}
}

func TestWakeConfigPreviewRestoresThePreviousProfileAndReleaseRemovesOnlyOurBlock(t *testing.T) {
	w, p, monitors := wakeFixture(t, numericDefaults)
	if _, err := w.Sync(p, monitors); err != nil {
		t.Fatal(err)
	}
	previous := readWakeTestFile(t, w.monitorsPath)
	p.Outputs[0].Scale, p.Outputs[0].X = 1.5, -1920
	snapshot, err := w.Sync(p, monitors)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readWakeTestFile(t, w.monitorsPath), `position = "-1920x576", scale = 1.5`) {
		t.Fatal("preview did not update wake geometry")
	}
	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}
	if readWakeTestFile(t, w.monitorsPath) != previous || readWakeTestFile(t, w.scalePath) != "1.33333\n" {
		t.Fatal("preview did not restore both wake settings")
	}
	for range 2 {
		if err := w.Release(); err != nil {
			t.Fatal(err)
		}
	}
	if readWakeTestFile(t, w.monitorsPath) != numericDefaults {
		t.Fatal("unmanage changed user defaults")
	}
}

func TestWakeConfigPreservesSymlinksAndRefusesReadOnlyOrEditedBlocks(t *testing.T) {
	for _, kind := range []string{"symlink", "read-only", "edited block", "broken markers"} {
		t.Run(kind, func(t *testing.T) {
			w, p, monitors := wakeFixture(t, numericDefaults)
			switch kind {
			case "symlink":
				target := filepath.Join(t.TempDir(), "monitors.lua")
				if err := os.Rename(w.monitorsPath, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, w.monitorsPath); err != nil {
					t.Fatal(err)
				}
				if _, err := w.Sync(p, monitors); err != nil {
					t.Fatal(err)
				}
				if err := w.Release(); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Readlink(w.monitorsPath); err != nil || readWakeTestFile(t, target) != numericDefaults {
					t.Fatal("damaged dotfile symlink")
				}
			case "read-only":
				if err := os.Chmod(w.monitorsPath, 0o444); err != nil {
					t.Fatal(err)
				}
				if _, err := w.Sync(p, monitors); err == nil {
					t.Fatal("did not report read-only config")
				}
				if readWakeTestFile(t, w.monitorsPath) != numericDefaults {
					t.Fatal("changed read-only defaults")
				}
			case "edited block":
				snapshot, err := w.Sync(p, monitors)
				if err != nil {
					t.Fatal(err)
				}
				edited := strings.Replace(readWakeTestFile(t, w.monitorsPath), "scale = 1.33333", "scale = 2", 1)
				writeWakeTestFile(t, w.monitorsPath, edited, 0o640)
				if err := snapshot.Restore(); err == nil || readWakeTestFile(t, w.monitorsPath) != edited {
					t.Fatal("overwrote an edited wake block")
				}
			case "broken markers":
				edited := wakeStart + numericDefaults
				writeWakeTestFile(t, w.monitorsPath, edited, 0o640)
				if _, err := w.Sync(p, monitors); err == nil {
					t.Fatal("accepted unterminated block")
				}
				if err := w.Release(); err == nil || readWakeTestFile(t, w.monitorsPath) != edited {
					t.Fatal("removed user content after a broken marker")
				}
			}
		})
	}
}

func TestWakeConfigRollsBackIfRememberingScaleFails(t *testing.T) {
	w, p, monitors := wakeFixture(t, numericDefaults)
	w.scalePath = w.monitorsPath + "/cannot-write-through-a-file"
	if _, err := w.Sync(p, monitors); err == nil {
		t.Fatal("expected scale write failure")
	}
	if readWakeTestFile(t, w.monitorsPath) != numericDefaults {
		t.Fatal("failed sync left its block behind")
	}
}

func TestWakeConfigPreservesLuaByteOrderMark(t *testing.T) {
	w, p, monitors := wakeFixture(t, "\ufeff"+numericDefaults)
	snapshot, err := w.Sync(p, monitors)
	if err != nil {
		t.Fatal(err)
	}
	if got := readWakeTestFile(t, w.monitorsPath); !strings.HasPrefix(got, "\ufeff"+wakeStart) || strings.Count(got, "\ufeff") != 1 {
		t.Fatal("moved the Lua byte-order mark into the file body")
	}
	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}
	if readWakeTestFile(t, w.monitorsPath) != "\ufeff"+numericDefaults {
		t.Fatal("rollback changed the file encoding")
	}
}

func TestWakeConfigSkipsNonOmarchyAndLegacyConfigs(t *testing.T) {
	w, p, monitors := wakeFixture(t, config.GeneratedLuaHeader+"\n-- old layout\n")
	before := readWakeTestFile(t, w.monitorsPath)
	if _, err := w.Sync(p, monitors); err != nil || readWakeTestFile(t, w.monitorsPath) != before {
		t.Fatal("changed the old generated config before retirement")
	}
	if err := os.Remove(w.monitorsPath); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Sync(p, monitors); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.monitorsPath); !os.IsNotExist(err) {
		t.Fatal("created a Lua config on a legacy installation")
	}
	t.Setenv("PATH", t.TempDir())
	w = NewWakeConfig()
	if w != nil {
		t.Fatal("enabled Omarchy integration on another desktop")
	}
	if _, err := w.Sync(p, monitors); err != nil {
		t.Fatal(err)
	}
	if err := w.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestWakeConfigResolvesDisabledPanelsAndRejectsUnsafeOrAmbiguousValues(t *testing.T) {
	w, p, monitors := wakeFixture(t, numericDefaults)
	p.Outputs[0].Enabled = false
	p.Outputs[0].Name = "old-connector"
	if _, err := w.Sync(p, monitors); err != nil || !strings.Contains(readWakeTestFile(t, w.monitorsPath), `output = "eDP-1"`) {
		t.Fatal("did not resolve a disabled panel by hardware identity")
	}
	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		p.Outputs[0].Scale = scale
		if _, ok := internalOutput(p, monitors); ok {
			t.Fatalf("accepted invalid scale %v", scale)
		}
	}
	p.Outputs[0].Scale = 1
	p.Outputs = append(p.Outputs, p.Outputs[0])
	if _, ok := internalOutput(p, monitors); ok {
		t.Fatal("chose between ambiguous internal panels")
	}
	p.Outputs = p.Outputs[:1]
	p.Outputs[0].Name = "eDP-1\"); os.execute(\"bad\")"
	if _, ok := internalOutput(p, nil); ok {
		t.Fatal("accepted an unsafe connector")
	}
}

// Run the unmodified upstream script with a synthetic desktop. No command can
// reach the real compositor, lid state, or user's configuration.
func TestStockOmarchyWakeDoesNotResetAnAppliedProfile(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("Omarchy's script requires jq")
	}
	for _, defaults := range []string{numericDefaults, strings.Replace(numericDefaults, "= 1.6", "= \"auto\"", 1),
		"hl.monitor({ output = \"eDP-1\", position = \"0x0\", scale = 2 })\n" + numericDefaults} {
		t.Run(defaults, func(t *testing.T) {
			w, p, monitors := wakeFixture(t, defaults)
			bin := t.TempDir()
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			for _, command := range []string{"omarchy-hyprland-monitor-internal", "omarchy-hyprland-monitor-internal-mirror", "omarchy-hw-clamshell", "omarchy-hyprland-monitor-external-active"} {
				writeWakeTestFile(t, filepath.Join(bin, command), "#!/bin/sh\nexit 1\n", 0o755)
			}
			writeWakeTestFile(t, filepath.Join(bin, "omarchy-hyprland-monitor-laptop"), "#!/bin/sh\necho eDP-1\n", 0o755)
			log := filepath.Join(t.TempDir(), "hyprctl.log")
			t.Setenv("WAKE_TEST_LOG", log)
			t.Setenv("WAKE_TEST_DISABLED", "false")
			writeWakeTestFile(t, filepath.Join(bin, "hyprctl"), `#!/bin/sh
case "$*" in
  'monitors all -j') printf '[{"name":"eDP-1","scale":1.3333334,"disabled":%s}]' "$WAKE_TEST_DISABLED" ;;
  *) printf '%s\n' "$*" >> "$WAKE_TEST_LOG" ;;
esac
`, 0o755)
			runWake := func() string {
				t.Helper()
				writeWakeTestFile(t, log, "", 0o600)
				if output, err := exec.Command("bash", "testdata/omarchy-clamshell.sh").CombinedOutput(); err != nil {
					t.Fatalf("stock wake script: %v\n%s", err, output)
				}
				return readWakeTestFile(t, log)
			}
			// First reproduce #58 with the old state-file-only mitigation.
			writeWakeTestFile(t, w.scalePath, "1.33333\n", 0o644)
			before := runWake()
			if !strings.Contains(defaults, `= "auto"`) && !strings.Contains(before, "eval hl.monitor") {
				t.Fatal("fixture did not reproduce the reported reset")
			}
			if _, err := w.Sync(p, monitors); err != nil {
				t.Fatal(err)
			}
			for range 2 { // lock and unlock
				if got := runWake(); got != "" {
					t.Fatalf("wake reset an already-correct panel: %s", got)
				}
			}
			t.Setenv("WAKE_TEST_DISABLED", "true")
			if got := runWake(); !strings.Contains(got, `position = "4096x576", scale = 1.33333`) {
				t.Fatalf("lid recovery lost saved geometry: %s", got)
			}
		})
	}
}
