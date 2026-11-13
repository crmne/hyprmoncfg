package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAllowsAtMostOneMainOutput(t *testing.T) {
	outputs := func(a, b bool) []OutputConfig {
		return []OutputConfig{
			{Key: "a", Name: "DP-1", Enabled: true, Scale: 1, Main: a},
			{Key: "b", Name: "DP-2", Enabled: true, Scale: 1, Main: b},
		}
	}
	for _, tc := range []struct {
		a, b    bool
		wantErr bool
	}{
		{false, false, false},
		{true, false, false},
		{false, true, false},
		{true, true, true},
	} {
		err := New("test", outputs(tc.a, tc.b)).Validate()
		if (err != nil) != tc.wantErr {
			t.Fatalf("main=%v,%v: err=%v, wantErr=%v", tc.a, tc.b, err, tc.wantErr)
		}
	}

	main, ok := New("test", outputs(false, true)).MainOutput()
	if !ok || main.Key != "b" {
		t.Fatalf("MainOutput() = %+v, %v; want b", main, ok)
	}
	if _, ok := New("test", outputs(false, false)).MainOutput(); ok {
		t.Fatal("MainOutput() found a main display in a profile without one")
	}
}

func TestStoreRejectsProfileWithTwoMainOutputs(t *testing.T) {
	store := NewStore(t.TempDir())
	p := New("two-mains", []OutputConfig{
		{Key: "a", Name: "DP-1", Enabled: true, Scale: 1, Main: true},
		{Key: "b", Name: "DP-2", Enabled: true, Scale: 1, Main: true},
	})
	if err := store.Save(p); err == nil {
		t.Fatal("Save accepted a profile with two main displays")
	}

	// a hand-edited file with two mains must not load either
	path := store.PathsForName(p.Name).JSON
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"name":"two-mains","outputs":[` +
		`{"key":"a","name":"DP-1","enabled":true,"scale":1,"main":true},` +
		`{"key":"b","name":"DP-2","enabled":true,"scale":1,"main":true}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(p.Name); err == nil {
		t.Fatal("Load accepted a profile with two main displays")
	}

	// the same file with one main loads fine, so the failure above is the main check
	p.Outputs[1].Main = false
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(p.Name)
	if err != nil {
		t.Fatal(err)
	}
	if main, ok := loaded.MainOutput(); !ok || main.Key != "a" {
		t.Fatalf("main display lost on round trip: %+v", loaded.Outputs)
	}
}
