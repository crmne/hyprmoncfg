package apply

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func enableWakeEngine(t *testing.T, lua bool, outcome string) (*Engine, string, []hypr.Monitor) {
	t.Helper()
	var engine *Engine
	var logPath string
	if lua {
		e, _, log := initLuaProbeTestEngine(t, "ok", []byte("-- original\nhl.monitor(\"eDP-1,disable\")\n"))
		engine, logPath = &e, log
	} else {
		var err error
		engine, logPath, err = initTestEngine(t)
		if err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Dir(logPath)
	// Both displays report asleep after reload. Only the previously disabled
	// eDP-1 is eligible for a wake; DP-1's deliberate sleep must be preserved.
	after := append([]hypr.Monitor(nil), monitors...)
	data, _ := json.Marshal(after)
	state := filepath.Join(dir, "wake-state.json")
	if err := os.WriteFile(state, data, 0600); err != nil {
		t.Fatal(err)
	}
	after[1].DPMSStatus = true
	data, _ = json.Marshal(after)
	awake := filepath.Join(dir, "wake-awake.json")
	if err := os.WriteFile(awake, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WAKE_STATE", state)
	t.Setenv("WAKE_AWAKE", awake)
	t.Setenv("WAKE_OUTCOME", outcome)
	scriptPath := filepath.Join(dir, "hyprctl")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	marker := "printf '%s\\n' \"$*\" >> \"$HYPRCTL_LOG\""
	injected := marker + `
if [[ "$cmd1" == "-j" && "$cmd2" == "monitors" ]]; then
 cat "$WAKE_STATE"; exit 0
fi
if [[ "$cmd1" == "--batch" && "$cmd2" == *dpms* ]]; then
 if [[ "$WAKE_OUTCOME" == "fail" ]]; then exit 1; fi
 if [[ "$WAKE_OUTCOME" == "wake" ]]; then cp "$WAKE_AWAKE" "$WAKE_STATE"; fi
 exit 0
fi
`
	if !strings.Contains(string(script), marker) {
		t.Fatal("missing fake command log")
	}
	if err := os.WriteFile(scriptPath, []byte(strings.Replace(string(script), marker, injected, 1)), 0755); err != nil {
		t.Fatal(err)
	}
	before := append([]hypr.Monitor(nil), monitors...)
	before[1].Disabled = true
	return engine, logPath, before
}

func TestApplyWakesOnlyNewlyEnabledDisplayAndReverts(t *testing.T) {
	for _, lua := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "lua"}[lua], func(t *testing.T) {
			engine, logPath, before := enableWakeEngine(t, lua, "wake")
			state, err := engine.Apply(context.Background(), newTestProfile(), before, ApplyModeInteractive)
			if err != nil {
				t.Fatal(err)
			}
			log, _ := os.ReadFile(logPath)
			want := "dispatch dpms on eDP-1"
			if lua {
				want = `dispatch hl.dsp.dpms({ action = "enable", monitor = "eDP-1" })`
			}
			if !strings.Contains(string(log), want) {
				t.Fatalf("missing targeted wake: %s", log)
			}
			after, err := engine.Client.Monitors(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if after[0].DPMSStatus || !after[1].DPMSStatus {
				t.Fatal("wake changed the wrong display")
			}
			if err := engine.Revert(context.Background(), state); err != nil {
				t.Fatal(err)
			}
			log, _ = os.ReadFile(logPath)
			if !lua && !strings.Contains(string(log), "eDP-1,disable") {
				t.Fatal("rollback did not disable newly enabled output")
			}
			if lua {
				restored, err := os.ReadFile(state.MonitorsConf.Path)
				if err != nil || string(restored) != string(state.MonitorsConf.Content) {
					t.Fatal("rollback did not restore disabled output config")
				}
			}
		})
	}
}

func TestApplyDoesNotWakeAlreadyEnabledSleepingDisplays(t *testing.T) {
	engine, logPath, before := enableWakeEngine(t, false, "wake")
	before[1].Disabled = false
	if _, err := engine.Apply(context.Background(), newTestProfile(), before, ApplyModeInteractive); err != nil {
		t.Fatal(err)
	}
	log, _ := os.ReadFile(logPath)
	if strings.Contains(string(log), "dpms") {
		t.Fatalf("unrelated edit woke sleeping displays: %s", log)
	}
}

func TestApplyRejectsAndRollsBackAnEnableThatNeverWakes(t *testing.T) {
	for _, outcome := range []string{"ignore", "fail"} {
		t.Run(outcome, func(t *testing.T) {
			engine, logPath, before := enableWakeEngine(t, false, outcome)
			original, err := os.ReadFile(engine.MonitorsConfPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = engine.Apply(context.Background(), newTestProfile(), before, ApplyModeInteractive)
			if !errors.Is(err, ErrVerificationFailed) || !strings.Contains(err.Error(), "remains asleep") {
				t.Fatalf("false successful enable: %v", err)
			}
			restored, _ := os.ReadFile(engine.MonitorsConfPath)
			if string(restored) != string(original) {
				t.Fatal("failed wake left generated config changed")
			}
			log, _ := os.ReadFile(logPath)
			if got := strings.Count(string(log), "dispatch dpms on eDP-1"); got != 3 {
				t.Fatalf("wake attempts=%d, want bounded 3", got)
			}
			if !strings.Contains(string(log), "eDP-1,disable") {
				t.Fatal("failed wake did not restore disabled output")
			}
		})
	}
}

// Do not dispatch to a missing or still-disabled connector: a Lua dispatcher
// with an unmatched selector may fall back to all monitors.
func TestEnableWakeWaitsForPresentEnabledTarget(t *testing.T) {
	engine, logPath, before := enableWakeEngine(t, true, "wake")
	for _, after := range [][]hypr.Monitor{before[:1], before} {
		check := engine.enableWakeCheck(newTestProfile(), before, true)
		if err := check(context.Background(), after); err == nil {
			t.Fatal("unavailable target counted as awake")
		}
	}
	log, _ := os.ReadFile(logPath)
	if strings.Contains(string(log), "dpms") {
		t.Fatal("sent wake to unavailable target")
	}
}
