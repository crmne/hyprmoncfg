package profile

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func (w WorkspaceSettings) Validate() error {
	if !w.Enabled {
		return nil
	}

	switch w.Strategy {
	case "", WorkspaceStrategyManual, WorkspaceStrategySequential, WorkspaceStrategyInterleave:
	default:
		return fmt.Errorf("invalid workspace strategy %q", w.Strategy)
	}

	if w.MaxWorkspaces < 0 {
		return fmt.Errorf("workspace max cannot be negative")
	}
	if w.GroupSize < 0 {
		return fmt.Errorf("workspace group size cannot be negative")
	}
	return nil
}

func WorkspaceSettingsFromHypr(monitors []hypr.Monitor, rules []hypr.WorkspaceRule) WorkspaceSettings {
	settings := WorkspaceSettings{
		Enabled:       len(rules) > 0,
		Strategy:      WorkspaceStrategyManual,
		MaxWorkspaces: 9,
		GroupSize:     3,
		MonitorOrder:  hypr.MonitorOrder(monitors),
	}

	if len(rules) == 0 {
		settings.Strategy = WorkspaceStrategySequential
		return settings
	}

	settings.Rules = make([]WorkspaceRule, 0, len(rules))
	for _, rule := range rules {
		outputKey, outputName := matchMonitorRule(rule.Monitor, monitors)
		settings.Rules = append(settings.Rules, WorkspaceRule{
			Workspace:  rule.WorkspaceString,
			OutputKey:  outputKey,
			OutputName: outputName,
			Default:    rule.Default,
			Persistent: rule.Persistent,
		})
	}

	sort.SliceStable(settings.Rules, func(i, j int) bool {
		return workspaceSortKey(settings.Rules[i].Workspace) < workspaceSortKey(settings.Rules[j].Workspace)
	})

	return settings
}

func ResolveWorkspaceRules(p Profile, monitors []hypr.Monitor) []WorkspaceRule {
	settings := p.Workspaces
	if !settings.Enabled {
		return nil
	}

	switch settings.Strategy {
	case "", WorkspaceStrategyManual:
		return normalizeManualRules(settings.Rules, p)
	case WorkspaceStrategySequential:
		return generatedWorkspaceRules(p, monitors, false)
	case WorkspaceStrategyInterleave:
		return generatedWorkspaceRules(p, monitors, true)
	default:
		return nil
	}
}

func WorkspacePreview(settings WorkspaceSettings, outputs []OutputConfig, monitors []hypr.Monitor) map[string][]string {
	profileView := Profile{Outputs: outputs, Workspaces: settings}
	resolved := ResolveWorkspaceRules(profileView, monitors)
	preview := make(map[string][]string)
	for _, rule := range resolved {
		label := rule.OutputName
		if label == "" {
			label = rule.OutputKey
		}
		preview[label] = append(preview[label], rule.Workspace)
	}
	return preview
}

func normalizeManualRules(rules []WorkspaceRule, p Profile) []WorkspaceRule {
	if len(rules) == 0 {
		return nil
	}

	out := make([]WorkspaceRule, 0, len(rules))
	for _, rule := range rules {
		if rule.OutputKey == "" && rule.OutputName == "" {
			continue
		}
		if rule.OutputName == "" {
			if output, ok := p.OutputByKey(rule.OutputKey); ok {
				rule.OutputName = output.Name
			}
		}
		out = append(out, rule)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return workspaceSortKey(out[i].Workspace) < workspaceSortKey(out[j].Workspace)
	})
	return out
}

func generatedWorkspaceRules(p Profile, monitors []hypr.Monitor, interleave bool) []WorkspaceRule {
	order := orderedOutputKeys(p, monitors)
	if len(order) == 0 {
		return nil
	}

	settings := p.Workspaces
	maxWorkspaces := settings.MaxWorkspaces
	if maxWorkspaces <= 0 {
		maxWorkspaces = 9
	}
	groupSize := settings.GroupSize
	if groupSize <= 0 {
		groupSize = 3
	}

	rules := make([]WorkspaceRule, 0, maxWorkspaces)
	seenDefault := make(map[string]bool, len(order))
	for idx := 1; idx <= maxWorkspaces; idx++ {
		monitorIndex := 0
		if interleave {
			monitorIndex = (idx - 1) % len(order)
		} else {
			monitorIndex = ((idx - 1) / groupSize) % len(order)
		}

		key := order[monitorIndex]
		output, ok := p.OutputByKey(key)
		if !ok {
			continue
		}

		rule := WorkspaceRule{
			Workspace:  strconv.Itoa(idx),
			OutputKey:  key,
			OutputName: output.Name,
		}
		if !seenDefault[key] {
			rule.Default = true
			rule.Persistent = true
			seenDefault[key] = true
		}
		rules = append(rules, rule)
	}
	return rules
}

func orderedOutputKeys(p Profile, monitors []hypr.Monitor) []string {
	byKey := make(map[string]OutputConfig, len(p.Outputs))
	for _, output := range p.Outputs {
		if output.Enabled && output.MirrorOf == "" {
			byKey[output.Key] = output
		}
	}

	keys := make([]string, 0, len(byKey))
	for _, key := range p.Workspaces.MonitorOrder {
		if _, ok := byKey[key]; ok {
			keys = append(keys, key)
			delete(byKey, key)
		}
	}

	fallback := append([]string(nil), hypr.MonitorOrder(monitors)...)
	for _, key := range fallback {
		if _, ok := byKey[key]; ok {
			keys = append(keys, key)
			delete(byKey, key)
		}
	}

	extras := make([]string, 0, len(byKey))
	for key := range byKey {
		extras = append(extras, key)
	}
	sort.Strings(extras)
	keys = append(keys, extras...)
	return keys
}

func matchMonitorRule(selector string, monitors []hypr.Monitor) (string, string) {
	selector = strings.TrimSpace(selector)
	for _, monitor := range monitors {
		if selector == monitor.Name || selector == monitor.MonitorSelector() {
			return monitor.HardwareKey(), monitor.Name
		}
	}

	if strings.HasPrefix(selector, "desc:") {
		desc := strings.TrimPrefix(selector, "desc:")
		for _, monitor := range monitors {
			if strings.TrimSpace(monitor.Description) == strings.TrimSpace(desc) {
				return monitor.HardwareKey(), monitor.Name
			}
		}
	}

	return selector, selector
}

func workspaceSortKey(name string) string {
	if id, err := strconv.Atoi(name); err == nil {
		return fmt.Sprintf("%08d", id)
	}
	return "zzzzzzzz:" + name
}
