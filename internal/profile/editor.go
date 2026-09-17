package profile

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

type EditorEdit struct {
	DisableUnknownOutputs *bool              `json:"disable_unknown_outputs,omitempty"`
	OutputKey             string             `json:"output_key,omitempty"`
	Enabled               *bool              `json:"enabled,omitempty"`
	Mode                  *string            `json:"mode,omitempty"`
	Scale                 *float64           `json:"scale,omitempty"`
	VRR                   *int               `json:"vrr,omitempty"`
	Transform             *int               `json:"transform,omitempty"`
	MirrorOf              *string            `json:"mirror_of,omitempty"`
	X                     *int               `json:"x,omitempty"`
	Y                     *int               `json:"y,omitempty"`
	Bitdepth              *int               `json:"bitdepth,omitempty"`
	CM                    *string            `json:"cm,omitempty"`
	SDRBrightness         *float64           `json:"sdr_brightness,omitempty"`
	SDRSaturation         *float64           `json:"sdr_saturation,omitempty"`
	SDRMinLum             *float64           `json:"sdr_min_luminance,omitempty"`
	SDRMaxLum             *int               `json:"sdr_max_luminance,omitempty"`
	SDREOTF               *string            `json:"sdr_eotf,omitempty"`
	MinLuminance          *float64           `json:"min_luminance,omitempty"`
	MaxLuminance          *int               `json:"max_luminance,omitempty"`
	MaxAvgLum             *int               `json:"max_avg_luminance,omitempty"`
	ForceWide             *int               `json:"supports_wide_color,omitempty"`
	ForceHDR              *int               `json:"supports_hdr,omitempty"`
	ICC                   *string            `json:"icc,omitempty"`
	SnapDistance          int                `json:"snap_distance,omitempty"`
	Workspaces            *WorkspaceSettings `json:"workspaces,omitempty"`
}

type SnapEdge int

const (
	SnapEdgeLeft SnapEdge = iota
	SnapEdgeRight
	SnapEdgeTop
	SnapEdgeBottom
)

type SnapMark struct {
	OutputIndex int
	Edge        SnapEdge
}

type SnapAxisCandidate struct {
	Pos   int
	Dist  int
	Marks []SnapMark
}

type SnapAnalysis struct {
	X SnapAxisCandidate
	Y SnapAxisCandidate
}

// EditorProfileFromState builds the common draft both interactive frontends
// edit. Live state remains authoritative for observable display properties;
// profile-only settings are recovered from the exact active profile, or from
// the best hardware match when the layout is custom.
func EditorProfileFromState(profiles []Profile, monitors []hypr.Monitor, rules []hypr.WorkspaceRule) (draft Profile, sourceName, suggestedName string) {
	draft = FromState("", monitors, rules)

	var source *Profile
	if active, ok := ExactStateMatch(profiles, monitors, rules); ok {
		sourceName = active.Name
		suggestedName = active.Name
		draft.Name = active.Name
		draft.CreatedAt = active.CreatedAt
		draft.UpdatedAt = active.UpdatedAt
		draft.Workspaces = active.Workspaces
		draft.Exec = active.Exec
		source = &active
	} else if suggested, _, ok := BestMatch(profiles, monitors); ok {
		suggestedName = suggested.Name
		source = &suggested
	}

	if source != nil {
		PreserveUnreportedSettings(&draft, *source)
	}
	if sourceName != "" {
		PreserveExactScales(&draft, *source)
	}
	return draft, sourceName, suggestedName
}

// PreserveUnreportedSettings carries across profile fields Hyprland cannot
// accurately return from its live monitor state. Keeping this list in the
// profile package prevents the TUI, daemon integrations, and future graphical
// clients from gradually disagreeing about what a layout-only edit preserves.
func PreserveUnreportedSettings(draft *Profile, saved Profile) {
	if draft == nil {
		return
	}
	draft.DisableUnknownOutputs = saved.DisableUnknownOutputs
	for idx := range draft.Outputs {
		stored, ok := saved.OutputByKey(draft.Outputs[idx].Key)
		if !ok {
			continue
		}
		output := &draft.Outputs[idx]
		output.VRR = stored.VRR
		output.Bitdepth = stored.Bitdepth
		output.CM = stored.CM
		output.MinLuminance = stored.MinLuminance
		output.MaxLuminance = stored.MaxLuminance
		output.SupportsWideColor = stored.SupportsWideColor
		output.SupportsHDR = stored.SupportsHDR
		output.MaxAvgLuminance = stored.MaxAvgLuminance
		output.SDREOTF = stored.SDREOTF
		output.ICC = stored.ICC
	}
}

// PreserveExactScales restores a saved sharp scale after Hyprland rounds its
// readback to two decimal places. It is only safe for an exact state match.
func PreserveExactScales(draft *Profile, saved Profile) {
	if draft == nil {
		return
	}
	for idx := range draft.Outputs {
		output := &draft.Outputs[idx]
		if !output.Enabled {
			continue
		}
		stored, ok := saved.OutputByKey(output.Key)
		if !ok || !stored.Enabled {
			continue
		}
		if ScaleMatchesRoundedReadback(output.Width, output.Height, stored.Scale, output.Scale) {
			output.Scale = stored.Scale
		}
	}
}

// ApplyEditorEdit is the shared mutation boundary for graphical editors. The
// daemon exposes it over IPC while the TUI can call the geometry helpers below
// directly when it is running without a daemon.
func ApplyEditorEdit(draft Profile, edit EditorEdit) (Profile, error) {
	draft.Normalize()
	if edit.DisableUnknownOutputs != nil {
		draft.DisableUnknownOutputs = *edit.DisableUnknownOutputs
	}
	if edit.Workspaces != nil {
		if err := edit.Workspaces.Validate(); err != nil {
			return Profile{}, err
		}
		draft.Workspaces = *edit.Workspaces
	}

	if strings.TrimSpace(edit.OutputKey) != "" {
		index := -1
		for idx := range draft.Outputs {
			if draft.Outputs[idx].Key == edit.OutputKey {
				index = idx
				break
			}
		}
		if index < 0 {
			return Profile{}, fmt.Errorf("unknown output %q", edit.OutputKey)
		}

		output := &draft.Outputs[index]
		oldWidth, oldHeight := output.LogicalSize()
		if edit.Enabled != nil {
			output.Enabled = *edit.Enabled
		}
		if edit.Mode != nil {
			mode := strings.TrimSpace(*edit.Mode)
			width, height, refresh, ok := hypr.ParseMode(mode)
			if !ok {
				return Profile{}, fmt.Errorf("invalid display mode %q", mode)
			}
			output.Mode = mode
			output.Width = width
			output.Height = height
			output.Refresh = refresh
		}
		if edit.Scale != nil {
			output.Scale = scaling.Round(scaling.Clamp(*edit.Scale))
		}
		if edit.VRR != nil {
			if *edit.VRR < 0 || *edit.VRR > 2 {
				return Profile{}, fmt.Errorf("invalid VRR mode %d", *edit.VRR)
			}
			output.VRR = *edit.VRR
		}
		if edit.Transform != nil {
			if *edit.Transform < 0 || *edit.Transform > 7 {
				return Profile{}, fmt.Errorf("invalid transform %d", *edit.Transform)
			}
			output.Transform = *edit.Transform
		}
		if edit.MirrorOf != nil {
			target := strings.TrimSpace(*edit.MirrorOf)
			if target == output.Key {
				return Profile{}, errors.New("a display cannot mirror itself")
			}
			if target != "" {
				if _, ok := draft.OutputByKey(target); !ok {
					return Profile{}, fmt.Errorf("unknown mirror target %q", target)
				}
			}
			output.MirrorOf = target
		}
		if edit.X != nil {
			output.X = *edit.X
		}
		if edit.Y != nil {
			output.Y = *edit.Y
		}
		if edit.Bitdepth != nil {
			if *edit.Bitdepth != 8 && *edit.Bitdepth != 10 {
				return Profile{}, fmt.Errorf("invalid bit depth %d", *edit.Bitdepth)
			}
			output.Bitdepth = *edit.Bitdepth
		}
		if edit.CM != nil {
			value := strings.TrimSpace(*edit.CM)
			if !editorStringOption(value, "", "srgb", "auto", "wide", "hdr", "hdredid", "dcip3", "dp3", "adobe", "edid") {
				return Profile{}, fmt.Errorf("invalid color management preset %q", value)
			}
			output.CM = value
		}
		if edit.SDRBrightness != nil {
			if *edit.SDRBrightness < 0 || *edit.SDRBrightness > 3 {
				return Profile{}, errors.New("SDR brightness must be between 0 and 3")
			}
			output.SDRBrightness = *edit.SDRBrightness
		}
		if edit.SDRSaturation != nil {
			if *edit.SDRSaturation < 0 || *edit.SDRSaturation > 3 {
				return Profile{}, errors.New("SDR saturation must be between 0 and 3")
			}
			output.SDRSaturation = *edit.SDRSaturation
		}
		if edit.SDRMinLum != nil {
			if *edit.SDRMinLum < 0 || *edit.SDRMinLum > 1 {
				return Profile{}, errors.New("SDR minimum luminance must be between 0 and 1")
			}
			output.SDRMinLuminance = *edit.SDRMinLum
		}
		if edit.SDRMaxLum != nil {
			if *edit.SDRMaxLum < 0 || *edit.SDRMaxLum > 1000 {
				return Profile{}, errors.New("SDR maximum luminance must be between 0 and 1000")
			}
			output.SDRMaxLuminance = *edit.SDRMaxLum
		}
		if edit.SDREOTF != nil {
			value := strings.TrimSpace(*edit.SDREOTF)
			if !editorStringOption(value, "", "default", "gamma22", "srgb") {
				return Profile{}, fmt.Errorf("invalid SDR curve %q", value)
			}
			output.SDREOTF = value
		}
		if edit.MinLuminance != nil {
			if *edit.MinLuminance < 0 || *edit.MinLuminance > 1000 {
				return Profile{}, errors.New("minimum luminance must be between 0 and 1000")
			}
			output.MinLuminance = *edit.MinLuminance
		}
		if edit.MaxLuminance != nil {
			if *edit.MaxLuminance < 0 || *edit.MaxLuminance > 2000 {
				return Profile{}, errors.New("maximum luminance must be between 0 and 2000")
			}
			output.MaxLuminance = *edit.MaxLuminance
		}
		if edit.MaxAvgLum != nil {
			if *edit.MaxAvgLum < 0 || *edit.MaxAvgLum > 2000 {
				return Profile{}, errors.New("maximum average luminance must be between 0 and 2000")
			}
			output.MaxAvgLuminance = *edit.MaxAvgLum
		}
		if edit.ForceWide != nil {
			if *edit.ForceWide < -1 || *edit.ForceWide > 1 {
				return Profile{}, fmt.Errorf("invalid wide color override %d", *edit.ForceWide)
			}
			output.SupportsWideColor = *edit.ForceWide
		}
		if edit.ForceHDR != nil {
			if *edit.ForceHDR < -1 || *edit.ForceHDR > 1 {
				return Profile{}, fmt.Errorf("invalid HDR override %d", *edit.ForceHDR)
			}
			output.SupportsHDR = *edit.ForceHDR
		}
		if edit.ICC != nil {
			value := strings.TrimSpace(*edit.ICC)
			if value != "" && !filepath.IsAbs(value) {
				return Profile{}, errors.New("ICC profile path must be absolute")
			}
			output.ICC = value
		}

		ReflowAfterResize(draft.Outputs, index, oldWidth, oldHeight)
		if (edit.X != nil || edit.Y != nil) && edit.SnapDistance > 0 {
			ApplySnap(draft.Outputs, index, edit.SnapDistance)
			PlaceOutsideOverlaps(draft.Outputs, index)
		}
	}

	enabled := 0
	for _, output := range draft.Outputs {
		if output.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return Profile{}, errors.New("at least one display must stay enabled")
	}
	if err := ValidateLayout(draft.Outputs); err != nil {
		return Profile{}, err
	}
	draft.Normalize()
	return draft, nil
}

func editorStringOption(value string, allowed ...string) bool {
	for _, option := range allowed {
		if value == option {
			return true
		}
	}
	return false
}

func (o OutputConfig) LogicalSize() (int, int) {
	scale := scaling.Round(scaling.Clamp(o.Scale))
	width := int(math.Round(float64(o.Width) / scale))
	height := int(math.Round(float64(o.Height) / scale))
	if o.Transform%2 == 1 {
		width, height = height, width
	}
	return max(1, width), max(1, height)
}

// ReflowAfterResize keeps flush neighbors and deliberate gaps intact when a
// mode, scale, or transform changes an output's logical size.
func ReflowAfterResize(outputs []OutputConfig, index, oldWidth, oldHeight int) {
	if index < 0 || index >= len(outputs) || !outputs[index].Enabled {
		return
	}
	resized := outputs[index]
	newWidth, newHeight := resized.LogicalSize()
	dx := newWidth - oldWidth
	dy := newHeight - oldHeight
	oldRight := resized.X + oldWidth
	oldBottom := resized.Y + oldHeight
	for idx := range outputs {
		if idx == index || !outputs[idx].Enabled {
			continue
		}
		if dx != 0 && outputs[idx].X >= oldRight {
			outputs[idx].X += dx
		}
		if dy != 0 && outputs[idx].Y >= oldBottom {
			outputs[idx].Y += dy
		}
	}
}

func AnalyzeSnap(outputs []OutputConfig, selectedIndex, threshold int) SnapAnalysis {
	analysis := SnapAnalysis{
		X: SnapAxisCandidate{Dist: threshold + 1},
		Y: SnapAxisCandidate{Dist: threshold + 1},
	}
	if selectedIndex < 0 || selectedIndex >= len(outputs) || !outputs[selectedIndex].Enabled {
		return analysis
	}
	selected := outputs[selectedIndex]
	width, height := selected.LogicalSize()
	analysis.X.Pos = selected.X
	analysis.Y.Pos = selected.Y

	considerX := func(pos int, marks ...SnapMark) {
		distance := absInt(selected.X - pos)
		if distance < analysis.X.Dist {
			analysis.X = SnapAxisCandidate{Pos: pos, Dist: distance, Marks: append([]SnapMark(nil), marks...)}
		}
	}
	considerY := func(pos int, marks ...SnapMark) {
		distance := absInt(selected.Y - pos)
		if distance < analysis.Y.Dist {
			analysis.Y = SnapAxisCandidate{Pos: pos, Dist: distance, Marks: append([]SnapMark(nil), marks...)}
		}
	}

	for idx, other := range outputs {
		if idx == selectedIndex || !other.Enabled || other.MirrorOf != "" {
			continue
		}
		otherWidth, otherHeight := other.LogicalSize()
		if spansOverlap(selected.Y, selected.Y+height, other.Y, other.Y+otherHeight) {
			considerX(other.X-width, SnapMark{selectedIndex, SnapEdgeRight}, SnapMark{idx, SnapEdgeLeft})
			considerX(other.X+otherWidth, SnapMark{selectedIndex, SnapEdgeLeft}, SnapMark{idx, SnapEdgeRight})
		}
		considerX(other.X, SnapMark{selectedIndex, SnapEdgeLeft}, SnapMark{idx, SnapEdgeLeft})
		considerX(other.X+otherWidth-width, SnapMark{selectedIndex, SnapEdgeRight}, SnapMark{idx, SnapEdgeRight})
		if spansOverlap(selected.X, selected.X+width, other.X, other.X+otherWidth) {
			considerY(other.Y-height, SnapMark{selectedIndex, SnapEdgeBottom}, SnapMark{idx, SnapEdgeTop})
			considerY(other.Y+otherHeight, SnapMark{selectedIndex, SnapEdgeTop}, SnapMark{idx, SnapEdgeBottom})
		}
		considerY(other.Y, SnapMark{selectedIndex, SnapEdgeTop}, SnapMark{idx, SnapEdgeTop})
		considerY(other.Y+otherHeight-height, SnapMark{selectedIndex, SnapEdgeBottom}, SnapMark{idx, SnapEdgeBottom})
	}
	considerX(0)
	considerY(0)
	return analysis
}

func ApplySnap(outputs []OutputConfig, selectedIndex, threshold int) SnapAnalysis {
	analysis := AnalyzeSnap(outputs, selectedIndex, threshold)
	if selectedIndex < 0 || selectedIndex >= len(outputs) {
		return analysis
	}
	if analysis.X.Dist <= threshold {
		outputs[selectedIndex].X = analysis.X.Pos
	}
	if analysis.Y.Dist <= threshold {
		outputs[selectedIndex].Y = analysis.Y.Pos
	}
	return analysis
}

// PlaceOutsideOverlaps turns a pointer drop on top of another display into
// the nearest valid edge placement. Drag-and-drop editors should not require
// pixel-perfect releases or surface a validation error for a collision they
// can resolve unambiguously. Typed position edits still use ValidateLayout so
// callers that need exact coordinates do not have those coordinates changed.
func PlaceOutsideOverlaps(outputs []OutputConfig, selectedIndex int) bool {
	if selectedIndex < 0 || selectedIndex >= len(outputs) {
		return false
	}
	selected := outputs[selectedIndex]
	if !selected.Enabled || selected.MirrorOf != "" || !outputOverlapsAny(outputs, selectedIndex, selected.X, selected.Y) {
		return false
	}

	width, height := selected.LogicalSize()
	type candidate struct {
		x, y     int
		distance int64
	}
	candidates := make([]candidate, 0, (len(outputs)-1)*4)
	add := func(x, y int) {
		if outputOverlapsAny(outputs, selectedIndex, x, y) {
			return
		}
		dx := int64(x - selected.X)
		dy := int64(y - selected.Y)
		candidates = append(candidates, candidate{x: x, y: y, distance: dx*dx + dy*dy})
	}

	for index, other := range outputs {
		if index == selectedIndex || !other.Enabled || other.MirrorOf != "" {
			continue
		}
		otherWidth, otherHeight := other.LogicalSize()
		add(other.X-width, selected.Y)
		add(other.X+otherWidth, selected.Y)
		add(selected.X, other.Y-height)
		add(selected.X, other.Y+otherHeight)
	}
	if len(candidates) == 0 {
		return false
	}

	best := candidates[0]
	for _, option := range candidates[1:] {
		if option.distance < best.distance {
			best = option
		}
	}
	outputs[selectedIndex].X = best.x
	outputs[selectedIndex].Y = best.y
	return true
}

func outputOverlapsAny(outputs []OutputConfig, selectedIndex, x, y int) bool {
	selected := outputs[selectedIndex]
	width, height := selected.LogicalSize()
	for index, other := range outputs {
		if index == selectedIndex || !other.Enabled || other.MirrorOf != "" {
			continue
		}
		otherWidth, otherHeight := other.LogicalSize()
		if x < other.X+otherWidth && x+width > other.X &&
			y < other.Y+otherHeight && y+height > other.Y {
			return true
		}
	}
	return false
}

func ValidateLayout(outputs []OutputConfig) error {
	type rectangle struct {
		name   string
		x1, y1 int
		x2, y2 int
	}
	rectangles := make([]rectangle, 0, len(outputs))
	for _, output := range outputs {
		if !output.Enabled || output.MirrorOf != "" {
			continue
		}
		width, height := output.LogicalSize()
		name := strings.TrimSpace(output.Name)
		if name == "" {
			name = strings.TrimSpace(output.Key)
		}
		if name == "" {
			name = "display"
		}
		rectangles = append(rectangles, rectangle{name, output.X, output.Y, output.X + width, output.Y + height})
	}
	for left := 0; left < len(rectangles); left++ {
		for right := left + 1; right < len(rectangles); right++ {
			if rectangles[left].x1 < rectangles[right].x2 && rectangles[left].x2 > rectangles[right].x1 &&
				rectangles[left].y1 < rectangles[right].y2 && rectangles[left].y2 > rectangles[right].y1 {
				return fmt.Errorf("layout overlaps: %s intersects %s", rectangles[left].name, rectangles[right].name)
			}
		}
	}
	return nil
}

func spansOverlap(a1, a2, b1, b2 int) bool { return a1 < b2 && a2 > b1 }

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
