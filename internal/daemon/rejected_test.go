package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestFirstSetupPreservesLiveModeRotationAndPlacement(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "HDMI-A-1", Make: "LG", Model: "TV", Width: 3840, Height: 2160, RefreshRate: 60, X: 1080, Scale: 1, DPMSStatus: true, VRR: 1, CurrentFormat: "XBGR8888", AvailableModes: []string{"4096x2160@119.88Hz", "3840x2160@60Hz"}},
		{Name: "DP-3", Make: "Dell", Model: "Portrait", Width: 1920, Height: 1080, RefreshRate: 60, Y: 107, Scale: 1, Transform: 1, DPMSStatus: true},
	}
	data, _ := json.Marshal(monitors)
	env := newApplyBestTestEnvWithMonitors(t, string(data), string(data))
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(svc.applied.requested.Outputs, profile.FromMonitors("draft", monitors).Outputs) {
		t.Fatalf("startup changed working displays: %+v", svc.applied.requested.Outputs)
	}
	before := reloadCount(env.logPath)
	for range 3 {
		if err := svc.applyBest(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if reloadCount(env.logPath) != before {
		t.Fatal("unchanged unsaved setup kept applying")
	}
}

func TestLiveDraftStillEnablesModelessNewDisplay(t *testing.T) {
	working := hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, Transform: 1, DPMSStatus: true}
	newDisplay := hypr.Monitor{Name: "HDMI-A-1", Disabled: true, AvailableModes: []string{"1920x1080@60Hz"}}
	p := liveDraft([]hypr.Monitor{working, newDisplay}, nil)
	out, _ := p.OutputByKey(working.HardwareKey())
	if out.Transform != 1 || out.Width != 1920 {
		t.Fatalf("working display changed: %+v", out)
	}
	added, _ := p.OutputByKey(newDisplay.HardwareKey())
	if !added.Enabled || added.Width != 1920 || added.X != 1080 {
		t.Fatalf("new display was not extended: %+v", added)
	}
}

func TestRejectedAutomaticLayoutStopsRetriesAndAcceptsEditedProfile(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "HDMI-A-1", Make: "LG", Model: "TV", Width: 3840, Height: 2160, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
	env := newRunTestEnv(t, monitors, func(s *Service) {
		s.cfg.RecoveryInterval = 10 * time.Millisecond
		s.cfg.RecoveryMaxInterval = 20 * time.Millisecond
		s.cfg.PollInterval = 10 * time.Millisecond
		p := profile.FromMonitors("Home", monitors)
		p.Outputs[0].Width, p.Outputs[0].Mode = 4096, "4096x2160@119.88Hz"
		if err := s.store.Save(p); err != nil {
			t.Fatal(err)
		}
	})
	defer env.stop()
	waitFor(t, 6*time.Second, func() bool { return env.logs.contains("automatic retry paused:") }, "rejected mode guard")
	count := reloadCount(env.logPath)
	if count != 2 {
		t.Fatalf("wanted one apply and one rollback, got %d", count)
	}
	time.Sleep(150 * time.Millisecond)
	if reloadCount(env.logPath) != count {
		t.Fatalf("rejected configuration kept reloading: %s", env.logs.all())
	}
	if err := env.svc.applyAutomatic(context.Background()); !errors.Is(err, errAutomaticApplyRejected) {
		t.Fatalf("repeated event was not blocked: %v", err)
	}
	if reloadCount(env.logPath) != count {
		t.Fatal("explicit reconciliation repeated rejected apply")
	}
	if err := env.svc.store.Save(profile.FromMonitors("Home", monitors)); err != nil {
		t.Fatal(err)
	}
	if err := env.svc.applyAutomatic(context.Background()); err != nil {
		t.Fatalf("corrected profile blocked: %v", err)
	}
	if reloadCount(env.logPath) != count+1 {
		t.Fatal("corrected profile was not applied")
	}
}

func TestRejectedGuardDoesNotBlockModelessRecoveryOrChangedState(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "DP-1", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
	p := profile.FromMonitors("Desk", monitors)
	r := rememberRejected(p, monitors, nil)
	if !r.matches(p, monitors, nil) {
		t.Fatal("guard did not match")
	}
	p.CreatedAt, p.UpdatedAt = time.Now(), time.Now()
	if !r.matches(p, monitors, nil) {
		t.Fatal("regenerated draft timestamp restarted rejected apply")
	}
	monitors[0].VRR = 1
	monitors[0].CurrentFormat = "XBGR2101010"
	if !r.matches(p, monitors, nil) {
		t.Fatal("live VRR or buffer format restarted rejected apply")
	}
	monitors[0].Width = 0
	if r.matches(p, monitors, nil) {
		t.Fatal("guard blocked modeless recovery")
	}
	monitors[0].Width = 1280
	if r.matches(p, monitors, nil) {
		t.Fatal("guard blocked changed live state")
	}
	monitors[0].Width = 1920
	p.Outputs[0].Refresh = 30
	if r.matches(p, monitors, nil) {
		t.Fatal("guard blocked changed request")
	}
}

func TestRejectedApplyRetainsVerificationError(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "DP-1", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
	data, _ := json.Marshal(monitors)
	env := newApplyBestTestEnvWithMonitors(t, string(data), string(data))
	p := profile.FromMonitors("Desk", monitors)
	p.Outputs[0].Width, p.Outputs[0].Mode = 3840, "3840x1080@60Hz"
	if err := env.store.Save(p); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	err := svc.applyAutomatic(context.Background())
	if !errors.Is(err, apply.ErrVerificationFailed) || !errors.Is(err, errAutomaticApplyRejected) {
		t.Fatalf("lost failure cause: %v", err)
	}
	contents, _ := os.ReadFile(env.monitorsConfPath)
	if strings.Contains(string(contents), "3840x1080") {
		t.Fatal("rejected config was not rolled back")
	}
}
