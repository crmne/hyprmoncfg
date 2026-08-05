package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsDaemonCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmdline string
		want    bool
	}{
		{
			name:    "bare executable",
			cmdline: "hyprmoncfgd\x00--quiet\x00",
			want:    true,
		},
		{
			name:    "installed executable",
			cmdline: "/usr/bin/hyprmoncfgd\x00",
			want:    true,
		},
		{
			name:    "Nix wrapper argv zero",
			cmdline: "/nix/store/hash-hyprmoncfg/bin/hyprmoncfgd\x00",
			want:    true,
		},
		{
			name:    "Nix wrapped executable name",
			cmdline: "/nix/store/hash-hyprmoncfg/bin/.hyprmoncfgd-wrapped\x00",
			want:    false,
		},
		{
			name:    "layout editor",
			cmdline: "/usr/bin/hyprmoncfg\x00",
			want:    false,
		},
		{
			name: "empty command line",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isDaemonCommand([]byte(test.cmdline)); got != test.want {
				t.Fatalf("isDaemonCommand(%q) = %v, want %v", test.cmdline, got, test.want)
			}
		})
	}
}

func TestDaemonRunningIn(t *testing.T) {
	root := t.TempDir()
	writeCmdline(t, root, "100", "/usr/bin/hyprmoncfg\x00")
	writeCmdline(t, root, "101", "/nix/store/hash-hyprmoncfg/bin/hyprmoncfgd\x00")

	if !daemonRunningIn(root) {
		t.Fatal("expected to find the daemon by argv[0]")
	}
}

func TestDaemonRunningInReturnsFalseWithoutDaemon(t *testing.T) {
	root := t.TempDir()
	writeCmdline(t, root, "100", "/usr/bin/hyprmoncfg\x00")

	if daemonRunningIn(root) {
		t.Fatal("did not expect to find a daemon")
	}
}

func writeCmdline(t *testing.T, root, pid, cmdline string) {
	t.Helper()

	processDir := filepath.Join(root, pid)
	if err := os.Mkdir(processDir, 0o755); err != nil {
		t.Fatalf("create process directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(processDir, "cmdline"), []byte(cmdline), 0o644); err != nil {
		t.Fatalf("write command line: %v", err)
	}
}
