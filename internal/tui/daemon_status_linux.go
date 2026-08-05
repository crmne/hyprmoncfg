package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
)

const procRoot = "/proc"

func isDaemonRunning() bool {
	return daemonRunningIn(procRoot)
}

func daemonRunningIn(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}

		cmdline, err := os.ReadFile(filepath.Join(root, entry.Name(), "cmdline"))
		if err == nil && isDaemonCommand(cmdline) {
			return true
		}
	}

	return false
}

func isDaemonCommand(cmdline []byte) bool {
	// Wrappers can change the kernel process name while preserving argv[0].
	argv0, _, _ := bytes.Cut(cmdline, []byte{0})
	return filepath.Base(string(argv0)) == "hyprmoncfgd"
}
