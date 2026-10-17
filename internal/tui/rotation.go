package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
)

// rotationAutoValue is the Rotation choice that makes a built-in panel turn
// with the device. The panel's rotation then means upright.
const rotationAutoValue = "auto"

type rotationLockMsg struct {
	locked bool
	err    error
}

// rotationQuarters is how far the daemon turns panels that rotate
// automatically, so live state read while the device is turned still drafts
// the upright layout.
func (m Model) rotationQuarters() int {
	if m.rotation == nil {
		return 0
	}
	return m.rotation.Quarters
}

func (m Model) rotationAutoSupported(output editableOutput) bool {
	return m.rotation != nil && m.rotation.Available && output.IsInternal
}

// rotationLabel shows Auto for a panel that turns with the device, with its
// upright rotation when that is not normal, and whether it is locked.
func rotationLabel(output editableOutput, rotation *appstatus.Rotation) string {
	if !output.RotationAuto {
		return transformLabel(output.Transform)
	}
	label := "Auto"
	if output.Transform != 0 {
		label += ", upright " + fieldOptionLabel(6, transformLabel(output.Transform))
	}
	if rotation != nil && rotation.Locked {
		label += ", locked"
	}
	return label
}

func (m *Model) toggleRotationLockCmd() tea.Cmd {
	if m.ipc == nil || m.rotation == nil {
		m.setStatusErr("Automatic rotation needs hyprmoncfgd")
		return nil
	}
	if !m.rotation.Available {
		m.setStatusErr("No orientation sensor: install iio-sensor-proxy on a device with an accelerometer")
		return nil
	}
	client := m.ipc
	locked := !m.rotation.Locked
	return tea.Sequence(
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if client == nil {
				return rotationLockMsg{err: errors.New("hyprmoncfgd is not connected")}
			}
			return rotationLockMsg{locked: locked, err: client.SetRotationLock(ctx, locked)}
		},
		m.refreshCmd(true),
	)
}
