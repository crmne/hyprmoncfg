package lid

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/godbus/dbus/v5"
)

const (
	upowerDestination = "org.freedesktop.UPower"
	upowerPath        = dbus.ObjectPath("/org/freedesktop/UPower")
	upowerInterface   = "org.freedesktop.UPower"
	propertiesSignal  = "org.freedesktop.DBus.Properties.PropertiesChanged"
)

const DefaultPollInterval = time.Second

const (
	inputSysRoot      = "/sys/class/input"
	inputDevRoot      = "/dev/input"
	inputIoctlBase    = uintptr('E')
	inputIoctlRead    = uintptr(2)
	inputIoctlGetSWNr = uintptr(0x1b)
)

var inputIoctlGetSW = inputIoctlRequest(inputIoctlRead, inputIoctlBase, inputIoctlGetSWNr, 1)

type State string

const (
	Unknown State = "unknown"
	Open    State = "open"
	Closed  State = "closed"
)

func (s State) Known() bool {
	return s == Open || s == Closed
}

func ReadState(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return Unknown, err
	}

	if state, err := readUPowerState(); err == nil {
		return state, nil
	}

	state, acpiErr := readACPIState()
	if acpiErr == nil {
		return state, nil
	}

	state, inputErr := readInputState()
	if inputErr != nil {
		return Unknown, fmt.Errorf("no lid state source found: %w", errors.Join(acpiErr, inputErr))
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

func readInputState() (State, error) {
	events, err := inputLidEventDevices(inputSysRoot)
	if err != nil {
		return Unknown, err
	}
	if len(events) == 0 {
		return Unknown, errors.New("input lid switch not found")
	}

	var lastErr error
	for _, event := range events {
		state, err := readInputEventLidState(filepath.Join(inputDevRoot, event))
		if err == nil {
			return state, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("input lid switch state not readable")
	}
	return Unknown, lastErr
}

func inputLidEventDevices(sysRoot string) ([]string, error) {
	return inputSwitchEventDevices(sysRoot, switchLid)
}

const (
	switchLid        = 0
	switchTabletMode = 1
)

func inputSwitchEventDevices(sysRoot string, bit int) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(sysRoot, "event*", "device"))
	if err != nil {
		return nil, err
	}

	events := make([]string, 0, len(paths))
	for _, path := range paths {
		caps, err := os.ReadFile(filepath.Join(path, "capabilities", "sw"))
		if err != nil || !inputSwitchCapabilitySet(string(caps), bit) {
			continue
		}
		events = append(events, filepath.Base(filepath.Dir(path)))
	}
	return events, nil
}

// ErrNoTabletModeSwitch means the device has no tablet-mode switch, as on a
// tablet or a laptop that doesn't fold.
var ErrNoTabletModeSwitch = errors.New("no tablet-mode switch")

// ReadTabletMode reports whether a convertible is folded into tablet mode.
// Reading the switch needs access to its input device, which members of the
// input group have.
func ReadTabletMode() (bool, error) {
	events, err := inputSwitchEventDevices(inputSysRoot, switchTabletMode)
	if err != nil {
		return false, err
	}
	if len(events) == 0 {
		return false, ErrNoTabletModeSwitch
	}
	var lastErr error
	for _, event := range events {
		switches, err := readInputSwitches(filepath.Join(inputDevRoot, event))
		if err == nil {
			return switches&(1<<switchTabletMode) != 0, nil
		}
		lastErr = err
	}
	return false, lastErr
}

func inputSwitchCapabilitySet(mask string, bit int) bool {
	if bit < 0 {
		return false
	}
	words := strings.Fields(mask)
	if len(words) == 0 {
		return false
	}

	wordIndexFromRight := bit / 64
	wordIndex := len(words) - 1 - wordIndexFromRight
	if wordIndex < 0 || wordIndex >= len(words) {
		return false
	}
	value, err := strconv.ParseUint(words[wordIndex], 16, 64)
	if err != nil {
		return false
	}
	return value&(uint64(1)<<(bit%64)) != 0
}

func readInputEventLidState(path string) (State, error) {
	switches, err := readInputSwitches(path)
	if err != nil {
		return Unknown, err
	}
	if switches&(1<<switchLid) != 0 {
		return Closed, nil
	}
	return Open, nil
}

func readInputSwitches(path string) (byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var switches [1]byte
	if err := inputIoctl(file.Fd(), inputIoctlGetSW, unsafe.Pointer(&switches[0])); err != nil {
		return 0, err
	}
	return switches[0], nil
}

func inputIoctlRequest(dir uintptr, typ uintptr, nr uintptr, size uintptr) uintptr {
	return (dir << 30) | (size << 16) | (typ << 8) | nr
}

func inputIoctl(fd uintptr, request uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}
