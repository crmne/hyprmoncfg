package wallpaper

import (
	"reflect"
	"testing"
)

func TestDocumentSetClearRenameDrop(t *testing.T) {
	doc := NewDocument()
	doc = doc.Set("Desk", "DEL|U2720Q|ABC", "/w/a.png")
	doc = doc.Set("Desk", "LG|27GL|1", "/w/b.png")
	doc = doc.Set("", "k", "/w/c.png")
	doc = doc.Set("Desk", "", "/w/c.png")
	doc = doc.Set("Desk", "k", "")
	if got := doc.For("Desk", "DEL|U2720Q|ABC"); got != "/w/a.png" {
		t.Fatalf("For = %q", got)
	}
	if len(doc.Profiles) != 1 || len(doc.Profiles["Desk"]) != 2 {
		t.Fatalf("empty inputs must be ignored: %#v", doc.Profiles)
	}

	doc = doc.Clear("Desk", "LG|27GL|1")
	doc = doc.Clear("Desk", "DEL|U2720Q|ABC")
	if _, ok := doc.Profiles["Desk"]; ok {
		t.Fatalf("a profile with no wallpapers must be removed: %#v", doc.Profiles)
	}

	doc = doc.Set("Desk", "k", "/w/a.png").Set("Home", "k", "/w/h.png")
	if stale := doc.Rename("Desk", "Home"); stale.For("Home", "k") != "/w/a.png" || stale.Has("Desk") {
		t.Fatalf("a stale entry under the new name must be replaced, as the plugin does: %#v", stale.Profiles)
	}
	if same := doc.Rename("Desk", "Desk"); !reflect.DeepEqual(same, doc) {
		t.Fatalf("rename to the same name must be a no-op")
	}
	moved := doc.Rename("Desk", "desk")
	if moved.For("desk", "k") != "/w/a.png" || moved.For("Desk", "k") != "" {
		t.Fatalf("case-only rename must move the entry: %#v", moved.Profiles)
	}
	moved = doc.Rename("Desk", "Office")
	if moved.For("Office", "k") != "/w/a.png" || moved.For("Desk", "k") != "" {
		t.Fatalf("rename must move the entry: %#v", moved.Profiles)
	}
	if doc.For("Desk", "k") != "/w/a.png" {
		t.Fatalf("mutations must not change the receiver")
	}

	dropped := doc.Drop("Desk")
	if dropped.For("Desk", "k") != "" || dropped.For("Home", "k") != "/w/h.png" {
		t.Fatalf("drop: %#v", dropped.Profiles)
	}
	if !doc.Has("Desk") || dropped.Has("Desk") {
		t.Fatalf("Has must report whether a profile has wallpapers")
	}
}
