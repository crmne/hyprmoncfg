package lid

import (
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
