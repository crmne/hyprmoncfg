package appstatus

import (
	"sort"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

const SchemaVersion = 1

type Document struct {
	SchemaVersion      int               `json:"schema_version"`
	Version            string            `json:"version"`
	Daemon             Daemon            `json:"daemon"`
	ActiveProfile      *ProfileReference `json:"active_profile"`
	RecommendedProfile *ProfileMatch     `json:"recommended_profile"`
	Profiles           []ProfileSummary  `json:"profiles"`
	Monitors           []MonitorSummary  `json:"monitors"`
}

type Daemon struct {
	Running bool `json:"running"`
	// Unmanaged reports that monitor configuration was handed back to Hyprland.
	// The zero value means managed, so a client that never sets it, or one too
	// old to know the field, reads as the behavior hyprmoncfg has always had.
	Unmanaged bool `json:"unmanaged,omitempty"`
	// ProfileOverride names the profile a person confirmed manually for the
	// current connected monitor set. Empty means automatic best-match selection.
	// The override is intentionally session-scoped and disappears when the
	// hardware set changes or the daemon restarts.
	ProfileOverride string `json:"profile_override,omitempty"`
	// Preview keeps an interactive display change discoverable while the bar is
	// rebuilt for the new monitor layout. A replacement panel can present the
	// same confirmation and reclaim the transaction before its deadline.
	Preview *PreviewReference `json:"preview,omitempty"`
}

type PreviewReference struct {
	TransactionID string          `json:"transaction_id"`
	Reclaimable   bool            `json:"reclaimable"`
	ProfileName   string          `json:"profile_name"`
	Deadline      time.Time       `json:"deadline"`
	SaveOnCommit  bool            `json:"save_on_commit,omitempty"`
	Profile       profile.Profile `json:"profile"`
}

type ProfileReference struct {
	Name string `json:"name"`
}

type ProfileMatch struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

type ProfileSummary struct {
	Name                    string                `json:"name"`
	OutputCount             int                   `json:"output_count"`
	EnabledOutputs          int                   `json:"enabled_outputs"`
	ConnectedOutputs        int                   `json:"connected_outputs"`
	ConnectedEnabledOutputs int                   `json:"connected_enabled_outputs"`
	ExactDisplayMatch       bool                  `json:"exact_display_match"`
	MatchScore              int                   `json:"match_score"`
	MatchReasons            []profile.MatchReason `json:"match_reasons"`
	UpdatedAt               time.Time             `json:"updated_at"`
	Active                  bool                  `json:"active"`
	Recommended             bool                  `json:"recommended"`
}

type MonitorSummary struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Make          string  `json:"make"`
	Model         string  `json:"model"`
	Mode          string  `json:"mode"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	RefreshRate   float64 `json:"refresh_rate"`
	X             int     `json:"x"`
	Y             int     `json:"y"`
	Scale         float64 `json:"scale"`
	Transform     int     `json:"transform"`
	LogicalWidth  int     `json:"logical_width"`
	LogicalHeight int     `json:"logical_height"`
	Internal      bool    `json:"internal"`
	Focused       bool    `json:"focused"`
	Enabled       bool    `json:"enabled"`
	// MirrorOf names the connector this monitor mirrors, empty when it drives
	// its own image. A mirroring monitor shares the position of its source, so
	// anything drawing a layout has to leave it out and name it separately.
	MirrorOf string `json:"mirror_of,omitempty"`
}

// EditorDocument is the richer, on-demand view used by compact graphical
// editors. Status stays deliberately small because it is broadcast whenever
// anything changes; mode lists and a complete profile-shaped draft are only
// fetched when an editor is actually open.
type EditorDocument struct {
	Profile               profile.Profile            `json:"profile"`
	Profiles              []profile.Profile          `json:"profiles"`
	Displays              []EditorDisplay            `json:"displays"`
	WorkspacePlan         []WorkspacePlan            `json:"workspace_plan"`
	ProfileWorkspacePlans map[string][]WorkspacePlan `json:"profile_workspace_plans"`
	SourceProfile         string                     `json:"source_profile,omitempty"`
	SuggestedProfile      string                     `json:"suggested_profile,omitempty"`
}

type EditorDraft struct {
	Profile       profile.Profile `json:"profile"`
	WorkspacePlan []WorkspacePlan `json:"workspace_plan"`
}

type WorkspacePlan struct {
	OutputKey  string   `json:"output_key"`
	OutputName string   `json:"output_name"`
	Workspaces []string `json:"workspaces"`
}

type EditorDisplay struct {
	Key            string    `json:"key"`
	Focused        bool      `json:"focused"`
	Internal       bool      `json:"internal"`
	DPMS           bool      `json:"dpms"`
	PhysicalWidth  int       `json:"physical_width"`
	PhysicalHeight int       `json:"physical_height"`
	Workspace      string    `json:"workspace,omitempty"`
	AvailableModes []string  `json:"available_modes"`
	ScaleOptions   []float64 `json:"scale_options"`
}

// BuildEditor turns live Hyprland state into an editable profile without
// dropping settings Hyprland cannot report back accurately. Those fields are
// recovered from the exact active profile, or from the best hardware match
// when the live layout is custom. This lets a compact editor change scale or
// position without silently erasing HDR, ICC, or luminance configuration.
func BuildEditor(profiles []profile.Profile, monitors []hypr.Monitor, rules []hypr.WorkspaceRule) EditorDocument {
	draft, sourceName, suggestedName := profile.EditorProfileFromState(profiles, monitors, rules)
	document := EditorDocument{
		Profile:               draft,
		Profiles:              append([]profile.Profile{}, profiles...),
		Displays:              make([]EditorDisplay, 0, len(monitors)),
		WorkspacePlan:         BuildEditorDraft(draft).WorkspacePlan,
		ProfileWorkspacePlans: make(map[string][]WorkspacePlan, len(profiles)),
		SourceProfile:         sourceName,
		SuggestedProfile:      suggestedName,
	}
	for _, saved := range profiles {
		document.ProfileWorkspacePlans[saved.Name] = BuildEditorDraft(saved).WorkspacePlan
	}

	matchCounts := hypr.MonitorMatchCounts(monitors)
	for _, monitor := range monitors {
		modes := append([]string(nil), monitor.AvailableModes...)
		current := monitor.ModeString()
		found := false
		for _, mode := range modes {
			if mode == current {
				found = true
				break
			}
		}
		if current != "" && !found {
			modes = append([]string{current}, modes...)
		}
		document.Displays = append(document.Displays, EditorDisplay{
			Key:            hypr.MonitorOutputKey(monitor, matchCounts),
			Focused:        monitor.Focused,
			Internal:       monitor.IsInternal(),
			DPMS:           monitor.DPMSStatus,
			PhysicalWidth:  monitor.PhysicalWidth,
			PhysicalHeight: monitor.PhysicalHeight,
			Workspace:      monitor.ActiveWorkspace.Name,
			AvailableModes: modes,
			ScaleOptions:   editorScaleOptions(monitor.Width, monitor.Height, monitor.Scale),
		})
	}

	return document
}

func BuildEditorDraft(draft profile.Profile) EditorDraft {
	plan := make([]WorkspacePlan, 0, len(draft.Outputs))
	byKey := make(map[string]int, len(draft.Outputs))
	for _, output := range draft.Outputs {
		if !output.Enabled || output.MirrorOf != "" {
			continue
		}
		byKey[output.Key] = len(plan)
		plan = append(plan, WorkspacePlan{
			OutputKey:  output.Key,
			OutputName: output.Name,
			Workspaces: []string{},
		})
	}
	for _, rule := range profile.ResolveWorkspaceRules(draft, nil) {
		if index, ok := byKey[rule.OutputKey]; ok {
			plan[index].Workspaces = append(plan[index].Workspaces, rule.Workspace)
		}
	}
	return EditorDraft{Profile: draft, WorkspacePlan: plan}
}

func editorScaleOptions(width, height int, current float64) []float64 {
	options := scaling.GridScales(width, height, 1, scaling.MaxScale)
	candidate := scaling.Round(current)
	if !scaling.Sharp(width, height, candidate) {
		var ok bool
		candidate, ok = scaling.ClosestSharp(width, height, candidate)
		if !ok {
			return options
		}
	}
	for _, option := range options {
		if option == candidate {
			return options
		}
	}
	options = append(options, candidate)
	sort.Float64s(options)
	return options
}

func Build(version string, daemonRunning bool, profiles []profile.Profile, monitors []hypr.Monitor, rules []hypr.WorkspaceRule) Document {
	document := Document{
		SchemaVersion: SchemaVersion,
		Version:       version,
		Daemon:        Daemon{Running: daemonRunning},
		Profiles:      make([]ProfileSummary, 0, len(profiles)),
		Monitors:      make([]MonitorSummary, 0, len(monitors)),
	}

	activeName := ""
	if active, ok := profile.ExactStateMatch(profiles, monitors, rules); ok {
		activeName = active.Name
		document.ActiveProfile = &ProfileReference{Name: active.Name}
	}

	recommendedName := ""
	if recommended, score, ok := profile.BestMatch(profiles, monitors); ok {
		recommendedName = recommended.Name
		document.RecommendedProfile = &ProfileMatch{Name: recommended.Name, Score: score}
	}

	for _, saved := range profiles {
		match := profile.EvaluateMatch(saved, monitors)
		enabledOutputs := 0
		for _, output := range saved.Outputs {
			if output.Enabled {
				enabledOutputs++
			}
		}
		document.Profiles = append(document.Profiles, ProfileSummary{
			Name:                    saved.Name,
			OutputCount:             len(saved.Outputs),
			EnabledOutputs:          enabledOutputs,
			ConnectedOutputs:        match.EnabledMatched + match.DisabledMatched,
			ConnectedEnabledOutputs: match.ConnectedEnabledOutputs,
			ExactDisplayMatch:       match.ExactDisplayMatch(),
			MatchScore:              match.Score,
			MatchReasons:            profile.ExplainMatch(match),
			UpdatedAt:               saved.UpdatedAt,
			Active:                  saved.Name == activeName,
			Recommended:             saved.Name == recommendedName,
		})
	}

	for _, monitor := range monitors {
		logicalWidth, logicalHeight := monitor.LogicalSize()
		document.Monitors = append(document.Monitors, MonitorSummary{
			Name:          monitor.Name,
			Description:   monitor.Description,
			Make:          monitor.Make,
			Model:         monitor.Model,
			Mode:          monitor.ModeString(),
			Width:         monitor.Width,
			Height:        monitor.Height,
			RefreshRate:   monitor.RefreshRate,
			X:             monitor.X,
			Y:             monitor.Y,
			Scale:         monitor.Scale,
			Transform:     monitor.Transform,
			LogicalWidth:  logicalWidth,
			LogicalHeight: logicalHeight,
			Internal:      monitor.IsInternal(),
			Focused:       monitor.Focused,
			Enabled:       !monitor.Disabled,
			MirrorOf:      monitor.MirrorOf,
		})
	}

	return document
}
