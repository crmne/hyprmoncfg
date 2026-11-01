package omarchywatch

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestPanelSettingsReportsOnlyFreshFieldChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings")
	settings := &PanelSettings{path: path}
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if changes, err := settings.Changes(); err != nil || len(changes) != 0 {
		t.Fatalf("missing file: %v %v", changes, err)
	}
	write("1920x1080@60\t0x0\t1\t0\tDesk\n")
	changes, err := settings.Changes()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]PanelEdit{"Desk": {Mode: "1920x1080@60", Position: "0x0", Scale: "1", Transform: "0"}}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("new line: %#v", changes)
	}
	write("1920x1080@60\t100x0\t1\t0\tDesk\n")
	changes, _ = settings.Changes()
	if !reflect.DeepEqual(changes, map[string]PanelEdit{"Desk": {Position: "100x0"}}) {
		t.Fatalf("only the position changed: %#v", changes)
	}
	if changes, _ = settings.Changes(); len(changes) != 0 {
		t.Fatalf("no change: %#v", changes)
	}
}

func TestPanelSettingsFirstReadIsBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings")
	if err := os.WriteFile(path, []byte("\t5x5\t2\t\tDesk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings := &PanelSettings{path: path}
	if changes, err := settings.Changes(); err != nil || len(changes) != 0 {
		t.Fatalf("old settings are not edits: %v %v", changes, err)
	}
}

func TestNilPanelSettingsHasNoChanges(t *testing.T) {
	var settings *PanelSettings
	if changes, err := settings.Changes(); err != nil || changes != nil {
		t.Fatalf("%v %v", changes, err)
	}
}

func TestImportPanelEdits(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Desk", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
		{Name: "DP-2", Description: "Twin", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
		{Name: "DP-3", Description: "Twin", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
	}
	base := profile.FromMonitors("Home", monitors)

	got, imported, skipped, _ := ImportPanelEdits(base, monitors, map[string]PanelEdit{
		"Desk": {Mode: "2560x1440@144", Position: "-100x20", Scale: "1.5", Transform: "1"},
	})
	if len(imported) != 1 || len(skipped) != 0 {
		t.Fatalf("imported=%v skipped=%v", imported, skipped)
	}
	var desk profile.OutputConfig
	for _, out := range got.Outputs {
		if out.Name == "DP-1" {
			desk = out
		}
	}
	if desk.Width != 2560 || desk.Height != 1440 || desk.Refresh != 144 || desk.X != -100 || desk.Y != 20 || desk.Scale != 1.5 || desk.Transform != 1 {
		t.Fatalf("%+v", desk)
	}
	if base.Outputs[0].Scale != 1 {
		t.Fatal("the input profile changed")
	}

	if _, imported, skipped, _ = ImportPanelEdits(base, monitors, map[string]PanelEdit{"Twin #2": {Scale: "2"}}); len(imported) != 0 || len(skipped) != 1 {
		t.Fatalf("identical monitors: imported=%v skipped=%v", imported, skipped)
	}
	_, imported, skipped, absent := ImportPanelEdits(base, monitors, map[string]PanelEdit{"Gone": {Scale: "2"}})
	if len(imported) != 0 || len(skipped) != 0 || absent["Gone"].Scale != "2" {
		t.Fatalf("absent monitor: imported=%v skipped=%v absent=%v", imported, skipped, absent)
	}

	auto := base
	auto.Outputs = append([]profile.OutputConfig(nil), base.Outputs...)
	for i := range auto.Outputs {
		auto.Outputs[i].RotationAuto = true
	}
	if _, imported, _, _ = ImportPanelEdits(auto, monitors, map[string]PanelEdit{"Desk": {Transform: "1"}}); len(imported) != 0 {
		t.Fatalf("automatic rotation keeps its transform: %v", imported)
	}
}

func TestHeldEditsWaitForTheirMonitor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings")
	settings := &PanelSettings{path: path}
	if _, err := settings.Changes(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("\t5x5\t2\t\tDesk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, _ := settings.Changes()
	settings.Hold(changes)
	if err := os.WriteFile(path, []byte("\t9x9\t2\t\tDesk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, _ = settings.Changes()
	if want := (PanelEdit{Position: "9x9", Scale: "2"}); changes["Desk"] != want {
		t.Fatalf("held edit merged with the newer one: %#v", changes["Desk"])
	}
	if changes, _ = settings.Changes(); len(changes) != 0 {
		t.Fatalf("an edit is delivered once: %#v", changes)
	}
}
