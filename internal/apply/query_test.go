package apply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLuaProbeReadTimeoutRestoresConfig(t *testing.T) {
	initial := []byte("-- previous generated layout\n")
	engine, target, logPath := initLuaProbeTestEngine(t, "ok", initial)
	engine.QueryTimeout = 40 * time.Millisecond
	helper := filepath.Join(filepath.Dir(logPath), "hyprctl")
	source, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	blocked := strings.Replace(string(source), `  printf '%s' "$HYPRMONCFG_TEST_EVAL_REPLY"`, "  exec sleep 20", 1)
	if err := os.WriteFile(helper, []byte(blocked), 0755); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = engine.Apply(context.Background(), newTestProfile(), monitors, ApplyModeInteractive)
	if !errors.Is(err, ErrQueryTimeout) || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("Lua verification was not bounded: elapsed=%s error=%v", time.Since(started), err)
	}
	if restored, err := os.ReadFile(target); err != nil || string(restored) != string(initial) {
		t.Fatalf("timed-out Lua read did not restore config: %s (%v)", restored, err)
	}
}
