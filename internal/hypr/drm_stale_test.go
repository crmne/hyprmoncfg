package hypr

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDRMConnectorStatesReadStatusAndEnabled(t *testing.T) {
	root := t.TempDir()
	for name, state := range map[string][2]string{
		"card1-DP-1":     {"disconnected", "enabled"},
		"card1-DP-2":     {"connected", "enabled"},
		"card1-HDMI-A-1": {"disconnected", "disabled"},
		"card1":          {"disconnected", "enabled"},
	} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte(state[0]+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "enabled"), []byte(state[1]+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := []DRMConnectorState{
		{Name: "DP-1", Enabled: true},
		{Name: "DP-2", Connected: true, Enabled: true},
		{Name: "HDMI-A-1"},
	}
	if got := drmConnectorStates(root); !reflect.DeepEqual(got, want) {
		t.Fatalf("states = %+v, want %+v", got, want)
	}
}
