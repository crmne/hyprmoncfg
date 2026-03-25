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
