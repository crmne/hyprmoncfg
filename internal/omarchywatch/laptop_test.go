package omarchywatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestLaptopToggleSharesNativeIntentAndRollsBack(t *testing.T) {
	l := &LaptopToggle{path: filepath.Join(t.TempDir(), "toggles", "internal-monitor-disable.lua")}
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Laptop", Model: "Panel", Width: 1920, Height: 1080, Scale: 1},
		{Name: "DP-1", Make: "Dell", Model: "External", Width: 1920, Height: 1080, X: 1920, Scale: 1},
	}
	p := profile.FromMonitors("desk", monitors)
	for i := range p.Outputs {
		if p.Outputs[i].Name == "eDP-1" {
			p.Outputs[i].Enabled = false
		}
	}
	snapshot, err := l.Sync(p, monitors)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(l.path); err != nil || !strings.Contains(string(got), `output = "eDP-1", disabled = true`) {
		t.Fatalf("native wake guard missing: %s (%v)", got, err)
	}
	if _, changed, err := l.Changed(); err != nil || changed {
		t.Fatal("mistook our own apply for a user toggle")
	}
	// The native Laptop Display action removes this very file to enable it.
	if err := os.Remove(l.path); err != nil {
		t.Fatal(err)
	}
	if disabled, changed, err := l.Changed(); err != nil || disabled || !changed {
		t.Fatal("missed the native enable request")
	}
	l.Reset()
	if _, changed, err := l.Changed(); err != nil || changed {
		t.Fatal("treated resume recovery as a user preference")
	}
	if err := l.Restore(snapshot); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := l.Changed(); err != nil || changed {
		t.Fatal("rollback manufactured another toggle")
	}
}

func TestLaptopToggleIsClearedForAnUndockedLaptop(t *testing.T) {
	l := &LaptopToggle{path: filepath.Join(t.TempDir(), "internal-monitor-disable.lua")}
	if err := os.WriteFile(l.path, []byte("previous flag"), 0o644); err != nil {
		t.Fatal(err)
	}
	monitors := []hypr.Monitor{{Name: "eDP-1", Make: "Laptop", Model: "Panel", Width: 1920, Height: 1080, Scale: 1}}
	snapshot, err := l.Sync(profile.FromMonitors("laptop", monitors), monitors)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.path); !os.IsNotExist(err) {
		t.Fatal("left the sole screen disabled")
	}
	if err := l.Restore(snapshot); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(l.path); err != nil || string(got) != "previous flag" {
		t.Fatal("did not restore the original native flag")
	}
}

func TestLaptopToggleUsesOmarchysActualStatePath(t *testing.T) {
	withFakeClamshell(t)
	t.Setenv("HOME", t.TempDir())
	l := NewLaptopToggle()
	if l == nil || l.path != filepath.Join(os.Getenv("HOME"), ".local/state/omarchy/toggles/hypr/internal-monitor-disable.lua") {
		t.Fatalf("wrong Omarchy toggle path: %+v", l)
	}
	t.Setenv("PATH", t.TempDir())
	if NewLaptopToggle() != nil {
		t.Fatal("enabled Omarchy integration on a non-Omarchy system")
	}
}
