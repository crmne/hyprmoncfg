package render

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

type Options struct {
	Format       config.HyprConfigFormat
	UseMonitorV2 bool
}

func RenderHyprlandConfig(p profile.Profile, monitors []hypr.Monitor, useV2 bool) (string, error) {
	return RenderConfig(p, monitors, Options{Format: config.HyprConfigLegacy, UseMonitorV2: useV2})
}

func RenderConfig(p profile.Profile, monitors []hypr.Monitor, opts Options) (string, error) {
	if opts.Format == config.HyprConfigLua {
		return renderLuaConfig(p, monitors, opts.UseMonitorV2)
	}
	return renderLegacyConfig(p, monitors, opts.UseMonitorV2)
}

func ProfileMonitors(p profile.Profile) []hypr.Monitor {
	p.Normalize()
	monitors := make([]hypr.Monitor, 0, len(p.Outputs))
	for _, output := range p.Outputs {
		name := strings.TrimSpace(output.Name)
		if name == "" {
			name = strings.TrimSpace(output.Key)
		}
		monitors = append(monitors, hypr.Monitor{
			Name:        name,
			Description: strings.TrimSpace(output.Description),
			Make:        output.Make,
			Model:       output.Model,
			Serial:      output.Serial,
			Width:       output.Width,
			Height:      output.Height,
			RefreshRate: output.Refresh,
			X:           output.X,
			Y:           output.Y,
			Scale:       output.Scale,
			Transform:   output.Transform,
			VRR:         hypr.VRRMode(output.VRR),
			Disabled:    !output.Enabled,
		})
	}

	byOutputKey := make(map[string]string, len(p.Outputs))
	for i, output := range p.Outputs {
		byOutputKey[output.Key] = monitors[i].Name
	}
	for i, output := range p.Outputs {
		if output.MirrorOf == "" {
			continue
		}
		monitors[i].MirrorOf = byOutputKey[output.MirrorOf]
	}

	return monitors
}

func CommandForOutput(name string, out profile.OutputConfig, mirrorTarget string) string {
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
	transform := out.Transform
	if transform < 0 || transform > 7 {
		transform = 0
	}

	vrr := out.VRR
	if vrr < 0 || vrr > 2 {
		vrr = 0
	}

	cmd := fmt.Sprintf("%s,%s,%dx%d,%s,transform,%d,vrr,%d", name, mode, x, y, scaling.Format(out.Scale), transform, vrr)
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

func WorkspaceRuleCommand(workspace string, monitorSelector string, isDefault bool, persistent bool) string {
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

func renderLegacyConfig(p profile.Profile, monitors []hypr.Monitor, useV2 bool) (string, error) {
	p = profile.ExtendConnected(p, monitors)
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
				mirrorTarget = legacyHyprlangSelector(resolver.SelectorForOutput(target.config, target.monitor), target.monitor)
			}
		}
		identifier := legacyHyprlangSelector(resolver.SelectorForOutput(item.config, item.monitor), item.monitor)
		if useV2 {
			monitorBlocks = append(monitorBlocks, renderMonitorV2Block(hyprlangEscape(identifier), item.config, hyprlangEscape(mirrorTarget)))
			continue
		}
		monitorBlocks = append(monitorBlocks, "monitor = "+hyprlangEscape(CommandForOutput(identifier, item.config, mirrorTarget)))
	}
	for _, monitor := range profile.OmittedMonitors(p, monitors) {
		identifier := legacyHyprlangSelector(selectorForMonitor(resolver, monitor), monitor)
		disabled := profile.OutputConfig{Enabled: false}
		if useV2 {
			monitorBlocks = append(monitorBlocks, renderMonitorV2Block(hyprlangEscape(identifier), disabled, ""))
		} else {
			monitorBlocks = append(monitorBlocks, "monitor = "+hyprlangEscape(CommandForOutput(identifier, disabled, "")))
		}
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
		selector := legacyHyprlangSelector(resolver.SelectorForOutput(output, monitor), monitor)
		workspaceLines = append(workspaceLines, "workspace = "+hyprlangEscape(WorkspaceRuleCommand(rule.Workspace, selector, rule.Default, rule.Persistent)))
	}

	sections := []string{config.GeneratedHeader(config.HyprConfigLegacy), strings.Join(monitorBlocks, "\n\n")}
	if len(workspaceLines) > 0 {
		sections = append(sections, strings.Join(workspaceLines, "\n"))
	}
	return strings.Join(sections, "\n\n") + "\n", nil
}

func renderLuaConfig(p profile.Profile, monitors []hypr.Monitor, useV2 bool) (string, error) {
	p = profile.ExtendConnected(p, monitors)
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
			monitorBlocks = append(monitorBlocks, renderLuaMonitorCall(identifier, item.config, mirrorTarget))
			continue
		}
		monitorBlocks = append(monitorBlocks, "hl.monitor("+luaQuote(CommandForOutput(identifier, item.config, mirrorTarget))+")")
	}
	for _, monitor := range profile.OmittedMonitors(p, monitors) {
		identifier := selectorForMonitor(resolver, monitor)
		disabled := profile.OutputConfig{Enabled: false}
		if useV2 {
			monitorBlocks = append(monitorBlocks, renderLuaMonitorCall(identifier, disabled, ""))
		} else {
			monitorBlocks = append(monitorBlocks, "hl.monitor("+luaQuote(CommandForOutput(identifier, disabled, ""))+")")
		}
	}

	workspaceLines := make([]string, 0)
	rules := profile.ResolveWorkspaceRules(p, monitors)
	for _, rule := range rules {
		output, ok := p.OutputByKey(rule.OutputKey)
		if !ok {
			output = profile.OutputConfig{Key: rule.OutputKey, Name: rule.OutputName}
		}
		monitor, ok := resolver.ResolveOutput(output)
		if !ok {
			monitor, ok = resolver.Resolve(output.MatchIdentity(), rule.OutputName)
		}
		if !ok {
			continue
		}
		selector := resolver.SelectorForOutput(output, monitor)
		workspaceLines = append(workspaceLines, renderLuaWorkspaceRule(rule.Workspace, selector, rule.Default, rule.Persistent))
	}

	sections := []string{config.GeneratedHeader(config.HyprConfigLua), strings.Join(monitorBlocks, "\n\n")}
	if len(workspaceLines) > 0 {
		sections = append(sections, strings.Join(workspaceLines, "\n"))
	}
	return strings.Join(sections, "\n\n") + "\n", nil
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
	lines = append(lines, "  mode = "+hyprlangEscape(mode))
	lines = append(lines, fmt.Sprintf("  position = %dx%d", output.X, output.Y))
	lines = append(lines, "  scale = "+scaling.Format(output.Scale))
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
		lines = append(lines, "  cm = "+hyprlangEscape(output.CM))
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
		lines = append(lines, "  sdr_eotf = "+hyprlangEscape(output.SDREOTF))
	}
	if output.ICC != "" {
		lines = append(lines, "  icc = "+hyprlangEscape(output.ICC))
	}
	if mirrorTarget != "" {
		lines = append(lines, "  mirror = "+mirrorTarget)
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

func renderLuaMonitorCall(identifier string, output profile.OutputConfig, mirrorTarget string) string {
	lines := []string{
		"hl.monitor({",
		"  output = " + luaQuote(identifier) + ",",
	}
	if !output.Enabled {
		lines = append(lines, "  disabled = true,", "})")
		return strings.Join(lines, "\n")
	}
	mode := strings.TrimSpace(strings.TrimSuffix(output.NormalizedMode(), "Hz"))
	if mode == "" {
		mode = "preferred"
	}
	lines = append(lines, "  mode = "+luaQuote(mode)+",")
	lines = append(lines, fmt.Sprintf("  position = %s,", luaQuote(fmt.Sprintf("%dx%d", output.X, output.Y))))
	lines = append(lines, "  scale = "+scaling.Format(output.Scale)+",")
	if output.Transform != 0 {
		lines = append(lines, fmt.Sprintf("  transform = %d,", output.Transform))
	}
	if output.VRR != 0 {
		lines = append(lines, fmt.Sprintf("  vrr = %d,", output.VRR))
	}
	if output.Bitdepth > 0 && output.Bitdepth != 8 {
		lines = append(lines, fmt.Sprintf("  bitdepth = %d,", output.Bitdepth))
	}
	if output.CM != "" && output.CM != "srgb" {
		lines = append(lines, "  cm = "+luaQuote(output.CM)+",")
	}
	if output.SDRBrightness != 0 && output.SDRBrightness != 1.0 {
		lines = append(lines, "  sdrbrightness = "+formatFloat(output.SDRBrightness, 2)+",")
	}
	if output.SDRSaturation != 0 && output.SDRSaturation != 1.0 {
		lines = append(lines, "  sdrsaturation = "+formatFloat(output.SDRSaturation, 2)+",")
	}
	if output.SDRMinLuminance != 0 || output.SDRMaxLuminance != 0 {
		lines = append(lines, "  sdr_min_luminance = "+formatFloat(output.SDRMinLuminance, 3)+",")
		lines = append(lines, fmt.Sprintf("  sdr_max_luminance = %d,", output.SDRMaxLuminance))
	}
	if output.MinLuminance != 0 || output.MaxLuminance != 0 {
		lines = append(lines, "  min_luminance = "+formatFloat(output.MinLuminance, 3)+",")
		lines = append(lines, fmt.Sprintf("  max_luminance = %d,", output.MaxLuminance))
	}
	if output.MaxAvgLuminance != 0 {
		lines = append(lines, fmt.Sprintf("  max_avg_luminance = %d,", output.MaxAvgLuminance))
	}
	if output.SupportsWideColor != 0 {
		lines = append(lines, fmt.Sprintf("  supports_wide_color = %d,", output.SupportsWideColor))
	}
	if output.SupportsHDR != 0 {
		lines = append(lines, fmt.Sprintf("  supports_hdr = %d,", output.SupportsHDR))
	}
	if output.SDREOTF != "" && output.SDREOTF != "default" {
		lines = append(lines, "  sdr_eotf = "+luaQuote(output.SDREOTF)+",")
	}
	if output.ICC != "" {
		lines = append(lines, "  icc = "+luaQuote(output.ICC)+",")
	}
	if mirrorTarget != "" {
		lines = append(lines, "  mirror = "+luaQuote(mirrorTarget)+",")
	}
	lines = append(lines, "})")
	return strings.Join(lines, "\n")
}

func renderLuaWorkspaceRule(workspace string, monitorSelector string, isDefault bool, persistent bool) string {
	parts := []string{
		"workspace = " + luaQuote(workspace),
		"monitor = " + luaQuote(monitorSelector),
	}
	if isDefault {
		parts = append(parts, "default = true")
	}
	if persistent {
		parts = append(parts, "persistent = true")
	}
	return "hl.workspace_rule({ " + strings.Join(parts, ", ") + " })"
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

func luaQuote(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for i := 0; i < len(value); i++ {
		switch c := value[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 || c == 0x7f || c >= 0x80 {
				b.WriteString(fmt.Sprintf(`\%03d`, c))
				continue
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func hyprlangEscape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "#", "##")
	value = hyprlangEscapeExpressionStarts(value)
	value = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(value)
	return value
}

func hyprlangEscapeExpressionStarts(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); {
		if value[i] != '{' {
			b.WriteByte(value[i])
			i++
			continue
		}

		j := i
		for j < len(value) && value[j] == '{' {
			j++
		}
		if j-i == 1 {
			b.WriteByte('{')
		} else {
			for ; i < j; i++ {
				b.WriteString(`\{`)
			}
			continue
		}
		i = j
	}
	return b.String()
}

func legacyHyprlangSelector(selector string, monitor hypr.Monitor) string {
	if hyprlangDescSelectorNeedsConnector(selector) {
		if name := strings.TrimSpace(monitor.Name); name != "" {
			return name
		}
	}
	return selector
}

func hyprlangDescSelectorNeedsConnector(selector string) bool {
	desc, ok := strings.CutPrefix(strings.TrimSpace(selector), "desc:")
	return ok && strings.ContainsAny(desc, "$,\r\n")
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

func selectorForMonitor(resolver profile.MonitorResolver, monitor hypr.Monitor) string {
	return resolver.SelectorForOutput(profile.OutputConfig{
		MatchKey: monitor.HardwareKey(),
		Name:     monitor.Name,
	}, monitor)
}
