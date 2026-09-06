package daemon

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestPreviewOwnershipAndSafetyRollback(t *testing.T) {
	for _, action := range []string{"timeout", "shutdown", "unmanage", "late commit"} {
		t.Run(action, func(t *testing.T) {
			const state = `[{"name":"eDP-1","make":"Framework","model":"Panel","serial":"A1","width":1920,"height":1080,"refreshRate":60,"scale":1,"dpmsStatus":true}]`
			env := newApplyBestTestEnvWithMonitors(t, state, state)
			svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
			before, err := os.ReadFile(env.monitorsConfPath)
			if err != nil {
				t.Fatal(err)
			}
			monitors, err := env.client.Monitors(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			target := profile.FromMonitors("Laptop", monitors)
			transaction, err := svc.Preview("tui", ipc.PreviewParams{Profile: &target, TimeoutSeconds: 10})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Shutdown() })
			document, err := svc.Status()
			if err != nil {
				t.Fatal(err)
			}
			if document.Daemon.Preview == nil || document.Daemon.Preview.Reclaimable {
				t.Fatal("live TUI preview is not an orphan")
			}
			if err := svc.Confirm("guard", ipc.TransactionParams{TransactionID: transaction.ID}); err == nil {
				t.Fatal("another client stole the preview")
			}
			svc.Disconnect("tui")
			document, err = svc.Status()
			if err != nil {
				t.Fatal(err)
			}
			if !document.Daemon.Preview.Reclaimable {
				t.Fatal("replacement client cannot discover the orphan")
			}
			if _, err := svc.ownedPending("replacement", transaction.ID); err != nil {
				t.Fatal(err)
			}
			switch action {
			case "timeout":
				svc.expirePreview(transaction.ID)
			case "shutdown":
				err = svc.Shutdown()
			case "unmanage":
				err = svc.Unmanage()
			case "late commit":
				svc.pending.deadline = time.Now().Add(-time.Second)
				if err := svc.Confirm("replacement", ipc.TransactionParams{TransactionID: transaction.ID}); err == nil {
					t.Fatal("kept an expired preview")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if svc.pending != nil {
				t.Fatal("preview survived its safety rollback")
			}
			if got, err := os.ReadFile(env.monitorsConfPath); err != nil || string(got) != string(before) {
				t.Fatalf("previous config not restored: %s (%v)", got, err)
			}
		})
	}
}
