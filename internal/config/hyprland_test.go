package config

import (
	"os"
	"path/filepath"
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

func TestVerifyLuaSourceChainFindsLiteralDofile(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "hyprland.lua")
	target := filepath.Join(root, "monitors.lua")

	if err := os.WriteFile(rootConfig, []byte("dofile('./monitors.lua')\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	if err := VerifyIncludeChain(HyprConfigLua, rootConfig, target); err != nil {
		t.Fatalf("expected lua include chain to verify, got %v", err)
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
}
