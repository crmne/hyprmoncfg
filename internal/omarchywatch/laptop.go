package omarchywatch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// LaptopToggle shares Omarchy's existing manual-disable flag. Its wake path
// honors this flag, and its menu and keybinding change it. Sharing that choice
// prevents two monitor managers from repeatedly undoing each other.
type LaptopToggle struct {
	path     string
	mu       sync.Mutex
	known    bool
	disabled bool
}

func NewLaptopToggle() *LaptopToggle {
	if _, err := exec.LookPath(clamshellCommand); err != nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	// Omarchy's shell commands use HOME here, even with XDG_STATE_HOME set.
	return &LaptopToggle{path: filepath.Join(home, ".local/state/omarchy/toggles/hypr/internal-monitor-disable.lua")}
}

func (l *LaptopToggle) Changed() (disabled, changed bool, err error) {
	if l == nil {
		return false, false, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = os.Stat(l.path)
	if err != nil && !os.IsNotExist(err) {
		return false, false, err
	}
	disabled = err == nil
	return disabled, l.known && disabled != l.disabled, nil
}

// Reset discards changes made by Omarchy's recovery path during sleep, lid, or
// external-monitor events. They are not a request to rewrite a saved profile.
func (l *LaptopToggle) Reset() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.known = false
	l.mu.Unlock()
}

func (l *LaptopToggle) Sync(p profile.Profile, monitors []hypr.Monitor) (config.FileSnapshot, error) {
	if l == nil {
		return config.FileSnapshot{}, nil
	}
	internal := ""
	disabled := true
	externalEnabled := false
	resolver := profile.NewMonitorResolver(monitors)
	for _, monitor := range monitors {
		if monitor.IsInternal() {
			if internal != "" {
				return config.FileSnapshot{}, nil
			}
			internal = monitor.Name
		}
	}
	if internal == "" {
		return config.FileSnapshot{}, nil
	}
	for _, output := range p.Outputs {
		monitor, ok := resolver.ResolveOutput(output)
		if !ok {
			continue
		}
		if monitor.Name == internal {
			disabled = !output.Enabled
		} else if output.Enabled && output.MirrorOf == "" {
			externalEnabled = true
		}
	}
	// Omarchy's recover path clears the flag when the laptop stands alone.
	disabled = disabled && externalEnabled
	l.mu.Lock()
	defer l.mu.Unlock()
	snapshot, err := config.SnapshotFile(l.path)
	if err != nil {
		return snapshot, err
	}
	if disabled {
		content := fmt.Sprintf("hl.monitor({ output = %s, disabled = true })\n", strconv.Quote(internal))
		if string(snapshot.Content) != content {
			if err = os.MkdirAll(filepath.Dir(l.path), 0o755); err == nil {
				err = config.WriteFileAtomic(l.path, []byte(content), 0o644)
			}
		}
	} else {
		err = os.Remove(l.path)
		if os.IsNotExist(err) {
			err = nil
		}
	}
	if err == nil {
		l.known, l.disabled = true, disabled
	}
	return snapshot, err
}

func (l *LaptopToggle) Restore(snapshot config.FileSnapshot) error {
	if l == nil || snapshot.Path == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := snapshot.Restore(); err != nil {
		return err
	}
	l.known, l.disabled = true, snapshot.Exists
	return nil
}
