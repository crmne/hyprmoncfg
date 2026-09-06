package hypr

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func instanceFixture(t *testing.T, runtime, signature, display string, live bool) {
	t.Helper()
	dir := filepath.Join(runtime, "hypr", signature)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hyprland.lock"), []byte(fmt.Sprintf("%d\n%s\n", os.Getpid(), display)), 0o600); err != nil {
		t.Fatal(err)
	}
	if live {
		listener, err := net.Listen("unix", filepath.Join(dir, ".socket.sock"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = listener.Close() })
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				_ = conn.Close()
			}
		}()
	}
}

func TestDiscoveryRecoversFromMalformedInstancesJSON(t *testing.T) {
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	path := filepath.Join(t.TempDir(), "hyprctl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf ']\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := &Client{hyprctl: path}
	if _, err := client.InstanceSignature(context.Background()); err == nil || !strings.Contains(err.Error(), "no running") {
		t.Fatalf("empty session diagnostic: %v", err)
	}
	instanceFixture(t, runtime, "stale", "wayland-0", false)
	instanceFixture(t, runtime, "active", "wayland-1", true)
	if got, err := client.InstanceSignature(context.Background()); err != nil || got != "active" {
		t.Fatalf("did not find live socket: %q, %v", got, err)
	}
	instanceFixture(t, runtime, "other", "wayland-2", true)
	if _, err := client.InstanceSignature(context.Background()); err == nil {
		t.Fatal("guessed between two live sessions")
	}
	t.Setenv("WAYLAND_DISPLAY", "wayland-2")
	if got, err := client.InstanceSignature(context.Background()); err != nil || got != "other" {
		t.Fatalf("ignored selected Wayland session: %q, %v", got, err)
	}
	t.Setenv("WAYLAND_DISPLAY", "wayland-missing")
	if _, err := client.InstanceSignature(context.Background()); err == nil {
		t.Fatal("used a different user's session selection")
	}
}

func TestMonitorSubscriptionStopsWhileSocketIsIdle(t *testing.T) {
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "test")
	dir := filepath.Join(runtime, "hypr", "test")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(dir, ".socket2.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &Client{}
	events, failures := client.SubscribeMonitorEvents(ctx)
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("unexpected event")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription leaked while waiting for a monitor event")
	}
	if err := <-failures; err != nil {
		t.Fatalf("cancellation reported as an IPC failure: %v", err)
	}
}
