package tui

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func mainRebuildMonitors() []hypr.Monitor {
	return []hypr.Monitor{
		{Name: "DP-1", Make: "Example", Model: "Left", Serial: "A", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, Focused: true, DPMSStatus: true},
		{Name: "DP-2", Make: "Example", Model: "Right", Serial: "B", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920, DPMSStatus: true},
	}
}

func editorMains(m Model) []bool {
	mains := make([]bool, len(m.editOutputs))
	for i, output := range m.editOutputs {
		mains[i] = output.Main
	}
	return mains
}

func TestLoadLiveStateRestoresSavedMain(t *testing.T) {
	monitors := mainRebuildMonitors()
	saved := profile.FromState("Desk", monitors, nil)
	saved.Outputs[1].Main = true
	m := Model{styles: newStyles(), monitors: monitors, profiles: []profile.Profile{saved}}

	m.loadLiveState()

	if got := editorMains(m); len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("main after rebuild = %v, want [false true]", got)
	}
}

func TestLoadLiveStateKeepsEditorMainAndExplicitOff(t *testing.T) {
	monitors := mainRebuildMonitors()
	saved := profile.FromState("Desk", monitors, nil)
	saved.Outputs[1].Main = true
	m := Model{styles: newStyles(), monitors: monitors, profiles: []profile.Profile{saved}}
	m.loadLiveState()

	// Turning Main off is explicit and must survive a background rebuild.
	m.editOutputs[1].Main = false
	m.loadLiveState()
	if got := editorMains(m); got[0] || got[1] {
		t.Fatalf("explicit Off lost on rebuild: %v", got)
	}

	// Moving Main to another display survives too, still one at most.
	m.editOutputs[0].Main = true
	m.loadLiveState()
	if got := editorMains(m); !got[0] || got[1] {
		t.Fatalf("moved main lost on rebuild: %v", got)
	}
}

func TestPreserveUnreportedSettingsKeepsOneMain(t *testing.T) {
	monitors := mainRebuildMonitors()
	draft := profile.FromState("", monitors, nil)
	previous := profile.FromState("", monitors, nil)
	previous.Outputs[0].Main, previous.Outputs[1].Main = true, true
	profile.PreserveUnreportedSettings(&draft, previous)
	if _, ok := draft.MainOutput(); !ok || !draft.Outputs[0].Main || draft.Outputs[1].Main {
		t.Fatalf("mains = %v,%v; want only the first", draft.Outputs[0].Main, draft.Outputs[1].Main)
	}
}
