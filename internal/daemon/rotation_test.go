package daemon

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/orientation"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestAutoRotationFollowsTheSensorOnlyInTabletMode(t *testing.T) {
	r := newAutoRotation("", t.Logf)
	if _, changed := r.observe(orientation.Reading{Available: true, Orientation: "left-up"}); !changed || r.turn() != 1 {
		t.Fatalf("turn = %d, want 1 on a device without a tablet-mode switch", r.turn())
	}
	if r.observeTabletMode(false, nil); r.turn() != 0 {
		t.Fatalf("a convertible folded as a laptop turned to %d", r.turn())
	}
	if status := r.status(); !status.Paused || status.TabletMode {
		t.Fatalf("status = %+v, want paused outside tablet mode", status)
	}
	if !r.observeTabletMode(true, nil) || r.turn() != 1 {
		t.Fatalf("tablet mode turn = %d, want 1", r.turn())
	}
	if _, changed := r.observe(orientation.Reading{Available: true, Orientation: "undefined"}); changed || r.turn() != 1 {
		t.Fatalf("lying flat changed the turn to %d", r.turn())
	}
	r.observeTabletMode(false, errors.New("permission denied"))
	if r.turn() != 1 {
		t.Fatal("an unreadable switch stopped rotation")
	}
	r.observeTabletMode(false, lid.ErrNoTabletModeSwitch)
	if r.turn() != 1 {
		t.Fatal("a device without a switch stopped rotating")
	}
	if _, changed := r.observe(orientation.Reading{}); !changed || r.turn() != 0 {
		t.Fatalf("a lost sensor kept the turn %d", r.turn())
	}
}

func TestRotationLockHoldsTheTurnAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	r := newAutoRotation(dir, t.Logf)
	r.observe(orientation.Reading{Available: true, Orientation: "right-up"})
	if _, err := r.set(boolPtr(true), nil); err != nil {
		t.Fatal(err)
	}
	if _, changed := r.observe(orientation.Reading{Available: true, Orientation: "normal"}); changed || r.turn() != 3 {
		t.Fatalf("the lock let the turn change to %d", r.turn())
	}
	restarted := newAutoRotation(dir, t.Logf)
	restarted.observe(orientation.Reading{Available: true, Orientation: "normal"})
	if status := restarted.status(); !status.Locked || status.Quarters != 3 {
		t.Fatalf("after a restart = %+v, want locked at 3", status)
	}
	if changed, err := restarted.set(boolPtr(false), nil); err != nil || !changed || restarted.turn() != 0 {
		t.Fatalf("unlock = %v, %v, turn %d; want the current orientation", changed, err, restarted.turn())
	}
}

// The tablet turns sideways: its panel gets transform 1, the monitor beside
// it moves to stay touching, and touch input turns with the panel.
func TestRunTurnsARotatingPanelWithTheSensor(t *testing.T) {
	panel := hypr.Monitor{Name: "eDP-1", Description: "BOE 0x0BCA", Make: "BOE", Model: "0x0BCA",
		Width: 1920, Height: 1200, RefreshRate: 60, Scale: 1, DPMSStatus: true, AvailableModes: []string{"1920x1200@60.00Hz"}}
	external := hypr.Monitor{Name: "DP-1", Description: "Dell U2720Q ABC", Make: "Dell", Model: "U2720Q", Serial: "ABC",
		Width: 2560, Height: 1440, RefreshRate: 60, X: 1920, Scale: 1, DPMSStatus: true, AvailableModes: []string{"2560x1440@60.00Hz"}}
	readings := make(chan orientation.Reading, 4)
	env := newRunTestEnvConfigured(t, []hypr.Monitor{panel, external}, func(cfg *Config) {
		cfg.WatchOrientation = func(context.Context) <-chan orientation.Reading { return readings }
	}, func(s *Service) {
		p, err := s.store.Load("Home")
		if err != nil {
			t.Fatal(err)
		}
		p.Outputs[0].RotationAuto = true
		if err := s.store.Save(p); err != nil {
			t.Fatal(err)
		}
	})
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	waitFor(t, 2*time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	turnedPanel, movedExternal := panel, external
	turnedPanel.Transform, movedExternal.X = 1, 1200
	if err := writeMonitorState(env.monitorStatePath+".next", []hypr.Monitor{turnedPanel, movedExternal}); err != nil {
		t.Fatal(err)
	}
	readings <- orientation.Reading{Available: true, Orientation: "left-up"}
	waitFor(t, 3*time.Second, func() bool {
		data, _ := os.ReadFile(env.monitorsConfPath)
		return strings.Contains(string(data), "output = desc:BOE 0x0BCA\n  mode = 1920x1200@60.00\n  position = 0x0\n  scale = 1\n  transform = 1")
	}, "panel turned")
	data, _ := os.ReadFile(env.monitorsConfPath)
	if !strings.Contains(string(data), "output = desc:Dell U2720Q ABC\n  mode = 2560x1440@60.00\n  position = 1200x0") || !strings.Contains(string(data), "touchdevice {\n    transform = 1") {
		t.Fatalf("generated config:\n%s", data)
	}
	waitFor(t, 3*time.Second, func() bool { return strings.Count(env.logs.all(), "applied profile: Home") >= 2 }, "turned layout applied")
	if saved, err := env.svc.store.Load("Home"); err != nil || saved.Outputs[0].Transform != 0 || saved.Outputs[1].X != 1920 {
		t.Fatalf("the saved profile changed: %+v, %v", saved.Outputs, err)
	}
	if status, err := env.svc.Status(); err != nil || status.Rotation == nil || status.Rotation.Quarters != 1 || status.ActiveProfile == nil {
		t.Fatalf("status = %+v, %v; want turned once and the profile still active", status.Rotation, err)
	}
}

func TestEditorStateOfATurnedDeviceIsUpright(t *testing.T) {
	panel := hypr.Monitor{Name: "eDP-1", Description: "BOE 0x0BCA", Make: "BOE", Model: "0x0BCA",
		Width: 1920, Height: 1200, RefreshRate: 60, Scale: 1, DPMSStatus: true, Transform: 1}
	env := newRunTestEnvConfigured(t, []hypr.Monitor{panel}, func(cfg *Config) {
		cfg.WatchOrientation = func(context.Context) <-chan orientation.Reading { return nil }
	}, func(s *Service) {
		p := profile.FromMonitors("Home", []hypr.Monitor{panel})
		p.Outputs[0].Transform, p.Outputs[0].RotationAuto = 0, true
		if err := s.store.Save(p); err != nil {
			t.Fatal(err)
		}
		s.rotation.observe(orientation.Reading{Available: true, Orientation: "left-up"})
	})
	defer env.stop()
	editor, err := env.svc.EditorState()
	if err != nil {
		t.Fatal(err)
	}
	if editor.SourceProfile != "Home" || editor.Profile.Outputs[0].Transform != 0 || !editor.Profile.Outputs[0].RotationAuto {
		t.Fatalf("editor = %q %+v, want the upright saved layout", editor.SourceProfile, editor.Profile.Outputs[0])
	}
	if !editor.Displays[0].RotationAutoSupported {
		t.Fatal("the built-in panel should offer automatic rotation")
	}
}

// Keeping a preview on a turned tablet pins the upright profile, so the next
// apply turns it once, not twice.
func TestKeptPreviewOnATurnedTabletIsTurnedOnce(t *testing.T) {
	panel := hypr.Monitor{Name: "eDP-1", Description: "BOE 0x0BCA", Make: "BOE", Model: "0x0BCA",
		Width: 1920, Height: 1200, RefreshRate: 60, Scale: 1, DPMSStatus: true, AvailableModes: []string{"1920x1200@60.00Hz"}}
	env := newRunTestEnvConfigured(t, []hypr.Monitor{panel}, func(cfg *Config) {
		cfg.WatchOrientation = func(context.Context) <-chan orientation.Reading { return nil }
	}, func(s *Service) {
		p, err := s.store.Load("Home")
		if err != nil {
			t.Fatal(err)
		}
		p.Outputs[0].RotationAuto = true
		if err := s.store.Save(p); err != nil {
			t.Fatal(err)
		}
		s.rotation.observe(orientation.Reading{Available: true, Orientation: "left-up"})
		turned := panel
		turned.Transform = 1
		if err := writeMonitorState(os.Getenv("HYPRCTL_MONITORS")+".next", []hypr.Monitor{turned}); err != nil {
			t.Fatal(err)
		}
	})
	defer env.stop()
	waitFor(t, 3*time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	transaction, err := env.svc.Preview("test-panel", ipc.PreviewParams{ProfileName: "Home", TimeoutSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	if transaction.Profile.Outputs[0].Transform != 1 {
		t.Fatalf("preview transform = %d, want the turned 1", transaction.Profile.Outputs[0].Transform)
	}
	if err := env.svc.Confirm("test-panel", ipc.TransactionParams{TransactionID: transaction.ID}); err != nil {
		t.Fatal(err)
	}
	if err := env.svc.applyAutomatic(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(env.monitorsConfPath)
	if !strings.Contains(string(data), "transform = 1") || strings.Contains(string(data), "transform = 2") {
		t.Fatalf("the pinned layout was turned again:\n%s", data)
	}
}

func boolPtr(value bool) *bool { return &value }

func TestReversedRotationTurnsTheOtherWayAndPersists(t *testing.T) {
	dir := t.TempDir()
	r := newAutoRotation(dir, t.Logf)
	r.observe(orientation.Reading{Available: true, Orientation: "left-up"})
	if changed, err := r.set(nil, boolPtr(true)); err != nil || !changed || r.turn() != 3 {
		t.Fatalf("reversed left-up = %d (changed %v, %v), want 3", r.turn(), changed, err)
	}
	r.observe(orientation.Reading{Available: true, Orientation: "bottom-up"})
	if r.turn() != 2 {
		t.Fatalf("reversed bottom-up = %d, want 2", r.turn())
	}
	restarted := newAutoRotation(dir, t.Logf)
	restarted.observe(orientation.Reading{Available: true, Orientation: "right-up"})
	if status := restarted.status(); !status.Reversed || status.Quarters != 1 {
		t.Fatalf("after a restart = %+v, want reversed right-up turned once", status)
	}
}
