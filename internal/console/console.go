// Package console switches the seat's virtual terminal through
// systemd-logind, which lets the user of the active local session do it
// without root.
package console

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	loginService     = "org.freedesktop.login1"
	seatPath         = dbus.ObjectPath("/org/freedesktop/login1/seat/seat0")
	seatInterface    = "org.freedesktop.login1.Seat"
	sessionInterface = "org.freedesktop.login1.Session"
	// Linux has 63 consoles; logind starts a login prompt on the first 6 by
	// default, and any of them will do.
	maxConsole = 63
)

// SwitchAwayAndBack switches seat0 to a console no session uses, waits, and
// switches back to the active session's console. A compositor that is
// switched away releases the display hardware and resets it on its return.
// The active session must be the caller's own graphical session on a
// console; anything else is left alone.
func SwitchAwayAndBack(ctx context.Context, away time.Duration) error {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return fmt.Errorf("connect to the system bus: %w", err)
	}
	defer conn.Close()

	seat := conn.Object(loginService, seatPath)
	if canTTY, err := property[bool](seat, seatInterface+".CanTTY"); err != nil {
		return err
	} else if !canTTY {
		return errors.New("seat0 has no consoles")
	}
	active, err := property[[]any](seat, seatInterface+".ActiveSession")
	if err != nil {
		return err
	}
	if len(active) != 2 {
		return errors.New("no active session on seat0")
	}
	activePath, _ := active[1].(dbus.ObjectPath)
	home, err := ownGraphicalConsole(conn.Object(loginService, activePath))
	if err != nil {
		return err
	}

	sessions, err := property[[][]any](seat, seatInterface+".Sessions")
	if err != nil {
		return err
	}
	used := map[uint32]bool{home: true}
	for _, entry := range sessions {
		if len(entry) != 2 {
			continue
		}
		path, _ := entry[1].(dbus.ObjectPath)
		if vt, err := property[uint32](conn.Object(loginService, path), sessionInterface+".VTNr"); err == nil {
			used[vt] = true
		}
	}
	target, ok := freeConsole(used)
	if !ok {
		return errors.New("no free console to switch to")
	}

	if err := seat.CallWithContext(ctx, seatInterface+".SwitchTo", 0, target).Err; err != nil {
		return fmt.Errorf("switch to console %d: %w", target, err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(away):
	}
	// Return even when ctx ended: staying on the other console would leave
	// the user looking at a login prompt.
	back, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := seat.CallWithContext(back, seatInterface+".SwitchTo", 0, home).Err; err != nil {
		return fmt.Errorf("switch back to console %d: %w", home, err)
	}
	return nil
}

func ownGraphicalConsole(session dbus.BusObject) (uint32, error) {
	user, err := property[[]any](session, sessionInterface+".User")
	if err != nil {
		return 0, err
	}
	if len(user) != 2 {
		return 0, errors.New("active session has no user")
	}
	if uid, _ := user[0].(uint32); int(uid) != os.Getuid() {
		return 0, errors.New("the active session belongs to another user")
	}
	kind, err := property[string](session, sessionInterface+".Type")
	if err != nil {
		return 0, err
	}
	if kind != "wayland" && kind != "x11" {
		return 0, fmt.Errorf("the active session is not graphical (%s)", kind)
	}
	vt, err := property[uint32](session, sessionInterface+".VTNr")
	if err != nil {
		return 0, err
	}
	if vt == 0 {
		return 0, errors.New("the active session is not on a console")
	}
	return vt, nil
}

// freeConsole picks the lowest console no session uses.
func freeConsole(used map[uint32]bool) (uint32, bool) {
	for vt := uint32(1); vt <= maxConsole; vt++ {
		if !used[vt] {
			return vt, true
		}
	}
	return 0, false
}

func property[T any](object dbus.BusObject, name string) (T, error) {
	var zero T
	variant, err := object.GetProperty(name)
	if err != nil {
		return zero, fmt.Errorf("read %s: %w", name, err)
	}
	value, ok := variant.Value().(T)
	if !ok {
		return zero, fmt.Errorf("read %s: unexpected %s", name, variant.Signature())
	}
	return value, nil
}
