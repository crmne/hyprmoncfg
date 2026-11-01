package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestRenamingTheAppliedProfileDoesNotReapplyOrRunItsHook(t *testing.T) {
	monitors := wakeMonitors()
	data, _ := json.Marshal(monitors)
	env := newApplyBestTestEnvWithMonitors(t, string(data), string(data))
	hookLog := filepath.Join(t.TempDir(), "hook.log")
	hook := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho ran >> "+hookLog+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	requested := profile.FromMonitors("Dock", monitors)
	requested.Exec = hook
	if err := env.store.Save(requested); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	t.Cleanup(func() { _ = svc.Shutdown() })
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	reloads := func() int {
		log, err := os.ReadFile(env.logPath)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Count(string(log), "\nreload\n")
	}
	hookRuns := func() int {
		log, err := os.ReadFile(hookLog)
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Count(string(log), "ran")
	}
	beforeReloads, beforeRuns := reloads(), hookRuns()

	if err := svc.Rename(ipc.RenameParams{Name: "Dock", NewName: "Studio"}); err != nil {
		t.Fatal(err)
	}
	if svc.lastProfile.Name != "Studio" || svc.applied == nil || svc.applied.requested.Name != "Studio" {
		t.Fatalf("references did not follow: last %q", svc.lastProfile.Name)
	}
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reloads(); got != beforeReloads {
		t.Fatalf("rename reloaded: %d, was %d", got, beforeReloads)
	}
	if got := hookRuns(); got != beforeRuns {
		t.Fatalf("rename ran the post-apply command: %d, was %d", got, beforeRuns)
	}
	document, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !document.Daemon.RenameSupported {
		t.Fatal("status does not advertise rename")
	}
	if document.ActiveProfile == nil || document.ActiveProfile.Name != "Studio" {
		t.Fatalf("active profile after rename: %+v", document.ActiveProfile)
	}
}

func TestRenameFollowsAManualChoice(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	p := profile.FromMonitors("Dock", wakeMonitors())
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	svc := &Service{store: store}
	svc.setManualOverride("set", p)
	if err := svc.Rename(ipc.RenameParams{Name: "Dock", NewName: "Studio"}); err != nil {
		t.Fatal(err)
	}
	manual, ok := svc.manualOverride("set")
	if !ok || manual.Name != "Studio" {
		t.Fatalf("manual choice %q %v", manual.Name, ok)
	}
}

func TestRenameWaitsForAnInteractivePreview(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	if err := store.Save(profile.FromMonitors("Dock", wakeMonitors())); err != nil {
		t.Fatal(err)
	}
	svc := &Service{store: store, pending: &pendingTransaction{}}
	if err := svc.Rename(ipc.RenameParams{Name: "Dock", NewName: "Studio"}); !errors.Is(err, errPreviewActive) {
		t.Fatalf("rename during preview: %v", err)
	}
	if _, err := store.Load("Dock"); err != nil {
		t.Fatalf("preview rename moved the profile: %v", err)
	}
}

func TestRenameRefusesTheForcedProfile(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	if err := store.Save(profile.FromMonitors("Dock", wakeMonitors())); err != nil {
		t.Fatal(err)
	}
	svc := &Service{store: store, cfg: Config{ForcedProfile: "dock"}}
	if err := svc.Rename(ipc.RenameParams{Name: "Dock", NewName: "Studio"}); err == nil || !strings.Contains(err.Error(), "--profile") {
		t.Fatalf("forced profile renamed: %v", err)
	}
}
