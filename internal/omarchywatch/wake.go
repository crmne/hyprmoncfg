package omarchywatch

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

const wakeStart = "-- BEGIN hyprmoncfg wake settings\n"
const wakeEnd = "-- END hyprmoncfg wake settings\n"

const clamshellCommand = "omarchy-hyprland-monitor-clamshell"
const togglesStateDir = "omarchy/toggles/hypr"
const internalScaleFile = "internal-monitor-scale"

var connectorName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// WakeConfig shares the active laptop geometry with Omarchy's wake script.
// That script reads the first connector-specific, single-line rule in
// monitors.lua; it does not follow our generated include. Keep a small owned
// block ahead of the user's rules, leaving their defaults intact for unmanage.
type WakeConfig struct {
	monitorsPath string
	scalePath    string
}

func NewWakeConfig() *WakeConfig {
	if _, err := exec.LookPath(clamshellCommand); err != nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	// These are Omarchy's paths, which do not honor XDG overrides.
	return &WakeConfig{
		monitorsPath: filepath.Join(home, ".config/hypr/monitors.lua"),
		scalePath:    filepath.Join(home, ".local/state", togglesStateDir, internalScaleFile),
	}
}

type WakeSnapshot struct {
	scale         config.FileSnapshot
	monitorsPath  string
	previousBlock string
	writtenBlock  string
}

func (w *WakeConfig) Sync(p profile.Profile, monitors []hypr.Monitor) (WakeSnapshot, error) {
	if w == nil {
		return WakeSnapshot{}, nil
	}
	output, ok := internalOutput(p, monitors)
	if !ok {
		return WakeSnapshot{}, nil
	}
	snapshot := WakeSnapshot{}
	content, err := os.ReadFile(w.monitorsPath)
	if err != nil && !os.IsNotExist(err) {
		return snapshot, err
	}
	// Do not create a Lua config on legacy Omarchy, or add a block to the old
	// generated file while it is waiting to be retired after a safe apply.
	if err == nil && !config.IsGeneratedMonitorsConfig(content) {
		previous, body, err := splitWakeBlock(string(content))
		if err != nil {
			return snapshot, err
		}
		block := wakeStart + "-- Shared with Omarchy while hyprmoncfg manages displays.\n" +
			fmt.Sprintf("hl.monitor({ output = %q, mode = \"preferred\", position = \"%dx%d\", scale = %s })\n",
				output.Name, output.X, output.Y, strconv.FormatFloat(output.Scale, 'f', -1, 64)) + wakeEnd
		if block != previous {
			if err := writeWakeConfig(w.monitorsPath, joinWakeBlock(block, body)); err != nil {
				return snapshot, err
			}
			snapshot.monitorsPath, snapshot.previousBlock, snapshot.writtenBlock = w.monitorsPath, previous, block
		}
	}
	scale, err := config.SnapshotFile(w.scalePath)
	if err == nil {
		value := strconv.FormatFloat(output.Scale, 'f', -1, 64) + "\n"
		if string(scale.Content) != value {
			err = config.WriteFileAtomic(w.scalePath, []byte(value), 0o644)
			if err == nil {
				snapshot.scale = scale
			}
		}
	}
	if err != nil {
		return WakeSnapshot{}, errors.Join(err, snapshot.Restore())
	}
	return snapshot, nil
}

// Restore changes only our block, so edits elsewhere made during a preview
// survive cancellation. Edits inside the block belong to the user too.
func (s WakeSnapshot) Restore() error {
	var blockErr error
	if s.monitorsPath != "" {
		content, err := os.ReadFile(s.monitorsPath)
		blockErr = err
		if err == nil {
			block, body, err := splitWakeBlock(string(content))
			blockErr = err
			if err == nil && block != s.previousBlock {
				if block != s.writtenBlock {
					blockErr = fmt.Errorf("wake settings in %s changed during preview; leaving your edits intact", s.monitorsPath)
				} else {
					blockErr = writeWakeConfig(s.monitorsPath, joinWakeBlock(s.previousBlock, body))
				}
			}
		}
	}
	return errors.Join(blockErr, s.scale.Restore())
}

func (w *WakeConfig) Release() error {
	if w == nil {
		return nil
	}
	content, err := os.ReadFile(w.monitorsPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	block, body, err := splitWakeBlock(string(content))
	if err != nil || block == "" {
		return err
	}
	return writeWakeConfig(w.monitorsPath, body)
}

func splitWakeBlock(content string) (block, body string, err error) {
	// A Lua byte-order mark must remain the first bytes in the file.
	bom := ""
	if strings.HasPrefix(content, "\ufeff") {
		bom, content = "\ufeff", strings.TrimPrefix(content, "\ufeff")
	}
	if !strings.Contains(content, wakeStart) && !strings.Contains(content, wakeEnd) {
		return "", bom + content, nil
	}
	if strings.HasPrefix(content, wakeStart) && strings.Count(content, wakeStart) == 1 && strings.Count(content, wakeEnd) == 1 {
		end := strings.Index(content, wakeEnd) + len(wakeEnd)
		return content[:end], bom + content[end:], nil
	}
	return "", "", errors.New("hyprmoncfg wake settings were moved or their markers were edited; leaving monitors.lua intact")
}

func joinWakeBlock(block, body string) string {
	if strings.HasPrefix(body, "\ufeff") {
		return "\ufeff" + block + strings.TrimPrefix(body, "\ufeff")
	}
	return block + body
}

func writeWakeConfig(path, content string) error {
	if !config.Writable(path) {
		return fmt.Errorf("%s is read-only; Omarchy wake settings could not be synchronized", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(path, []byte(content), info.Mode().Perm())
}

// Include disabled panels: Omarchy needs these settings when reopening the lid.
func internalOutput(p profile.Profile, monitors []hypr.Monitor) (profile.OutputConfig, bool) {
	resolver := profile.NewMonitorResolver(monitors)
	var internal profile.OutputConfig
	found := false
	for _, output := range p.Outputs {
		if monitor, ok := resolver.ResolveOutput(output); ok {
			output.Name = monitor.Name
		}
		if !hypr.IsInternalConnector(output.Name) {
			continue
		}
		if found || !connectorName.MatchString(output.Name) || output.Scale <= 0 || math.IsNaN(output.Scale) || math.IsInf(output.Scale, 0) {
			return profile.OutputConfig{}, false
		}
		internal, found = output, true
	}
	return internal, found
}
