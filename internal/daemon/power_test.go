package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPowerSupplyDetection(t *testing.T) {
	for _, tc := range []struct {
		name, status, online, scope string
		battery, known              bool
	}{
		{"battery", "Discharging", "0", "System", true, true},
		{"ac", "Charging", "1", "System", false, true},
		{"full on ac", "Full", "1", "System", false, true},
		{"unknown", "Unknown", "", "System", false, false},
		{"peripheral", "Discharging", "0", "Device", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for dir, fields := range map[string]map[string]string{"BAT0": {"type": "Battery", "scope": tc.scope, "status": tc.status}, "AC": {"type": "Mains", "online": tc.online}} {
				if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
					t.Fatal(err)
				}
				for field, value := range fields {
					if err := os.WriteFile(filepath.Join(root, dir, field), []byte(value), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			battery, known := readBatteryState(root)
			if battery != tc.battery || known != tc.known {
				t.Fatalf("got %v/%v", battery, known)
			}
		})
	}
	if _, known := readBatteryState(t.TempDir()); known {
		t.Fatal("desktop must have unknown battery state")
	}
}
