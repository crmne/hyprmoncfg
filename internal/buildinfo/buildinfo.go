package buildinfo

import (
	"fmt"
	"runtime/debug"
	"strings"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func init() {
	if Version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	// Use the module version only for clean releases (e.g. "v1.0.1"),
	// not pseudo-versions (e.g. "v1.0.2-0.20260331...-hash").
	if v := info.Main.Version; v != "" && v != "(devel)" {
		v = strings.TrimPrefix(v, "v")
		if !strings.Contains(v, "-") {
			Version = v
		}
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			Commit = s.Value[:7]
		}
		if s.Key == "vcs.time" {
			Date = s.Value
		}
	}
}

func Summary(name string) string {
	return fmt.Sprintf("%s %s (%s, %s)", name, Version, Commit, Date)
}
