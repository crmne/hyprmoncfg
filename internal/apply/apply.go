package apply

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type Engine struct {
	Client             *hypr.Client
	MonitorsConfPath   string
	HyprlandConfigPath string
}

type RevertState struct {
	MonitorsConf config.FileSnapshot
}

func SnapshotState(monitors []hypr.Monitor, rules []hypr.WorkspaceRule, workspaces []hypr.WorkspaceState) []string {
	commands := SnapshotCommands(monitors)
	commands = append(commands, snapshotWorkspaceRuleCommands(rules)...)
	commands = append(commands, snapshotWorkspaceMoveCommands(workspaces)...)
	return commands
}

func CommandsForProfile(p profile.Profile, monitors []hypr.Monitor) ([]string, error) {
	if len(monitors) == 0 {
		return nil, fmt.Errorf("no monitors detected")
	}

	byKey := make(map[string]hypr.Monitor, len(monitors))
	byName := make(map[string]hypr.Monitor, len(monitors))
	for _, m := range monitors {
		byKey[m.HardwareKey()] = m
		byName[m.Name] = m
	}

	commands := make([]string, 0, len(p.Outputs))
	for _, out := range p.Outputs {
		mon, ok := byKey[out.Key]
		if !ok && out.Name != "" {
			mon, ok = byName[out.Name]
		}
		if !ok {
			continue
		}
		commands = append(commands, commandForOutput(mon.Name, out))
	}

	if len(commands) == 0 {
		return nil, fmt.Errorf("profile %q does not match any connected monitor", p.Name)
	}
	return commands, nil
}

func WorkspaceCommandsForProfile(p profile.Profile, monitors []hypr.Monitor) []string {
	rules := profile.ResolveWorkspaceRules(p, monitors)
	if len(rules) == 0 {
		return nil
	}

	byKey := make(map[string]hypr.Monitor, len(monitors))
	byName := make(map[string]hypr.Monitor, len(monitors))
	for _, monitor := range monitors {
		byKey[monitor.HardwareKey()] = monitor
		byName[monitor.Name] = monitor
	}

	commands := make([]string, 0, len(rules)*2)
	for _, rule := range rules {
		monitor, ok := byKey[rule.OutputKey]
		if !ok && rule.OutputName != "" {
			monitor, ok = byName[rule.OutputName]
		}
		if !ok {
			continue
		}

		commands = append(commands, workspaceRuleCommand(rule.Workspace, monitor.MonitorSelector(), rule.Default, rule.Persistent))
		commands = append(commands, fmt.Sprintf("dispatch moveworkspacetomonitor %s %s", shellEscape(rule.Workspace), monitor.Name))
	}
	return commands
}

func SnapshotCommands(monitors []hypr.Monitor) []string {
	commands := make([]string, 0, len(monitors))
	for _, m := range monitors {
		if m.Disabled {
			commands = append(commands, fmt.Sprintf("%s,disable", m.Name))
			continue
		}
		out := profile.OutputConfig{
			Enabled:   true,
			Mode:      m.ModeString(),
			Width:     m.Width,
			Height:    m.Height,
			Refresh:   m.RefreshRate,
			X:         m.X,
			Y:         m.Y,
			Scale:     m.Scale,
			VRR:       boolToVRR(m.VRR),
			Transform: m.Transform,
		}
		commands = append(commands, commandForOutput(m.Name, out))
	}
	return commands
}

func (e Engine) Apply(ctx context.Context, p profile.Profile, monitors []hypr.Monitor) (RevertState, error) {
	if e.Client == nil {
		return RevertState{}, fmt.Errorf("nil hypr client")
	}
	if err := ValidateLayout(p.Outputs); err != nil {
		return RevertState{}, err
	}

	supportsV2, err := e.Client.SupportsMonitorV2(ctx)
	if err != nil {
		return RevertState{}, err
	}

	monitorsConfPath, err := config.ResolveMonitorsConfPath(e.MonitorsConfPath)
	if err != nil {
		return RevertState{}, err
	}
	hyprlandConfigPath, err := config.ResolveHyprlandConfigPath(e.HyprlandConfigPath)
	if err != nil {
		return RevertState{}, err
	}
	if err := config.VerifySourceChain(hyprlandConfigPath, monitorsConfPath); err != nil {
		return RevertState{}, err
	}
	backup, err := config.SnapshotFile(monitorsConfPath)
	if err != nil {
		return RevertState{}, err
	}

	rendered, err := RenderHyprlandConfig(p, monitors, supportsV2)
	if err != nil {
		return RevertState{}, err
	}
	if err := config.WriteFileAtomic(monitorsConfPath, []byte(rendered), 0o644); err != nil {
		return RevertState{}, err
	}
	if err := e.Client.Reload(ctx); err != nil {
		_ = backup.Restore()
		return RevertState{}, err
	}

	applied, err := e.Client.Monitors(ctx)
	if err != nil {
		_ = backup.Restore()
		_ = e.Client.Reload(ctx)
		return RevertState{}, err
	}
	if err := ValidateAppliedProfile(p, monitors, applied); err != nil {
		_ = backup.Restore()
		_ = e.Client.Reload(ctx)
		return RevertState{}, err
	}

	return RevertState{MonitorsConf: backup}, nil
}

func (e Engine) Revert(ctx context.Context, state RevertState) error {
	if e.Client == nil {
		return fmt.Errorf("nil hypr client")
	}
	if state.MonitorsConf.Path == "" {
		return nil
	}
	if err := state.MonitorsConf.Restore(); err != nil {
		return err
	}
	return e.Client.Reload(ctx)
}

func commandForOutput(name string, out profile.OutputConfig) string {
	if !out.Enabled {
		return fmt.Sprintf("%s,disable", name)
	}

	mode := strings.TrimSpace(out.NormalizedMode())
	if mode == "" {
		mode = "preferred"
	}
	mode = strings.TrimSuffix(mode, "Hz")

	x := out.X
	y := out.Y
	scale := out.Scale
	if scale <= 0 {
		scale = 1.0
	}
	transform := out.Transform
	if transform < 0 || transform > 7 {
		transform = 0
	}

	vrr := out.VRR
	if vrr < 0 || vrr > 2 {
		vrr = 0
	}

	return fmt.Sprintf("%s,%s,%dx%d,%s,transform,%d,vrr,%d", name, mode, x, y, formatFloat(scale, 3), transform, vrr)
}

func formatFloat(v float64, precision int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	s := strconv.FormatFloat(v, 'f', precision, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func keywordifyMonitorCommands(commands []string) []string {
	batch := make([]string, 0, len(commands))
	for _, cmd := range commands {
		if strings.HasPrefix(cmd, "dispatch ") || strings.HasPrefix(cmd, "keyword workspace ") || strings.HasPrefix(cmd, "keyword monitor ") {
			batch = append(batch, cmd)
			continue
		}
		batch = append(batch, "keyword monitor "+cmd)
	}
	return batch
}

func snapshotWorkspaceRuleCommands(rules []hypr.WorkspaceRule) []string {
	commands := make([]string, 0, len(rules))
	for _, rule := range rules {
		commands = append(commands, "keyword workspace "+workspaceRuleCommand(rule.WorkspaceString, rule.Monitor, rule.Default, rule.Persistent))
	}
	return commands
}

func snapshotWorkspaceMoveCommands(workspaces []hypr.WorkspaceState) []string {
	commands := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		if strings.HasPrefix(workspace.Name, "special:") || workspace.Monitor == "" {
			continue
		}
		commands = append(commands, fmt.Sprintf("dispatch moveworkspacetomonitor %s %s", shellEscape(workspace.Name), workspace.Monitor))
	}
	return commands
}

func workspaceRuleCommand(workspace string, monitorSelector string, isDefault bool, persistent bool) string {
	parts := []string{
		workspace,
		"monitor:" + monitorSelector,
	}
	if isDefault {
		parts = append(parts, "default:true")
	}
	if persistent {
		parts = append(parts, "persistent:true")
	}
	return strings.Join(parts, ", ")
}

func shellEscape(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\'' || r == '"'
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func boolToVRR(v bool) int {
	if v {
		return 1
	}
	return 0
}

func RenderHyprlandConfig(p profile.Profile, monitors []hypr.Monitor, useV2 bool) (string, error) {
	type matchedOutput struct {
		config  profile.OutputConfig
		monitor hypr.Monitor
	}

	byKey := make(map[string]hypr.Monitor, len(monitors))
	byName := make(map[string]hypr.Monitor, len(monitors))
	for _, monitor := range monitors {
		byKey[monitor.HardwareKey()] = monitor
		byName[monitor.Name] = monitor
	}

	matched := make([]matchedOutput, 0, len(p.Outputs))
	for _, output := range p.Outputs {
		monitor, ok := byKey[output.Key]
		if !ok && output.Name != "" {
			monitor, ok = byName[output.Name]
		}
		if !ok {
			continue
		}
		matched = append(matched, matchedOutput{config: output, monitor: monitor})
	}
	if len(matched) == 0 {
		return "", fmt.Errorf("profile %q does not match any connected monitor", p.Name)
	}

	blocks := []string{"# Generated by hyprmoncfg"}
	for _, item := range matched {
		if useV2 {
			blocks = append(blocks, renderMonitorV2Block(item.monitor, item.config))
			continue
		}
		blocks = append(blocks, "monitor = "+commandForOutput(monitorIdentifier(item.monitor), item.config))
	}

	rules := profile.ResolveWorkspaceRules(p, monitors)
	for _, rule := range rules {
		monitor, ok := byKey[rule.OutputKey]
		if !ok && rule.OutputName != "" {
			monitor, ok = byName[rule.OutputName]
		}
		if !ok {
			continue
		}
		blocks = append(blocks, "workspace = "+workspaceRuleCommand(rule.Workspace, monitor.MonitorSelector(), rule.Default, rule.Persistent))
	}

	return strings.Join(blocks, "\n\n") + "\n", nil
}

func ValidateLayout(outputs []profile.OutputConfig) error {
	type rect struct {
		name string
		x1   int
		y1   int
		x2   int
		y2   int
	}

	rects := make([]rect, 0, len(outputs))
	for _, output := range outputs {
		if !output.Enabled {
			continue
		}
		width, height := logicalOutputSize(output)
		rects = append(rects, rect{
			name: outputName(output),
			x1:   output.X,
			y1:   output.Y,
			x2:   output.X + width,
			y2:   output.Y + height,
		})
	}

	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].x1 < rects[j].x2 &&
				rects[i].x2 > rects[j].x1 &&
				rects[i].y1 < rects[j].y2 &&
				rects[i].y2 > rects[j].y1 {
				return fmt.Errorf("layout overlaps: %s intersects %s", rects[i].name, rects[j].name)
			}
		}
	}
	return nil
}

func ValidateAppliedProfile(p profile.Profile, before []hypr.Monitor, after []hypr.Monitor) error {
	afterByKey := make(map[string]hypr.Monitor, len(after))
	afterByName := make(map[string]hypr.Monitor, len(after))
	for _, monitor := range after {
		afterByKey[monitor.HardwareKey()] = monitor
		afterByName[monitor.Name] = monitor
	}

	beforeByKey := make(map[string]hypr.Monitor, len(before))
	beforeByName := make(map[string]hypr.Monitor, len(before))
	for _, monitor := range before {
		beforeByKey[monitor.HardwareKey()] = monitor
		beforeByName[monitor.Name] = monitor
	}

	for _, output := range p.Outputs {
		monitor, ok := beforeByKey[output.Key]
		if !ok && output.Name != "" {
			monitor, ok = beforeByName[output.Name]
		}
		if !ok {
			continue
		}

		applied, ok := afterByKey[monitor.HardwareKey()]
		if !ok {
			applied, ok = afterByName[monitor.Name]
		}
		if !ok {
			continue
		}

		if !output.Enabled {
			if !applied.Disabled {
				return fmt.Errorf("%s remained enabled after apply", monitor.Name)
			}
			continue
		}

		if applied.Disabled {
			return fmt.Errorf("%s was disabled after apply", monitor.Name)
		}
		if applied.X != output.X || applied.Y != output.Y {
			return fmt.Errorf("%s position mismatch: wanted %dx%d, got %dx%d", monitor.Name, output.X, output.Y, applied.X, applied.Y)
		}
		if output.Width > 0 && output.Height > 0 && (applied.Width != output.Width || applied.Height != output.Height) {
			return fmt.Errorf("%s mode mismatch: wanted %dx%d, got %dx%d", monitor.Name, output.Width, output.Height, applied.Width, applied.Height)
		}
		if math.Abs(applied.RefreshRate-output.Refresh) > 0.2 && output.Refresh > 0 {
			return fmt.Errorf("%s refresh mismatch: wanted %.2f, got %.2f", monitor.Name, output.Refresh, applied.RefreshRate)
		}
		if math.Abs(applied.Scale-output.Scale) > 0.02 {
			return fmt.Errorf("%s scale mismatch: wanted %.2f, got %.2f", monitor.Name, output.Scale, applied.Scale)
		}
		if applied.Transform != output.Transform {
			return fmt.Errorf("%s transform mismatch: wanted %d, got %d", monitor.Name, output.Transform, applied.Transform)
		}
		if boolToVRR(applied.VRR) != output.VRR {
			return fmt.Errorf("%s VRR mismatch: wanted %d, got %d", monitor.Name, output.VRR, boolToVRR(applied.VRR))
		}
	}

	return nil
}

func renderMonitorV2Block(monitor hypr.Monitor, output profile.OutputConfig) string {
	lines := []string{
		"monitorv2 {",
		"  output = " + monitorIdentifier(monitor),
	}
	if !output.Enabled {
		lines = append(lines, "  disabled = 1", "}")
		return strings.Join(lines, "\n")
	}

	mode := strings.TrimSpace(strings.TrimSuffix(output.NormalizedMode(), "Hz"))
	if mode == "" {
		mode = "preferred"
	}
	lines = append(lines, "  mode = "+mode)
	lines = append(lines, fmt.Sprintf("  position = %dx%d", output.X, output.Y))
	lines = append(lines, "  scale = "+formatFloat(clampScale(output.Scale), 3))
	if output.Transform != 0 {
		lines = append(lines, fmt.Sprintf("  transform = %d", output.Transform))
	}
	if output.VRR != 0 {
		lines = append(lines, fmt.Sprintf("  vrr = %d", output.VRR))
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

func monitorIdentifier(monitor hypr.Monitor) string {
	if desc := strings.TrimSpace(monitor.Description); desc != "" {
		return "desc:" + desc
	}
	return monitor.Name
}

func logicalOutputSize(output profile.OutputConfig) (int, int) {
	scale := clampScale(output.Scale)
	width := int(math.Round(float64(output.Width) / scale))
	height := int(math.Round(float64(output.Height) / scale))
	if output.Transform%2 == 1 {
		width, height = height, width
	}
	return max(1, width), max(1, height)
}

func clampScale(scale float64) float64 {
	if scale <= 0 {
		return 1
	}
	return scale
}

func outputName(output profile.OutputConfig) string {
	if strings.TrimSpace(output.Name) != "" {
		return output.Name
	}
	if strings.TrimSpace(output.Key) != "" {
		return output.Key
	}
	return "monitor"
}
