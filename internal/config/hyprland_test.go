package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVerifySourceChainFindsNestedRelativeSource(t *testing.T) {
	root := t.TempDir()
	hypr := filepath.Join(root, "hypr")
	if err := os.MkdirAll(filepath.Join(hypr, "conf.d"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	rootConfig := filepath.Join(hypr, "hyprland.conf")
	include := filepath.Join(hypr, "conf.d", "displays.conf")
	target := filepath.Join(hypr, "monitors.conf")

	if err := os.WriteFile(rootConfig, []byte("source = ./conf.d/displays.conf\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(include, []byte("source = ../monitors.conf\n"), 0o644); err != nil {
		t.Fatalf("write include: %v", err)
	}

	if err := VerifySourceChain(rootConfig, target); err != nil {
		t.Fatalf("expected nested source chain to verify, got %v", err)
	}
}

func TestVerifySourceChainSupportsGlobInclude(t *testing.T) {
	root := t.TempDir()
	hypr := filepath.Join(root, "hypr")
	if err := os.MkdirAll(filepath.Join(hypr, "conf.d"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	rootConfig := filepath.Join(hypr, "hyprland.conf")
	target := filepath.Join(hypr, "conf.d", "monitors.conf")

	if err := os.WriteFile(rootConfig, []byte("source = ./conf.d/*.conf\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("# generated elsewhere\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if err := VerifySourceChain(rootConfig, target); err != nil {
		t.Fatalf("expected glob include to verify target path, got %v", err)
	}
}

func TestVerifySourceChainRejectsUnsourcedTarget(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.conf")
	target := filepath.Join(root, "monitors.conf")

	if err := os.WriteFile(rootConfig, []byte("source = ./input.conf\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	err := VerifySourceChain(rootConfig, target)
	if err == nil {
		t.Fatal("expected verify to fail for unsourced target")
	}
	if !strings.Contains(err.Error(), "not sourced") {
		t.Fatalf("expected unsourced error, got %v", err)
	}
}

func TestResolveHyprlandConfigUsesLegacyBeforeLuaRelease(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	resolved, err := ResolveHyprlandConfig("0.54.0", "", "")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if resolved.Format != HyprConfigLegacy {
		t.Fatalf("expected legacy format, got %v", resolved.Format)
	}
	if filepath.Base(resolved.RootPath) != "hyprland.conf" {
		t.Fatalf("expected hyprland.conf root, got %s", resolved.RootPath)
	}
	if filepath.Base(resolved.MonitorsPath) != "monitors.conf" {
		t.Fatalf("expected monitors.conf target, got %s", resolved.MonitorsPath)
	}
}

func TestResolveHyprlandConfigUsesLegacyFor055WithoutLuaConfig(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "hypr"), 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}

	resolved, err := ResolveHyprlandConfig("0.55.0", "", "")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if resolved.Format != HyprConfigLegacy {
		t.Fatalf("expected legacy format, got %v", resolved.Format)
	}
}

func TestResolveHyprlandConfigUsesLuaFor055WithLuaConfig(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	hypr := filepath.Join(xdg, "hypr")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hypr, "hyprland.lua"), []byte("-- lua config\n"), 0o644); err != nil {
		t.Fatalf("write hyprland.lua: %v", err)
	}

	resolved, err := ResolveHyprlandConfig("0.55.0", "", "")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if resolved.Format != HyprConfigLua {
		t.Fatalf("expected lua format, got %v", resolved.Format)
	}
	if filepath.Base(resolved.RootPath) != "hyprland.lua" {
		t.Fatalf("expected hyprland.lua root, got %s", resolved.RootPath)
	}
	if filepath.Base(resolved.MonitorsPath) != "monitors.lua" {
		t.Fatalf("expected monitors.lua target, got %s", resolved.MonitorsPath)
	}
}

func TestResolveHyprlandConfigExplicitExtensionForcesFormat(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	luaRoot := filepath.Join(t.TempDir(), "hyprland.lua")
	resolved, err := ResolveHyprlandConfig("0.54.0", "", luaRoot)
	if err != nil {
		t.Fatalf("resolve lua config: %v", err)
	}
	if resolved.Format != HyprConfigLua {
		t.Fatalf("expected explicit .lua to force lua format, got %v", resolved.Format)
	}

	legacyRoot := filepath.Join(t.TempDir(), "hyprland.conf")
	resolved, err = ResolveHyprlandConfig("0.55.0", "", legacyRoot)
	if err != nil {
		t.Fatalf("resolve legacy config: %v", err)
	}
	if resolved.Format != HyprConfigLegacy {
		t.Fatalf("expected explicit .conf to force legacy format, got %v", resolved.Format)
	}
}

func TestVerifyLuaSourceChainFindsLiteralRequire(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "monitors.lua")

	if err := os.WriteFile(rootConfig, []byte("require('monitors')\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected lua include chain to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsStringCallRequire(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "monitors.lua")

	if err := os.WriteFile(rootConfig, []byte("require \"monitors\"\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected string-call lua require to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsStandardLiteralRequireForms(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "semicolon", content: `; require("monitors")`},
		{name: "assignment", content: `local monitors = require("monitors")`},
		{name: "long bracket", content: `require [[monitors]]`},
		{name: "leveled long bracket", content: `require [=[monitors]=]`},
		{name: "comment after call", content: `require("monitors") -- include generated monitors`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			rootConfig := filepath.Join(root, "hyprland.lua")
			target := filepath.Join(root, "monitors.lua")

			if err := os.WriteFile(rootConfig, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write root config: %v", err)
			}
			if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
				t.Fatalf("write target config: %v", err)
			}

			if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
				t.Fatalf("expected standard lua require form to verify, got %v", err)
			}
		})
	}
}

func TestVerifyLuaSourceChainHandlesCommentMarkerInsideString(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "mon--itors.lua")

	if err := os.WriteFile(rootConfig, []byte(`require("mon--itors")`), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected -- inside string literal to stay part of require, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsSlashRequire(t *testing.T) {
	root := t.TempDir()
	confDir := filepath.Join(root, "conf.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("mkdir conf dir: %v", err)
	}
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(confDir, "monitors.lua")

	if err := os.WriteFile(rootConfig, []byte("require 'conf.d/monitors'\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected slash-style lua require to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsDotRequire(t *testing.T) {
	root := t.TempDir()
	confDir := filepath.Join(root, "awesomeconf")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("mkdir conf dir: %v", err)
	}
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(confDir, "animation.lua")

	if err := os.WriteFile(rootConfig, []byte("require('awesomeconf.animation')\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected dot-style lua require to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsStringCallDofile(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "monitors.lua")

	if err := os.WriteFile(rootConfig, []byte("dofile 'monitors.lua'\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected string-call lua dofile to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsPackagePathStyleRequire(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, ".config")
	hypr := filepath.Join(configHome, "hypr")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	rootConfig := filepath.Join(hypr, "hyprland.lua")
	target := filepath.Join(hypr, "monitors.lua")

	content := `package.path = "` + filepath.ToSlash(filepath.Join(configHome, "?.lua")) + `;" .. package.path
require('hypr.monitors')
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected package-path style lua include to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsHomePackagePathRequire(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, ".config")
	hypr := filepath.Join(configHome, "hypr")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	rootConfig := filepath.Join(hypr, "hyprland.lua")
	target := filepath.Join(hypr, "monitors.lua")

	t.Setenv("HOME", root)
	content := `package.path = os.getenv("HOME") .. "/.config/?.lua;" .. package.path
require('hypr.monitors')
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected HOME package-path lua include to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsXDGConfigHomePackagePathRequire(t *testing.T) {
	root := t.TempDir()
	xdgConfigHome := filepath.Join(root, "xdg-config")
	hypr := filepath.Join(xdgConfigHome, "hypr")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	rootConfig := filepath.Join(hypr, "hyprland.lua")
	target := filepath.Join(hypr, "monitors.lua")

	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	content := `package.path = os.getenv("XDG_CONFIG_HOME") .. "/?.lua;" .. package.path
require('hypr.monitors')
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected XDG_CONFIG_HOME package-path lua include to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsMultilineEnvPackagePathRequire(t *testing.T) {
	root := t.TempDir()
	xdgConfigHome := filepath.Join(root, "xdg-config")
	hypr := filepath.Join(xdgConfigHome, "hypr")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	rootConfig := filepath.Join(hypr, "hyprland.lua")
	target := filepath.Join(hypr, "monitors.lua")

	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	content := `package.path = os.getenv("XDG_CONFIG_HOME") ..
  .. "/?.lua;"
  .. package.path
require('hypr.monitors')
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected multiline env package-path lua include to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsOmarchyPackagePathRequire(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	hypr := filepath.Join(home, ".config", "hypr")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	rootConfig := filepath.Join(hypr, "hyprland.lua")
	target := filepath.Join(hypr, "monitors.lua")

	t.Setenv("HOME", home)
	content := `package.path = os.getenv("HOME")
  .. "/.config/?.lua;"
  .. package.path
require("hypr.monitors")
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected Omarchy package.path style lua include to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsPackagePathAssignmentForms(t *testing.T) {
	tests := []struct {
		name    string
		content func(configHome string) string
	}{
		{
			name: "semicolon prefix",
			content: func(configHome string) string {
				return `; package.path = "` + filepath.ToSlash(filepath.Join(configHome, "?.lua")) + `;" .. package.path
require('hypr.monitors')
`
			},
		},
		{
			name: "no spaces",
			content: func(configHome string) string {
				return `package.path="` + filepath.ToSlash(filepath.Join(configHome, "?.lua")) + `;"..package.path
require('hypr.monitors')
`
			},
		},
		{
			name: "single quoted getenv",
			content: func(configHome string) string {
				return `package.path = os.getenv('XDG_CONFIG_HOME') .. "/?.lua;" .. package.path
require('hypr.monitors')
`
			},
		},
		{
			name: "no existing package append",
			content: func(configHome string) string {
				return `package.path = "` + filepath.ToSlash(filepath.Join(configHome, "?.lua")) + `;"
require('hypr.monitors')
`
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			configHome := filepath.Join(root, "config")
			hypr := filepath.Join(configHome, "hypr")
			if err := os.MkdirAll(hypr, 0o755); err != nil {
				t.Fatalf("mkdir hypr dir: %v", err)
			}
			rootConfig := filepath.Join(hypr, "hyprland.lua")
			target := filepath.Join(hypr, "monitors.lua")

			t.Setenv("XDG_CONFIG_HOME", configHome)
			if err := os.WriteFile(rootConfig, []byte(tt.content(configHome)), 0o644); err != nil {
				t.Fatalf("write root config: %v", err)
			}
			if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
				t.Fatalf("write target config: %v", err)
			}

			if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
				t.Fatalf("expected package.path assignment form to verify, got %v", err)
			}
		})
	}
}

func TestVerifyLuaSourceChainDoesNotUseImplicitParentFallback(t *testing.T) {
	root := t.TempDir()
	hypr := filepath.Join(root, "hypr")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	rootConfig := filepath.Join(hypr, "hyprland.lua")
	target := filepath.Join(hypr, "monitors.lua")

	if err := os.WriteFile(rootConfig, []byte("require('hypr.monitors')\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
		t.Fatal("expected package-path style require without package.path to fail")
	}
}

func TestVerifyLuaSourceChainIgnoresUnassignedPackagePathString(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(os.TempDir(), "monitors.lua")

	content := `package.path = package.path
local example = "/tmp/?.lua"
require("monitors")
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
		t.Fatal("expected unrelated package-path-shaped string to be ignored")
	}
}

func TestVerifyLuaSourceChainIgnoresPackagePathEnvOutsideAssignment(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(os.TempDir(), "monitors.lua")

	t.Setenv("HOME", root)
	content := `local home = os.getenv("HOME")
local example = "/tmp/?.lua"
require("monitors")
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
		t.Fatal("expected package-path-shaped string after os.getenv to be ignored")
	}
}

func TestVerifyLuaSourceChainIgnoresNonPackagePathAssignments(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "local concat", content: `local maybe = package.path .. "/tmp/?.lua"
require("monitors")
`},
		{name: "local compare", content: `local same = package.path == "/tmp/?.lua"
require("monitors")
`},
		{name: "if compare", content: `if package.path == "/tmp/?.lua" then end
require("monitors")
`},
		{name: "field assignment", content: `some.package.path = "/tmp/?.lua"
require("monitors")
`},
		{name: "longer field name", content: `package.pathname = "/tmp/?.lua"
require("monitors")
`},
		{name: "prefixed field name", content: `package.path_extra = "/tmp/?.lua"
require("monitors")
`},
		{name: "local similar name", content: `local package_path = "/tmp/?.lua"
require("monitors")
`},
		{name: "not equals", content: `package.path ~= "/tmp/?.lua"
require("monitors")
`},
		{name: "less equal", content: `package.path <= "/tmp/?.lua"
require("monitors")
`},
		{name: "quoted assignment", content: `local note = "package.path = '/tmp/?.lua;'"
require("monitors")
`},
		{name: "short comment assignment", content: `-- package.path = "/tmp/?.lua;"
require("monitors")
`},
		{name: "long comment assignment", content: `--[[
package.path = "/tmp/?.lua;"
]]
require("monitors")
`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			rootConfig := filepath.Join(root, "hyprland.lua")
			target := filepath.Join(os.TempDir(), "monitors.lua")

			if err := os.WriteFile(rootConfig, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write root config: %v", err)
			}

			if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
				t.Fatal("expected non-package.path assignment to be ignored")
			}
		})
	}
}

func TestVerifyLuaSourceChainIgnoresInvalidPackagePathContinuations(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "blank line", content: `package.path = os.getenv("HOME") ..

"/tmp/?.lua"
require("monitors")
`},
		{name: "no continuation", content: `package.path = os.getenv("HOME")
"/tmp/?.lua"
require("monitors")
`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			rootConfig := filepath.Join(root, "hyprland.lua")
			target := filepath.Join(os.TempDir(), "monitors.lua")

			t.Setenv("HOME", root)
			if err := os.WriteFile(rootConfig, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write root config: %v", err)
			}

			if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
				t.Fatal("expected invalid package.path continuation to be ignored")
			}
		})
	}
}

func TestVerifyLuaSourceChainHandlesPackagePathCommentMarkerInsideString(t *testing.T) {
	root := t.TempDir()
	hypr := filepath.Join(root, "tmp--config")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(hypr, "monitors.lua")

	content := `package.path = "` + filepath.ToSlash(filepath.Join(hypr, "?.lua")) + `;" -- generated path
require("monitors")
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected -- inside package.path string to stay part of pattern, got %v", err)
	}
}

func TestVerifyLuaSourceChainIgnoresUnsetEnvPackagePath(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(os.TempDir(), "monitors.lua")

	t.Setenv("MISSING_LUA_PATH", "")
	content := `package.path = os.getenv("MISSING_LUA_PATH") .. "/?.lua;"
require("monitors")
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
		t.Fatal("expected unset env package.path to be ignored")
	}
}

func TestVerifyLuaSourceChainIgnoresRequireInsideString(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "monitors.lua")

	if err := os.WriteFile(rootConfig, []byte(`local note = "require('monitors')"`), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
		t.Fatal("expected require inside a string literal to be ignored")
	}
}

func TestVerifyLuaSourceChainIgnoresRequireInsideLongComment(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "monitors.lua")

	content := `--[[
require("monitors")
]]
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
		t.Fatal("expected require inside a long comment to be ignored")
	}
}

func TestVerifyLuaSourceChainIgnoresRequireFieldCall(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "monitors.lua")

	content := `local loader = {}
loader.require("monitors")
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
		t.Fatal("expected require field call to be ignored")
	}
}

func TestVerifyLuaSourceChainIgnoresUnclosedParenthesizedRequire(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "monitors.lua")

	if err := os.WriteFile(rootConfig, []byte(`require("monitors"`), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err == nil {
		t.Fatal("expected unclosed parenthesized require to be ignored")
	}
}

func TestVerifyLuaSourceChainRejectsUnsourcedTarget(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "monitors.lua")

	if err := os.WriteFile(rootConfig, []byte("-- no include\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	err := VerifyIncludeChain(HyprConfigLua, rootConfig, target)
	if err == nil {
		t.Fatal("expected verify to fail for unsourced lua target")
	}
	if !strings.Contains(err.Error(), "not included") {
		t.Fatalf("expected unsourced error, got %v", err)
	}
	if !strings.Contains(err.Error(), `require("monitors")`) {
		t.Fatalf("expected co-located target suggestion, got %v", err)
	}
}

func TestVerifyLuaSourceChainSuggestsAbsoluteDofileForCustomTarget(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(t.TempDir(), "generated", "monitors.lua")

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(rootConfig, []byte("-- no include\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	err := VerifyIncludeChain(HyprConfigLua, rootConfig, target)
	if err == nil {
		t.Fatal("expected verify to fail for unsourced lua target")
	}
	want := "dofile(" + strconv.Quote(target) + ")"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected absolute dofile suggestion %s, got %v", want, err)
	}
}
