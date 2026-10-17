package console

import "testing"

func TestFreeConsoleSkipsConsolesInUse(t *testing.T) {
	if vt, ok := freeConsole(map[uint32]bool{1: true}); !ok || vt != 2 {
		t.Fatalf("freeConsole = %d, %v; want 2", vt, ok)
	}
	if vt, ok := freeConsole(map[uint32]bool{1: true, 2: true, 4: true}); !ok || vt != 3 {
		t.Fatalf("freeConsole = %d, %v; want 3", vt, ok)
	}
	used := map[uint32]bool{}
	for vt := uint32(1); vt <= maxConsole; vt++ {
		used[vt] = true
	}
	if _, ok := freeConsole(used); ok {
		t.Fatal("found a free console with every console in use")
	}
}
