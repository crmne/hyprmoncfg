package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

type MonitorResolver struct {
	byOutputKey map[string]hypr.Monitor
	byName      map[string]hypr.Monitor
	byMatchKey  map[string][]hypr.Monitor
}

func NewMonitorResolver(monitors []hypr.Monitor) MonitorResolver {
	resolver := MonitorResolver{
		byOutputKey: make(map[string]hypr.Monitor, len(monitors)),
		byName:      make(map[string]hypr.Monitor, len(monitors)),
		byMatchKey:  make(map[string][]hypr.Monitor, len(monitors)),
	}
	matchCounts := hypr.MonitorMatchCounts(monitors)
	for _, monitor := range monitors {
		resolver.byOutputKey[hypr.MonitorOutputKey(monitor, matchCounts)] = monitor
		resolver.byName[strings.TrimSpace(monitor.Name)] = monitor
		matchKey := strings.TrimSpace(monitor.HardwareKey())
		resolver.byMatchKey[matchKey] = append(resolver.byMatchKey[matchKey], monitor)
	}
	return resolver
}

func (r MonitorResolver) Resolve(matchKey string, name string) (hypr.Monitor, bool) {
	matchKey = strings.TrimSpace(matchKey)
	name = strings.TrimSpace(name)

	if matchKey != "" {
		candidates := r.byMatchKey[matchKey]
		switch len(candidates) {
		case 1:
			return candidates[0], true
		case 0:
		default:
			if name != "" {
				for _, monitor := range candidates {
					if strings.EqualFold(strings.TrimSpace(monitor.Name), name) {
						return monitor, true
					}
				}
			}
			return hypr.Monitor{}, false
		}
	}

	if name != "" {
		monitor, ok := r.byName[name]
		return monitor, ok
	}

	return hypr.Monitor{}, false
}

func (r MonitorResolver) ResolveOutput(output OutputConfig) (hypr.Monitor, bool) {
	if output.Key != "" {
		if monitor, ok := r.byOutputKey[strings.TrimSpace(output.Key)]; ok {
			return monitor, true
		}
	}
	return r.Resolve(output.MatchIdentity(), output.Name)
}

func (r MonitorResolver) IsAmbiguousMatchKey(matchKey string) bool {
	return len(r.byMatchKey[strings.TrimSpace(matchKey)]) > 1
}

func (r MonitorResolver) SelectorForOutput(output OutputConfig, monitor hypr.Monitor) string {
	if r.IsAmbiguousMatchKey(output.MatchIdentity()) {
		return monitor.Name
	}
	return monitor.MonitorSelector()
}

// OmittedMonitors returns connected outputs a profile does not mention. A
// profile is an authoritative layout, so these outputs are implicitly off.
// Keeping this resolution here gives config rendering, live apply validation,
// and active-profile detection one definition of "omitted".
func OmittedMonitors(p Profile, monitors []hypr.Monitor) []hypr.Monitor {
	p.Normalize()
	resolver := NewMonitorResolver(monitors)
	claimed := make(map[string]struct{}, len(p.Outputs))
	for _, output := range p.Outputs {
		monitor, ok := resolver.ResolveOutput(output)
		if !ok {
			continue
		}
		claimed[strings.ToLower(strings.TrimSpace(monitor.Name))] = struct{}{}
	}

	omitted := make([]hypr.Monitor, 0, len(monitors)-len(claimed))
	for _, monitor := range monitors {
		if _, ok := claimed[strings.ToLower(strings.TrimSpace(monitor.Name))]; ok {
			continue
		}
		omitted = append(omitted, monitor)
	}
	return omitted
}

type outputIdentityEntry struct {
	OldKey    string
	MatchKey  string
	Name      string
	OutputKey string
}

type outputIdentityIndex struct {
	byKey      map[string]OutputConfig
	byName     map[string]string
	byOldKey   map[string][]string
	byMatchKey map[string][]string
}

func normalizeIdentityRefs(outputs []OutputConfig, workspaces *WorkspaceSettings) {
	if len(outputs) == 0 {
		return
	}

	entries := make([]outputIdentityEntry, len(outputs))
	matchCounts := make(map[string]int, len(outputs))
	for i := range outputs {
		outputs[i].Key = strings.TrimSpace(outputs[i].Key)
		outputs[i].MatchKey = strings.TrimSpace(outputs[i].MatchKey)
		outputs[i].Name = strings.TrimSpace(outputs[i].Name)
		outputs[i].Description = strings.TrimSpace(outputs[i].Description)

		matchKey := outputMatchKeyFromFields(outputs[i])
		if matchKey == "" {
			matchKey = outputs[i].MatchIdentity()
		}
		matchKey = strings.TrimSpace(matchKey)

		entries[i] = outputIdentityEntry{
			OldKey:   outputs[i].Key,
			MatchKey: matchKey,
			Name:     outputs[i].Name,
		}
		matchCounts[matchKey]++
		outputs[i].MatchKey = matchKey
	}

	seenKeys := make(map[string]int, len(outputs))
	for i := range outputs {
		key := hypr.UniqueOutputKey(entries[i].MatchKey, outputDisambiguator(outputs[i], entries[i].OldKey), matchCounts[entries[i].MatchKey])
		if key == "" {
			key = fmt.Sprintf("output-%d", i+1)
		}
		if seen := seenKeys[key]; seen > 0 {
			key = fmt.Sprintf("%s#%d", key, seen+1)
		}
		seenKeys[key]++
		outputs[i].Key = key
		entries[i].OutputKey = key
	}

	index := newOutputIdentityIndex(outputs, entries)
	for i := range outputs {
		if outputs[i].MirrorOf == "" {
			continue
		}
		if resolved := index.resolveReference(outputs[i].MirrorOf, ""); resolved != "" {
			outputs[i].MirrorOf = resolved
		}
	}

	if workspaces == nil {
		return
	}

	for i := range workspaces.Rules {
		rule := &workspaces.Rules[i]
		if resolved := index.resolveReference(rule.OutputKey, rule.OutputName); resolved != "" {
			rule.OutputKey = resolved
		}
		// The key is the display's identity; the connector is only where it
		// was plugged in when the rule was saved. Displays swap connectors,
		// so a key the profile knows always supplies the current one.
		if rule.OutputKey != "" {
			if output, ok := index.byKey[rule.OutputKey]; ok && output.Name != "" {
				rule.OutputName = output.Name
			}
		}
	}

	if len(workspaces.MonitorOrder) > 0 {
		workspaces.MonitorOrder = index.resolveMonitorOrder(workspaces.MonitorOrder)
	}
}

func newOutputIdentityIndex(outputs []OutputConfig, entries []outputIdentityEntry) outputIdentityIndex {
	index := outputIdentityIndex{
		byKey:      make(map[string]OutputConfig, len(outputs)),
		byName:     make(map[string]string, len(outputs)),
		byOldKey:   make(map[string][]string, len(outputs)),
		byMatchKey: make(map[string][]string, len(outputs)),
	}

	for _, idx := range outputIndicesByLayout(outputs) {
		output := outputs[idx]
		entry := entries[idx]
		index.byKey[output.Key] = output
		if output.Name != "" {
			index.byName[output.Name] = output.Key
		}
		if entry.OldKey != "" {
			index.byOldKey[entry.OldKey] = append(index.byOldKey[entry.OldKey], output.Key)
		}
		if entry.MatchKey != "" {
			index.byMatchKey[entry.MatchKey] = append(index.byMatchKey[entry.MatchKey], output.Key)
		}
	}
	return index
}

func (i outputIdentityIndex) resolveReference(refKey string, refName string) string {
	refKey = strings.TrimSpace(refKey)
	refName = strings.TrimSpace(refName)

	if refKey != "" {
		if _, ok := i.byKey[refKey]; ok {
			return refKey
		}
	}

	if refName != "" {
		if refKey != "" {
			if resolved := i.pickByName(i.byOldKey[refKey], refName); resolved != "" {
				return resolved
			}
			if resolved := i.pickByName(i.byMatchKey[refKey], refName); resolved != "" {
				return resolved
			}
		}
		if resolved, ok := i.byName[refName]; ok {
			return resolved
		}
	}

	if refKey != "" {
		if keys := i.byOldKey[refKey]; len(keys) > 0 {
			return keys[0]
		}
		if keys := i.byMatchKey[refKey]; len(keys) > 0 {
			return keys[0]
		}
	}

	return ""
}

func (i outputIdentityIndex) resolveMonitorOrder(order []string) []string {
	if len(order) == 0 {
		return nil
	}

	byOldKey := copyKeyQueues(i.byOldKey)
	byMatchKey := copyKeyQueues(i.byMatchKey)
	seen := make(map[string]bool, len(order))
	resolved := make([]string, 0, len(order))

	for _, ref := range order {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := i.byKey[ref]; ok {
			if !seen[ref] {
				resolved = append(resolved, ref)
				seen[ref] = true
			}
			continue
		}

		key := popFirstUnused(byOldKey, ref, seen)
		if key == "" {
			key = popFirstUnused(byMatchKey, ref, seen)
		}
		if key == "" {
			key = i.resolveReference(ref, "")
		}
		if key == "" || seen[key] {
			continue
		}
		resolved = append(resolved, key)
		seen[key] = true
	}

	for _, idx := range outputIndicesByLayoutFromMap(i.byKey) {
		key := idx
		if output := i.byKey[key]; output.MirrorOf != "" {
			continue
		}
		if seen[key] {
			continue
		}
		resolved = append(resolved, key)
	}
	return resolved
}

func (i outputIdentityIndex) pickByName(keys []string, name string) string {
	name = strings.TrimSpace(name)
	for _, key := range keys {
		output, ok := i.byKey[key]
		if ok && strings.EqualFold(strings.TrimSpace(output.Name), name) {
			return key
		}
	}
	return ""
}

func copyKeyQueues(source map[string][]string) map[string][]string {
	out := make(map[string][]string, len(source))
	for key, values := range source {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func popFirstUnused(queues map[string][]string, ref string, seen map[string]bool) string {
	queue := queues[ref]
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		queues[ref] = queue
		if !seen[key] {
			return key
		}
	}
	return ""
}

func outputIndicesByLayout(outputs []OutputConfig) []int {
	indices := make([]int, len(outputs))
	for idx := range outputs {
		indices[idx] = idx
	}
	sort.SliceStable(indices, func(i, j int) bool {
		left := outputs[indices[i]]
		right := outputs[indices[j]]
		if left.X != right.X {
			return left.X < right.X
		}
		if left.Y != right.Y {
			return left.Y < right.Y
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Key < right.Key
	})
	return indices
}

func outputIndicesByLayoutFromMap(outputs map[string]OutputConfig) []string {
	keys := make([]string, 0, len(outputs))
	for key := range outputs {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left := outputs[keys[i]]
		right := outputs[keys[j]]
		if left.X != right.X {
			return left.X < right.X
		}
		if left.Y != right.Y {
			return left.Y < right.Y
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Key < right.Key
	})
	return keys
}

func outputMatchKeyFromFields(output OutputConfig) string {
	parts := []string{
		cleanProfileIDPart(output.Make),
		cleanProfileIDPart(output.Model),
		cleanProfileIDPart(output.Serial),
	}
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	if len(nonEmpty) > 0 {
		return strings.Join(nonEmpty, "|")
	}
	return ""
}

func outputDisambiguator(output OutputConfig, oldKey string) string {
	matchKey := strings.TrimSpace(output.MatchKey)
	oldKey = strings.TrimSpace(oldKey)
	if matchKey != "" && oldKey != "" {
		prefix := matchKey + "@"
		if strings.HasPrefix(oldKey, prefix) {
			if suffix := strings.TrimSpace(strings.TrimPrefix(oldKey, prefix)); suffix != "" {
				return suffix
			}
		}
	}
	return output.Name
}

func cleanProfileIDPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.Join(strings.Fields(value), " ")
	return value
}
