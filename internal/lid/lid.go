package lid

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	upowerDestination = "org.freedesktop.UPower"
	upowerPath        = dbus.ObjectPath("/org/freedesktop/UPower")
	upowerInterface   = "org.freedesktop.UPower"
	propertiesSignal  = "org.freedesktop.DBus.Properties.PropertiesChanged"
)

const DefaultPollInterval = time.Second

type State string

const (
	Unknown State = "unknown"
	Open    State = "open"
	Closed  State = "closed"
)

func (s State) Known() bool {
	return s == Open || s == Closed
}

func (s State) ClosedPtr() *bool {
	switch s {
	case Closed:
		closed := true
		return &closed
	case Open:
		closed := false
		return &closed
	default:
		return nil
	}
}

func ReadState(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return Unknown, err
	}

	if state, err := readUPowerState(); err == nil {
		return state, nil
	}

	state, err := readACPIState()
	if err != nil {
		return Unknown, fmt.Errorf("no lid state source found: %w", err)
	}
	return state, nil
}

func Watch(ctx context.Context, pollInterval time.Duration) (<-chan State, <-chan error) {
	states := make(chan State, 4)
	errs := make(chan error, 2)

	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}

	go func() {
		defer close(states)
		defer close(errs)

		initial, err := ReadState(ctx)
		if err != nil {
			sendErr(ctx, errs, err)
			return
		}

		last := Unknown
		emit := func(state State) bool {
			if !state.Known() || state == last {
				return true
			}
			last = state
			select {
			case <-ctx.Done():
				return false
			case states <- state:
				return true
			}
		}
		if !emit(initial) {
			return
		}

		upowerStates := watchUPowerSignals(ctx)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		var lastPollErr string
		for {
			select {
			case <-ctx.Done():
				return
			case state, ok := <-upowerStates:
				if !ok {
					upowerStates = nil
					continue
				}
				if !emit(state) {
					return
				}
			case <-ticker.C:
				state, err := ReadState(ctx)
				if err != nil {
					if msg := err.Error(); msg != lastPollErr {
						lastPollErr = msg
						sendErr(ctx, errs, err)
					}
					continue
				}
				lastPollErr = ""
				if !emit(state) {
					return
				}
			}
		}
	}()

	return states, errs
}

func sendErr(ctx context.Context, errs chan<- error, err error) {
	select {
	case <-ctx.Done():
	case errs <- err:
	default:
	}
}

func readUPowerState() (State, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return Unknown, err
	}
	defer conn.Close()

	return readUPowerStateFromConn(conn)
}

func readUPowerStateFromConn(conn *dbus.Conn) (State, error) {
	obj := conn.Object(upowerDestination, upowerPath)

	if present, err := boolProperty(obj, upowerInterface+".LidIsPresent"); err == nil && !present {
		return Unknown, errors.New("UPower reports no lid")
	}

	closed, err := boolProperty(obj, upowerInterface+".LidIsClosed")
	if err != nil {
		return Unknown, err
	}
	if closed {
		return Closed, nil
	}
	return Open, nil
}

type propertyGetter interface {
	GetProperty(name string) (dbus.Variant, error)
}

func boolProperty(obj propertyGetter, name string) (bool, error) {
	variant, err := obj.GetProperty(name)
	if err != nil {
		return false, err
	}
	value, ok := variant.Value().(bool)
	if !ok {
		return false, fmt.Errorf("%s is %T, not bool", name, variant.Value())
	}
	return value, nil
}

func watchUPowerSignals(ctx context.Context) <-chan State {
	states := make(chan State, 4)

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		close(states)
		return states
	}

	signalCh := make(chan *dbus.Signal, 8)
	conn.Signal(signalCh)

	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(upowerPath),
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchArg(0, upowerInterface),
	); err != nil {
		conn.RemoveSignal(signalCh)
		_ = conn.Close()
		close(states)
		return states
	}

	go func() {
		defer close(states)
		defer conn.Close()
		defer conn.RemoveSignal(signalCh)

		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signalCh:
				if !ok {
					return
				}
				state, ok := stateFromPropertiesSignal(signal)
				if !ok {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case states <- state:
				}
			}
		}
	}()

	return states
}

func stateFromPropertiesSignal(signal *dbus.Signal) (State, bool) {
	if signal == nil || signal.Name != propertiesSignal || signal.Path != upowerPath {
		return Unknown, false
	}
	if len(signal.Body) < 2 {
		return Unknown, false
	}

	iface, ok := signal.Body[0].(string)
	if !ok || iface != upowerInterface {
		return Unknown, false
	}

	changed, ok := signal.Body[1].(map[string]dbus.Variant)
	if !ok {
		return Unknown, false
	}

	variant, ok := changed["LidIsClosed"]
	if !ok {
		return Unknown, false
	}

	closed, ok := variant.Value().(bool)
	if !ok {
		return Unknown, false
	}
	if closed {
		return Closed, true
	}
	return Open, true
}

func readACPIState() (State, error) {
	paths, err := filepath.Glob("/proc/acpi/button/lid/*/state")
	if err != nil {
		return Unknown, err
	}
	if len(paths) == 0 {
		return Unknown, errors.New("/proc/acpi/button/lid/*/state not found")
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if state := parseACPIState(string(data)); state.Known() {
			return state, nil
		}
	}
	return Unknown, errors.New("ACPI lid state not readable")
}

func parseACPIState(value string) State {
	value = strings.ToLower(value)
	switch {
	case strings.Contains(value, "closed"):
		return Closed
	case strings.Contains(value, "open"):
		return Open
	default:
		return Unknown
	}
}
