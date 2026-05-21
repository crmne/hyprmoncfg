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
		t.Fatalf("expected package-path style lua include to verify, got %v", err)
	}
}

func TestVerifyLuaSourceChainFindsOmarchyPackagePathRequires(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, ".config")
	omarchyPath := filepath.Join(root, ".local", "share", "omarchy")
	hypr := filepath.Join(configHome, "hypr")
	omarchyDefault := filepath.Join(omarchyPath, "default", "hypr")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	if err := os.MkdirAll(omarchyDefault, 0o755); err != nil {
		t.Fatalf("mkdir omarchy default dir: %v", err)
	}
	rootConfig := filepath.Join(hypr, "hyprland.lua")
	target := filepath.Join(hypr, "monitors.lua")
	omarchyModule := filepath.Join(omarchyDefault, "omarchy.lua")

	t.Setenv("HOME", root)
	t.Setenv("OMARCHY_PATH", omarchyPath)
	content := `package.path = os.getenv("HOME")
  .. "/.config/?.lua;"
  .. (os.getenv("OMARCHY_PATH") or (os.getenv("HOME") .. "/.local/share/omarchy"))
  .. "/?.lua;"
  .. package.path

require("default.hypr.omarchy")
require("hypr.monitors")
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}
	if err := os.WriteFile(omarchyModule, []byte("-- defaults\n"), 0o644); err != nil {
		t.Fatalf("write omarchy module: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected Omarchy package-path lua include to verify, got %v", err)
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

func TestVerifyLuaSourceChainFindsXDGConfigHomeHyprPackagePathRequire(t *testing.T) {
	root := t.TempDir()
	xdgConfigHome := filepath.Join(root, "xdg-config")
	hypr := filepath.Join(xdgConfigHome, "hypr")
	if err := os.MkdirAll(hypr, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	rootConfig := filepath.Join(hypr, "hyprland.lua")
	target := filepath.Join(hypr, "monitors.lua")

	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	content := `package.path = os.getenv("XDG_CONFIG_HOME") .. "/hypr/?.lua;" .. package.path
require('monitors')
`
	if err := os.WriteFile(rootConfig, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
	if err := os.WriteFile(target, []byte("-- generated\n"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected XDG_CONFIG_HOME hypr package-path lua include to verify, got %v", err)
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
