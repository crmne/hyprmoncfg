package apply

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildkite/shellwords"
	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/omarchywatch"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/render"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

type applyMode int

const (
	ApplyModeInteractive applyMode = iota
	ApplyModeNonInteractive
)

const (
	applyValidationTimeout      = 3 * time.Second
	applyValidationPollInterval = 100 * time.Millisecond
	luaProbePrefix              = "__hyprmoncfg_probe_"
)

type Engine struct {
	Client             *hypr.Client
	LaptopToggle       *omarchywatch.LaptopToggle
	MonitorsConfPath   string
	HyprlandConfigPath string
	Logf               func(format string, args ...any)
}

type RevertState struct {
	MonitorsConf config.FileSnapshot
	LaptopToggle config.FileSnapshot
	RootConf     config.FileSnapshot
	RootWritten  []byte
	Commands     []string
}

type RenderOptions = render.Options

func SnapshotState(monitors []hypr.Monitor, rules []hypr.WorkspaceRule, workspaces []hypr.WorkspaceState) []string {
	return snapshotState(monitors, rules, workspaces, false)
}

func snapshotState(monitors []hypr.Monitor, rules []hypr.WorkspaceRule, workspaces []hypr.WorkspaceState, luaDispatch bool) []string {
	if luaDispatch {
		// Reloading the restored Lua config reinstates its monitor and workspace
		// rules. Lua-mode Hyprland rejects legacy `keyword` commands, so the live
		// rollback only needs to restore where existing workspaces were placed.
		return snapshotWorkspaceMoveCommands(workspaces, true)
	}
	commands := SnapshotCommands(monitors)
	commands = append(commands, snapshotWorkspaceRuleCommands(rules)...)
	commands = append(commands, snapshotWorkspaceMoveCommands(workspaces, luaDispatch)...)
	return commands
}

func CommandsForProfile(p profile.Profile, monitors []hypr.Monitor) ([]string, error) {
	p.Normalize()
	if len(monitors) == 0 {
		return nil, fmt.Errorf("no monitors detected")
	}

	resolver := profile.NewMonitorResolver(monitors)
	matched, matchedByKey := resolveProfileOutputs(p, resolver)
	if len(matched) == 0 {
		return nil, fmt.Errorf("profile %q does not match any connected monitor", p.Name)
	}

	commands := make([]string, 0, len(matched))
	for _, item := range matched {
		mirrorTarget := ""
		if item.config.MirrorOf != "" {
			if target, ok := matchedByKey[item.config.MirrorOf]; ok {
				mirrorTarget = target.monitor.Name
			}
		}
		commands = append(commands, render.CommandForOutput(item.monitor.Name, item.config, mirrorTarget))
	}
	for _, monitor := range profile.OmittedMonitors(p, monitors) {
		commands = append(commands, render.CommandForOutput(monitor.Name, profile.OutputConfig{Enabled: false}, ""))
	}

	if len(commands) == 0 {
		return nil, fmt.Errorf("profile %q does not match any connected monitor", p.Name)
	}
	return commands, nil
}

func WorkspaceCommandsForProfile(p profile.Profile, monitors []hypr.Monitor) []string {
	return workspaceCommandsForProfile(p, monitors, false)
}

func workspaceCommandsForProfile(p profile.Profile, monitors []hypr.Monitor, luaDispatch bool) []string {
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
		if !luaDispatch {
			commands = append(commands, "keyword workspace "+render.WorkspaceRuleCommand(rule.Workspace, selector, rule.Default, rule.Persistent))
		}
		commands = append(commands, workspaceMoveCommand(rule.Workspace, monitor.Name, luaDispatch))
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
		commands = append(commands, render.CommandForOutput(m.Name, out, m.MirrorOf))
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

	version, err := e.Client.Version(ctx)
	if err != nil {
		return RevertState{}, err
	}
	supportsV2, err := e.Client.SupportsMonitorV2(ctx)
	if err != nil {
		return RevertState{}, err
	}

	resolvedConfig, err := config.ResolveHyprlandConfig(firstNonEmpty(version.Version, version.Tag), e.MonitorsConfPath, e.HyprlandConfigPath)
	if err != nil {
		return RevertState{}, err
	}
	backup, err := config.SnapshotFile(resolvedConfig.MonitorsPath)
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
	luaDispatch := resolvedConfig.Format == config.HyprConfigLua
	revertState := RevertState{
		MonitorsConf: backup,
		Commands:     snapshotState(monitors, currentRules, currentWorkspaces, luaDispatch),
	}

	rendered, err := render.RenderConfig(p, monitors, render.Options{Format: resolvedConfig.Format, UseMonitorV2: supportsV2})
	if err != nil {
		return RevertState{}, err
	}

	renderedForReload := rendered
	luaProbe := ""
	if resolvedConfig.Format == config.HyprConfigLua {
		renderedForReload, luaProbe, err = addLuaExecutionProbe(rendered)
		if err != nil {
			return RevertState{}, err
		}
	}
	if err := config.WriteFileAtomic(resolvedConfig.MonitorsPath, []byte(renderedForReload), 0o644); err != nil {
		return RevertState{}, err
	}
	// Write the target before the include, and roll both back on every failure.
	// A cancelled apply still needs its own time to restore the previous layout.
	rollback := func(cause error) (RevertState, error) {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return RevertState{}, errors.Join(cause, wrapRollbackError("restore previous layout", e.Revert(restoreCtx, revertState)))
	}
	revertState.LaptopToggle, err = e.LaptopToggle.Sync(p, monitors)
	if err != nil {
		return rollback(fmt.Errorf("sync Omarchy laptop display: %w", err))
	}
	include, err := config.EnsureIncluded(resolvedConfig.RootPath, resolvedConfig.Format, resolvedConfig.MonitorsPath)
	if err != nil {
		return rollback(err)
	}
	if include.Changed() {
		revertState.RootConf = include.Previous
		revertState.RootWritten = include.Written
	}
	if err := e.Client.Reload(ctx); err != nil {
		return rollback(err)
	}
	if luaProbe != "" {
		if err := e.verifyLuaExecutionProbe(ctx, luaProbe, resolvedConfig.RootPath, resolvedConfig.MonitorsPath); err != nil {
			return rollback(err)
		}
		// The probe only needs to exist for the reload above. Keep the generated
		// file clean without causing a second monitor reload; each future apply
		// uses a fresh probe, so the current Lua global cannot satisfy it.
		if err := config.WriteFileAtomic(resolvedConfig.MonitorsPath, []byte(rendered), 0o644); err != nil {
			return rollback(fmt.Errorf("remove Lua execution probe: %w", err))
		}
	}

	applied, err := e.waitForAppliedProfile(ctx, p, monitors)
	if err != nil {
		return rollback(err)
	}

	if err := e.applyLiveCommands(ctx, workspaceCommandsForProfile(p, applied, luaDispatch)); err != nil {
		return rollback(err)
	}

	e.recordInternalScaleForOmarchy(p, monitors)

	if mode == ApplyModeNonInteractive {
		e.retireLegacyMonitorsFile(resolvedConfig)
		if err = e.PostApply(ctx, p); err != nil {
			if e.Logf != nil {
				e.Logf("post apply: %v", err)
			}
		}
	}

	return revertState, nil
}

// retireLegacyMonitorsFile empties the monitors file an older hyprmoncfg owned,
// once its rules are safely being served from the new one.
func (e Engine) retireLegacyMonitorsFile(resolved config.ResolvedHyprConfig) {
	retired, err := config.RetireLegacyMonitorsFile(resolved.Format, resolved.MonitorsPath)
	if err != nil {
		if e.Logf != nil {
			e.Logf("could not retire the previous generated monitor config: %v", err)
		}
		return
	}
	if retired != "" && e.Logf != nil {
		e.Logf("hyprmoncfg no longer writes %s; its rules now live in %s", retired, resolved.MonitorsPath)
	}
}

// recordInternalScaleForOmarchy keeps Omarchy's remembered internal-panel scale
// in step with what we just applied, so its clamshell script restores the panel
// at our scale instead of its own default. Best effort: on any other system,
// and for any profile without an internal panel, it does nothing.
func (e Engine) recordInternalScaleForOmarchy(p profile.Profile, monitors []hypr.Monitor) {
	scale, ok := internalOutputScale(p, monitors)
	if !ok {
		return
	}
	if err := omarchywatch.StoreInternalScale(scale); err != nil && e.Logf != nil {
		e.Logf("could not record the internal panel scale for Omarchy: %v", err)
	}
}

// internalOutputScale returns the scale a profile wants on the laptop panel,
// including when the profile turns that panel off: that is exactly the value
// Omarchy needs when it turns the panel back on.
func internalOutputScale(p profile.Profile, monitors []hypr.Monitor) (float64, bool) {
	resolver := profile.NewMonitorResolver(monitors)
	for _, output := range p.Outputs {
		if output.Scale <= 0 {
			continue
		}
		if monitor, ok := resolver.ResolveOutput(output); ok {
			if monitor.IsInternal() {
				return output.Scale, true
			}
			continue
		}
		if hypr.IsInternalConnector(output.Name) {
			return output.Scale, true
		}
	}
	return 0, false
}

func addLuaExecutionProbe(rendered string) (string, string, error) {
	// A syntax-error reload can leave Hyprland's previous Lua state alive. Use a
	// fresh global on every apply so a marker from an older config cannot pass.
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", "", fmt.Errorf("generate Lua execution probe: %w", err)
	}
	probe := luaProbePrefix + hex.EncodeToString(token[:])
	return strings.TrimRight(rendered, "\n") + "\n\n_G." + probe + " = true\n", probe, nil
}

func (e Engine) verifyLuaExecutionProbe(ctx context.Context, probe string, rootPath string, targetPath string) error {
	assertion := fmt.Sprintf(`assert(_G.%s == true, "hyprmoncfg generated monitor config did not run")`, probe)
	response, evalErr := e.Client.Eval(ctx, assertion)
	if evalErr == nil && response == "ok" {
		return nil
	}

	detail := response
	if detail == "" && evalErr != nil {
		detail = evalErr.Error()
	}
	if detail == "" {
		detail = "Hyprland returned an empty response"
	}
	return fmt.Errorf(
		"%s did not run when Hyprland reloaded %s; add `dofile(%q)` to your Hyprland Lua config or pass a different --monitors-conf target (Hyprland response: %s)",
		targetPath,
		rootPath,
		targetPath,
		detail,
	)
}

func wrapRollbackError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func (e Engine) waitForAppliedProfile(ctx context.Context, p profile.Profile, before []hypr.Monitor) ([]hypr.Monitor, error) {
	deadline := time.NewTimer(applyValidationTimeout)
	defer deadline.Stop()

	ticker := time.NewTicker(applyValidationPollInterval)
	defer ticker.Stop()

	var lastErr error

	for {
		applied, err := e.Client.Monitors(ctx)
		if err != nil {
			lastErr = err
		} else if err := ValidateAppliedProfile(p, before, applied); err != nil {
			lastErr = err
		} else {
			return applied, nil
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("%w: %v", ctx.Err(), lastErr)
			}
			return nil, ctx.Err()
		case <-deadline.C:
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("timed out waiting for Hyprland monitor reload")
		case <-ticker.C:
		}
	}
}

func (e Engine) PostApply(ctx context.Context, target profile.Profile) error {
	parts, err := splitPostApplyCommand(target.Exec)
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return nil
	}

	command := parts[0]
	args := parts[1:]

	cmd := exec.CommandContext(ctx, command, args...)
	if e.Client != nil {
		instance, err := e.Client.InstanceSignature(ctx)
		if err != nil {
			return fmt.Errorf("resolve post-apply session: %w", err)
		}
		cmd.Env = append(os.Environ(), "HYPRLAND_INSTANCE_SIGNATURE="+instance)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to exec script: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func ValidatePostApplyExec(command string) error {
	parts, err := splitPostApplyCommand(command)
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return nil
	}

	executable := parts[0]
	if strings.Contains(executable, string(os.PathSeparator)) {
		info, err := os.Stat(executable)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s does not exist", executable)
			}
			return fmt.Errorf("cannot access %s: %w", executable, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", executable)
		}
		if info.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("not executable: %s", executable)
		}
		return nil
	}

	if _, err := exec.LookPath(executable); err != nil {
		return fmt.Errorf("%q is not executable or was not found in PATH", executable)
	}
	return nil
}

func splitPostApplyCommand(command string) ([]string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, nil
	}

	parts, err := shellwords.Split(command)
	if err != nil {
		return nil, fmt.Errorf("split shellwords: %w", err)
	}
	if len(parts) == 0 {
		return nil, nil
	}

	parts[0] = expandUserPath(parts[0])
	return parts, nil
}

func expandUserPath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

func (e Engine) Revert(ctx context.Context, state RevertState) error {
	if e.Client == nil {
		return fmt.Errorf("nil hypr client")
	}
	if state.RootConf.Path != "" {
		current, err := os.ReadFile(state.RootConf.Path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, state.RootWritten) && !bytes.Equal(current, state.RootConf.Content) {
			return fmt.Errorf("%s changed during the preview; keeping the generated file to avoid overwriting your edits", state.RootConf.Path)
		}
		// Remove the new include before removing a newly generated target.
		if !bytes.Equal(current, state.RootConf.Content) {
			if err := state.RootConf.Restore(); err != nil {
				return err
			}
		}
	}
	if state.MonitorsConf.Path != "" {
		if err := state.MonitorsConf.Restore(); err != nil {
			return err
		}
		if err := e.LaptopToggle.Restore(state.LaptopToggle); err != nil {
			return err
		}
		if err := e.Client.Reload(ctx); err != nil {
			return err
		}
	}
	return e.applyLiveCommands(ctx, state.Commands)
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
		commands = append(commands, "keyword workspace "+render.WorkspaceRuleCommand(rule.WorkspaceString, rule.Monitor, rule.Default, rule.Persistent))
	}
	return commands
}

func snapshotWorkspaceMoveCommands(workspaces []hypr.WorkspaceState, luaDispatch bool) []string {
	commands := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		if strings.HasPrefix(workspace.Name, "special:") || workspace.Monitor == "" {
			continue
		}
		commands = append(commands, workspaceMoveCommand(workspace.Name, workspace.Monitor, luaDispatch))
	}
	return commands
}

func workspaceMoveCommand(workspace string, monitor string, luaDispatch bool) string {
	if luaDispatch {
		return fmt.Sprintf("dispatch hl.dsp.workspace.move({ workspace = %q, monitor = %q })", workspace, monitor)
	}
	return fmt.Sprintf("dispatch moveworkspacetomonitor %s %s", shellEscape(workspace), monitor)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (e Engine) applyLiveCommands(ctx context.Context, commands []string) error {
	if e.Client == nil || len(commands) == 0 {
		return nil
	}
	return e.Client.Batch(ctx, keywordifyMonitorCommands(commands))
}

func RenderHyprlandConfig(p profile.Profile, monitors []hypr.Monitor, useV2 bool) (string, error) {
	return render.RenderHyprlandConfig(p, monitors, useV2)
}

func RenderConfig(p profile.Profile, monitors []hypr.Monitor, opts RenderOptions) (string, error) {
	return render.RenderConfig(p, monitors, opts)
}

func ValidateLayout(outputs []profile.OutputConfig) error {
	return profile.ValidateLayout(outputs)
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
			return fmt.Errorf("%s scale mismatch: wanted %s, got %s", monitor.Name, scaling.Format(output.Scale), scaling.Format(applied.Scale))
		}
		if applied.Transform != output.Transform {
			return fmt.Errorf("%s transform mismatch: wanted %d, got %d", monitor.Name, output.Transform, applied.Transform)
		}
		// VRR validation skipped: hyprctl reports VRR as a boolean (active
		// or not), not the configured mode (0/1/2).
	}

	for _, monitor := range profile.OmittedMonitors(p, before) {
		applied, ok := afterResolver.Resolve(monitor.HardwareKey(), monitor.Name)
		if ok && !applied.Disabled {
			return fmt.Errorf("%s remained enabled even though it is not in profile %q", monitor.Name, p.Name)
		}
	}

	return nil
}

func logicalOutputSize(output profile.OutputConfig) (int, int) {
	scale := scaling.Round(scaling.Default(output.Scale))
	width := int(math.Round(float64(output.Width) / scale))
	height := int(math.Round(float64(output.Height) / scale))
	if output.Transform%2 == 1 {
		width, height = height, width
	}
	return max(1, width), max(1, height)
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
