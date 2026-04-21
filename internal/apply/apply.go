package apply

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/buildkite/shellwords"
	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type applyMode int

const (
	ApplyModeInteractive applyMode = iota
	ApplyModeNonInteractive
)

type Engine struct {
	Client             *hypr.Client
	MonitorsConfPath   string
	HyprlandConfigPath string
	Logf               func(format string, args ...any)
}

type RevertState struct {
	MonitorsConf config.FileSnapshot
	Commands     []string
}

func SnapshotState(monitors []hypr.Monitor, rules []hypr.WorkspaceRule, workspaces []hypr.WorkspaceState) []string {
	commands := SnapshotCommands(monitors)
	commands = append(commands, snapshotWorkspaceRuleCommands(rules)...)
	commands = append(commands, snapshotWorkspaceMoveCommands(workspaces)...)
	return commands
}

func CommandsForProfile(p profile.Profile, monitors []hypr.Monitor) ([]string, error) {
	p.Normalize()
	if len(monitors) == 0 {
		return nil, fmt.Errorf("no monitors detected")
	}

	resolver := profile.NewMonitorResolver(monitors)
	matched, matchedByKey := resolveProfileOutputs(p, resolver)

	commands := make([]string, 0, len(matched))
	for _, item := range matched {
		mirrorTarget := ""
		if item.config.MirrorOf != "" {
			if target, ok := matchedByKey[item.config.MirrorOf]; ok {
				mirrorTarget = target.monitor.Name
			}
		}
		commands = append(commands, commandForOutput(item.monitor.Name, item.config, mirrorTarget))
	}

	if len(commands) == 0 {
		return nil, fmt.Errorf("profile %q does not match any connected monitor", p.Name)
	}
	return commands, nil
}

func WorkspaceCommandsForProfile(p profile.Profile, monitors []hypr.Monitor) []string {
	p.Normalize()
	rules := profile.ResolveWorkspaceRules(p, monitors)
	if len(rules) == 0 {
		return nil
	}

	resolver := profile.NewMonitorResolver(monitors)

	commands := make([]string, 0, len(rules)*2)
	for _, rule := range rules {
		output, ok := p.OutputByKey(rule.OutputKey)
		if !ok {
			output = profile.OutputConfig{
				Key:  rule.OutputKey,
				Name: rule.OutputName,
			}
		}
		monitor, ok := resolver.ResolveOutput(output)
		if !ok {
			monitor, ok = resolver.Resolve(output.MatchIdentity(), rule.OutputName)
		}
		if !ok {
			continue
		}

		selector := resolver.SelectorForOutput(output, monitor)
		commands = append(commands, "keyword workspace "+workspaceRuleCommand(rule.Workspace, selector, rule.Default, rule.Persistent))
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
			Enabled:         true,
			Mode:            m.ModeString(),
			Width:           m.Width,
			Height:          m.Height,
			Refresh:         m.RefreshRate,
			X:               m.X,
			Y:               m.Y,
			Scale:           m.Scale,
			VRR:             int(m.VRR),
			Transform:       m.Transform,
			Bitdepth:        m.Bitdepth(),
			CM:              m.ColorManagementPreset,
			SDRBrightness:   m.SDRBrightness,
			SDRSaturation:   m.SDRSaturation,
			SDRMinLuminance: m.SDRMinLuminance,
			SDRMaxLuminance: m.SDRMaxLuminance,
		}
		commands = append(commands, commandForOutput(m.Name, out, m.MirrorOf))
	}
	return commands
}

func (e Engine) Apply(ctx context.Context, p profile.Profile, monitors []hypr.Monitor, modearg ...applyMode) (RevertState, error) {
	mode := ApplyModeNonInteractive
	if len(modearg) > 0 {
		mode = modearg[0]
	}

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
	currentRules, err := e.Client.WorkspaceRules(ctx)
	if err != nil {
		return RevertState{}, err
	}
	currentWorkspaces, err := e.Client.Workspaces(ctx)
	if err != nil {
		return RevertState{}, err
	}
	revertState := RevertState{
		MonitorsConf: backup,
		Commands:     SnapshotState(monitors, currentRules, currentWorkspaces),
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

	if err := e.applyLiveCommands(ctx, WorkspaceCommandsForProfile(p, applied)); err != nil {
		_ = backup.Restore()
		_ = e.Client.Reload(ctx)
		_ = e.applyLiveCommands(ctx, revertState.Commands)
		return RevertState{}, err
	}

	if mode == ApplyModeNonInteractive {
		if err = e.PostApply(ctx, p); err != nil {
			if e.Logf != nil {
				e.Logf("post apply: %v", err)
			}
		}
	}

	return revertState, nil
}

func (e Engine) PostApply(ctx context.Context, target profile.Profile) error {
	postApplyCommand := strings.TrimSpace(target.Exec)

	if postApplyCommand == "" {
		return nil
	}

	parts, err := shellwords.Split(postApplyCommand)
	if err != nil {
		return fmt.Errorf("split shellwords: %w", err)
	}

	var args []string
	command := parts[0]

	if len(parts) > 1 {
		args = parts[1:]
	}

	cmd := exec.CommandContext(ctx, command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to exec script: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func (e Engine) Revert(ctx context.Context, state RevertState) error {
	if e.Client == nil {
		return fmt.Errorf("nil hypr client")
	}
	if state.MonitorsConf.Path != "" {
		if err := state.MonitorsConf.Restore(); err != nil {
			return err
		}
		if err := e.Client.Reload(ctx); err != nil {
			return err
		}
	}
	return e.applyLiveCommands(ctx, state.Commands)
}

func commandForOutput(name string, out profile.OutputConfig, mirrorTarget string) string {
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

	cmd := fmt.Sprintf("%s,%s,%dx%d,%s,transform,%d,vrr,%d", name, mode, x, y, formatFloat(scale, 3), transform, vrr)
	if out.Bitdepth > 0 && out.Bitdepth != 8 {
		cmd += fmt.Sprintf(",bitdepth,%d", out.Bitdepth)
	}
	if out.CM != "" && out.CM != "srgb" {
		cmd += ",cm," + out.CM
	}
	if out.SDRBrightness != 0 && out.SDRBrightness != 1.0 {
		cmd += ",sdrbrightness," + formatFloat(out.SDRBrightness, 2)
	}
	if out.SDRSaturation != 0 && out.SDRSaturation != 1.0 {
		cmd += ",sdrsaturation," + formatFloat(out.SDRSaturation, 2)
	}
	if out.SDREOTF != "" && out.SDREOTF != "default" {
		// v1 uses numeric: 0=default, 1=srgb, 2=gamma22
		switch out.SDREOTF {
		case "srgb":
			cmd += ",sdr_eotf,1"
		case "gamma22":
			cmd += ",sdr_eotf,2"
		}
	}
	if out.ICC != "" {
		cmd += ",icc," + out.ICC
	}
	if mirrorTarget != "" {
		cmd += ",mirror," + mirrorTarget
	}
	return cmd
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

func (e Engine) applyLiveCommands(ctx context.Context, commands []string) error {
	if e.Client == nil || len(commands) == 0 {
		return nil
	}
	return e.Client.Batch(ctx, keywordifyMonitorCommands(commands))
}

func RenderHyprlandConfig(p profile.Profile, monitors []hypr.Monitor, useV2 bool) (string, error) {
	p.Normalize()
	resolver := profile.NewMonitorResolver(monitors)
	matched, matchedByKey := resolveProfileOutputs(p, resolver)
	if len(matched) == 0 {
		return "", fmt.Errorf("profile %q does not match any connected monitor", p.Name)
	}

	monitorBlocks := make([]string, 0, len(matched))
	for _, item := range matched {
		mirrorTarget := ""
		if item.config.MirrorOf != "" {
			if target, ok := matchedByKey[item.config.MirrorOf]; ok {
				mirrorTarget = resolver.SelectorForOutput(target.config, target.monitor)
			}
		}
		identifier := resolver.SelectorForOutput(item.config, item.monitor)
		if useV2 {
			monitorBlocks = append(monitorBlocks, renderMonitorV2Block(identifier, item.config, mirrorTarget))
			continue
		}
		monitorBlocks = append(monitorBlocks, "monitor = "+commandForOutput(identifier, item.config, mirrorTarget))
	}

	workspaceLines := make([]string, 0)
	rules := profile.ResolveWorkspaceRules(p, monitors)
	for _, rule := range rules {
		output, ok := p.OutputByKey(rule.OutputKey)
		if !ok {
			output = profile.OutputConfig{
				Key:  rule.OutputKey,
				Name: rule.OutputName,
			}
		}
		monitor, ok := resolver.ResolveOutput(output)
		if !ok {
			monitor, ok = resolver.Resolve(output.MatchIdentity(), rule.OutputName)
		}
		if !ok {
			continue
		}
		selector := resolver.SelectorForOutput(output, monitor)
		workspaceLines = append(workspaceLines, "workspace = "+workspaceRuleCommand(rule.Workspace, selector, rule.Default, rule.Persistent))
	}

	sections := []string{"# Generated by hyprmoncfg", strings.Join(monitorBlocks, "\n\n")}
	if len(workspaceLines) > 0 {
		sections = append(sections, strings.Join(workspaceLines, "\n"))
	}
	return strings.Join(sections, "\n\n") + "\n", nil
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
		if !output.Enabled || output.MirrorOf != "" {
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
	p.Normalize()
	beforeResolver := profile.NewMonitorResolver(before)
	afterResolver := profile.NewMonitorResolver(after)

	for _, output := range p.Outputs {
		monitor, ok := beforeResolver.ResolveOutput(output)
		if !ok {
			continue
		}

		applied, ok := afterResolver.Resolve(monitor.HardwareKey(), monitor.Name)
		if !ok {
			continue
		}

		if !output.Enabled {
			if !applied.Disabled {
				return fmt.Errorf("%s remained enabled after apply", monitor.Name)
			}
			continue
		}
		if output.MirrorOf != "" {
			if applied.MirrorOf == "" {
				return fmt.Errorf("%s is not mirroring after apply", monitor.Name)
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
		// VRR validation skipped: hyprctl reports VRR as a boolean (active
		// or not), not the configured mode (0/1/2).
	}

	return nil
}

func renderMonitorV2Block(identifier string, output profile.OutputConfig, mirrorTarget string) string {
	lines := []string{
		"monitorv2 {",
		"  output = " + identifier,
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
	if output.Bitdepth > 0 && output.Bitdepth != 8 {
		lines = append(lines, fmt.Sprintf("  bitdepth = %d", output.Bitdepth))
	}
	if output.CM != "" && output.CM != "srgb" {
		lines = append(lines, "  cm = "+output.CM)
	}
	if output.SDRBrightness != 0 && output.SDRBrightness != 1.0 {
		lines = append(lines, "  sdrbrightness = "+formatFloat(output.SDRBrightness, 2))
	}
	if output.SDRSaturation != 0 && output.SDRSaturation != 1.0 {
		lines = append(lines, "  sdrsaturation = "+formatFloat(output.SDRSaturation, 2))
	}
	if output.SDRMinLuminance != 0 || output.SDRMaxLuminance != 0 {
		lines = append(lines, "  sdr_min_luminance = "+formatFloat(output.SDRMinLuminance, 3))
		lines = append(lines, fmt.Sprintf("  sdr_max_luminance = %d", output.SDRMaxLuminance))
	}
	if output.MinLuminance != 0 || output.MaxLuminance != 0 {
		lines = append(lines, "  min_luminance = "+formatFloat(output.MinLuminance, 3))
		lines = append(lines, fmt.Sprintf("  max_luminance = %d", output.MaxLuminance))
	}
	if output.MaxAvgLuminance != 0 {
		lines = append(lines, fmt.Sprintf("  max_avg_luminance = %d", output.MaxAvgLuminance))
	}
	if output.SupportsWideColor != 0 {
		lines = append(lines, fmt.Sprintf("  supports_wide_color = %d", output.SupportsWideColor))
	}
	if output.SupportsHDR != 0 {
		lines = append(lines, fmt.Sprintf("  supports_hdr = %d", output.SupportsHDR))
	}
	if output.SDREOTF != "" && output.SDREOTF != "default" {
		lines = append(lines, "  sdr_eotf = "+output.SDREOTF)
	}
	if output.ICC != "" {
		lines = append(lines, "  icc = "+output.ICC)
	}
	if mirrorTarget != "" {
		lines = append(lines, "  mirror = "+mirrorTarget)
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
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

type matchedOutput struct {
	config  profile.OutputConfig
	monitor hypr.Monitor
}

func resolveProfileOutputs(p profile.Profile, resolver profile.MonitorResolver) ([]matchedOutput, map[string]matchedOutput) {
	matched := make([]matchedOutput, 0, len(p.Outputs))
	matchedByKey := make(map[string]matchedOutput, len(p.Outputs))
	for _, output := range p.Outputs {
		monitor, ok := resolver.ResolveOutput(output)
		if !ok {
			continue
		}
		item := matchedOutput{config: output, monitor: monitor}
		matched = append(matched, item)
		matchedByKey[output.Key] = item
	}
	return matched, matchedByKey
}
