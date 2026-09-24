// capture-fixture creates isolated, synthetic input for documentation captures.
// It never contacts Hyprland or the daemon. The destination must not exist.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run ./scripts/capture-fixture NEW_DIRECTORY")
	}
	dir := os.Args[1]
	must(os.Mkdir(dir, 0700))
	monitors := []hypr.Monitor{
		{ID: 0, Name: "DP-1", Make: "Example", Model: "Studio", Serial: "DEMO-STUDIO", Description: "Example Studio",
			Width: 3840, Height: 2160, RefreshRate: 144, Scale: 1.5, Focused: true, DPMSStatus: true,
			PhysicalWidth: 710, PhysicalHeight: 400, AvailableModes: []string{hypr.FormatMode(3840, 2160, 144), hypr.FormatMode(3840, 2160, 60), hypr.FormatMode(2560, 1440, 60)}},
		{ID: 1, Name: "DP-2", Make: "Example", Model: "Portable", Serial: "DEMO-PORTABLE", Description: "Example Portable",
			Width: 2560, Height: 1600, RefreshRate: 60, Scale: 1.6, X: 2560, Y: 220, DPMSStatus: true,
			PhysicalWidth: 340, PhysicalHeight: 210, AvailableModes: []string{hypr.FormatMode(2560, 1600, 60), hypr.FormatMode(1920, 1200, 60)}},
	}
	rules := []hypr.WorkspaceRule{}
	for id := 1; id <= 6; id++ {
		name := "DP-1"
		if id > 3 {
			name = "DP-2"
		}
		rules = append(rules, hypr.WorkspaceRule{WorkspaceString: fmt.Sprint(id), Monitor: name, Default: id == 1 || id == 4, Persistent: id == 1 || id == 4})
	}
	desk := profile.FromState("Desk", monitors, rules)
	desk.Workspaces = profile.WorkspaceSettings{Enabled: true, Strategy: profile.WorkspaceStrategySequential,
		MaxWorkspaces: 6, GroupSize: 3, MonitorOrder: []string{monitors[0].HardwareKey(), monitors[1].HardwareKey()}}
	focus := profile.FromState("Focus", monitors, rules)
	focus.Workspaces = desk.Workspaces
	for i := range focus.Outputs {
		if focus.Outputs[i].Name == "DP-2" {
			focus.Outputs[i].Enabled = false
		}
	}
	focus.Normalize()
	profiles := []profile.Profile{desk, focus}
	store := profile.NewStore(filepath.Join(dir, "config"))
	for _, p := range profiles {
		must(store.Save(p))
	}
	profiles, err := store.List()
	must(err)
	writeJSON(dir, "monitors.json", monitors)
	writeJSON(dir, "workspacerules.json", rules)
	writeJSON(dir, "workspaces.json", []hypr.WorkspaceState{})
	writeJSON(dir, "status.json", appstatus.Build("1.19.0", true, profiles, monitors, rules))
	writeJSON(dir, "editor.json", appstatus.BuildEditor(profiles, monitors, rules))
	must(os.Mkdir(filepath.Join(dir, "runtime"), 0700))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func writeJSON(dir, name string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0600))
}
