package hypr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoveryQueriesAreBounded(t *testing.T) {
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "test")
	path := filepath.Join(t.TempDir(), "hyprctl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 10\n"), 0700); err != nil {
		t.Fatal(err)
	}
	c := &Client{hyprctl: path}
	queries := map[string]func() error{
		"monitors":   func() error { _, e := c.Monitors(context.Background()); return e },
		"workspaces": func() error { _, e := c.Workspaces(context.Background()); return e },
		"rules":      func() error { _, e := c.WorkspaceRules(context.Background()); return e },
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			if err := query(); err == nil {
				t.Fatal("expected timeout")
			}
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Fatalf("query stalled for %s", elapsed)
			}
		})
	}
}
