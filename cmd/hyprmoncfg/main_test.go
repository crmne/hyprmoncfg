package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/config"
)

func TestConfirmApplySignalRejectsConfiguration(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	var output bytes.Buffer

	keep, err := confirmApplyWithInput(10, reader, &output, signals)
	if err != nil {
		t.Fatalf("confirm after signal: %v", err)
	}
	if keep {
		t.Fatal("expected signal to reject unconfirmed configuration")
	}
}

func TestDoctorReportsMissingGeneratedMonitorConfig(t *testing.T) {
	dir := t.TempDir()
	rootPath := filepath.Join(dir, "hyprland.lua")
	monitorsPath := filepath.Join(dir, "hyprmoncfg-monitors.lua")
	if err := os.WriteFile(rootPath, []byte(config.IncludeLine(config.HyprConfigLua, monitorsPath)+"\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	cmd := newDoctorCmd(&monitorsPath, &rootPath)
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor: %v", err)
	}

	got := output.String()
	if !strings.Contains(got, "PROBLEM") || !strings.Contains(got, "does not exist") {
		t.Fatalf("doctor output did not report the missing generated file:\n%s", got)
	}
}

func TestRotationSummarySaysHowPanelsTurn(t *testing.T) {
	for _, tc := range []struct {
		rotation appstatus.Rotation
		want     string
	}{
		{appstatus.Rotation{}, "Automatic rotation: no orientation sensor"},
		{appstatus.Rotation{Available: true, Quarters: 1}, "Automatic rotation: following the sensor, turned 90°"},
		{appstatus.Rotation{Available: true, Locked: true, Quarters: 2}, "Automatic rotation: locked, upside down"},
		{appstatus.Rotation{Available: true, Paused: true}, "Automatic rotation: paused while folded as a laptop, upright"},
		{appstatus.Rotation{Available: true, Quarters: 3, Reversed: true}, "Automatic rotation: following the sensor, turned 270°, direction reversed"},
	} {
		if got := rotationSummary(tc.rotation); got != tc.want {
			t.Errorf("rotationSummary(%+v) = %q, want %q", tc.rotation, got, tc.want)
		}
	}
}

func TestRotationRejectsUnknownActions(t *testing.T) {
	for args, want := range map[string]string{
		"spin":             "unknown rotation action",
		"direction":        "direction is normal or reversed",
		"direction upside": "direction is normal or reversed",
		"lock now":         "lock takes no value",
	} {
		cmd := newRotationCmd()
		cmd.SetArgs(strings.Fields(args))
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: err = %v, want %q", args, err, want)
		}
	}
}
