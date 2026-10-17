package lid

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestParseACPIState(t *testing.T) {
	tests := []struct {
		value string
		want  State
	}{
		{value: "state:      open\n", want: Open},
		{value: "state:      closed\n", want: Closed},
		{value: "available:  yes\n", want: Unknown},
	}

	for _, tt := range tests {
		if got := parseACPIState(tt.value); got != tt.want {
			t.Fatalf("parseACPIState(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestStateFromPropertiesSignal(t *testing.T) {
	signal := &dbus.Signal{
		Path: upowerPath,
		Name: propertiesSignal,
		Body: []any{
			upowerInterface,
			map[string]dbus.Variant{
				"LidIsClosed": dbus.MakeVariant(true),
			},
			[]string{},
		},
	}

	got, ok := stateFromPropertiesSignal(signal)
	if !ok {
		t.Fatal("expected lid signal to be parsed")
	}
	if got != Closed {
		t.Fatalf("expected closed state, got %q", got)
	}
}

func TestStateFromPropertiesSignalIgnoresOtherProperties(t *testing.T) {
	signal := &dbus.Signal{
		Path: upowerPath,
		Name: propertiesSignal,
		Body: []any{
			upowerInterface,
			map[string]dbus.Variant{
				"OnBattery": dbus.MakeVariant(true),
			},
			[]string{},
		},
	}

	if _, ok := stateFromPropertiesSignal(signal); ok {
		t.Fatal("expected unrelated UPower property change to be ignored")
	}
}

func TestInputSwitchCapabilitySet(t *testing.T) {
	tests := []struct {
		mask string
		bit  int
		want bool
	}{
		{mask: "1\n", bit: 0, want: true},
		{mask: "0\n", bit: 0, want: false},
		{mask: "8 0\n", bit: 67, want: true},
		{mask: "8 0\n", bit: 0, want: false},
	}

	for _, tt := range tests {
		if got := inputSwitchCapabilitySet(tt.mask, tt.bit); got != tt.want {
			t.Fatalf("inputSwitchCapabilitySet(%q, %d) = %t, want %t", tt.mask, tt.bit, got, tt.want)
		}
	}
}

func TestInputLidEventDevicesDiscoversLidSwitch(t *testing.T) {
	tmp := t.TempDir()
	eventDir := filepath.Join(tmp, "event0", "device")
	if err := os.MkdirAll(filepath.Join(eventDir, "capabilities"), 0o755); err != nil {
		t.Fatalf("create input sysfs fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(eventDir, "name"), []byte("Apple SMC power/lid events\n"), 0o644); err != nil {
		t.Fatalf("write input device name: %v", err)
	}
	if err := os.WriteFile(filepath.Join(eventDir, "capabilities", "sw"), []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write input switch capabilities: %v", err)
	}

	events, err := inputLidEventDevices(tmp)
	if err != nil {
		t.Fatalf("inputLidEventDevices returned error: %v", err)
	}
	if len(events) != 1 || events[0] != "event0" {
		t.Fatalf("expected event0 lid device, got %v", events)
	}
}

func TestTabletModeSwitchIsFoundByItsOwnCapabilityBit(t *testing.T) {
	tmp := t.TempDir()
	for event, caps := range map[string]string{"event0": "1", "event1": "2", "event2": "3"} {
		dir := filepath.Join(tmp, event, "device", "capabilities")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sw"), []byte(caps+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tablet, err := inputSwitchEventDevices(tmp, switchTabletMode)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(tablet)
	if len(tablet) != 2 || tablet[0] != "event1" || tablet[1] != "event2" {
		t.Fatalf("tablet-mode switches = %v, want event1 and event2", tablet)
	}
	lids, _ := inputLidEventDevices(tmp)
	sort.Strings(lids)
	if len(lids) != 2 || lids[0] != "event0" || lids[1] != "event2" {
		t.Fatalf("lid switches = %v, want event0 and event2", lids)
	}
}
