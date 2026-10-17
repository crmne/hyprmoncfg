// Package orientation reports the device orientation from iio-sensor-proxy,
// the accelerometer service 2-in-1 laptops and tablets use.
package orientation

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	proxyService   = "net.hadess.SensorProxy"
	proxyPath      = dbus.ObjectPath("/net/hadess/SensorProxy")
	proxyInterface = "net.hadess.SensorProxy"
	propsInterface = "org.freedesktop.DBus.Properties"
)

// Reading is one report from the sensor. Available is false when there is
// no accelerometer, or iio-sensor-proxy is not installed or running.
type Reading struct {
	Available   bool
	Orientation string
}

// Quarters turns an iio-sensor-proxy orientation into counterclockwise
// quarter turns, the way Hyprland transforms count them. "undefined", which
// the sensor reports while the device lies flat, has none.
func Quarters(orientation string) (int, bool) {
	switch orientation {
	case "normal":
		return 0, true
	case "left-up":
		return 1, true
	case "bottom-up":
		return 2, true
	case "right-up":
		return 3, true
	}
	return 0, false
}

// Watch claims the accelerometer and reports the orientation whenever it
// changes, starting with the current one. It reports an unavailable reading
// when the sensor goes away and claims it again when the service returns.
// The channel closes when ctx ends or the system bus cannot be reached.
func Watch(ctx context.Context) <-chan Reading {
	readings := make(chan Reading, 4)
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		close(readings)
		return readings
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	matches := [][]dbus.MatchOption{
		{dbus.WithMatchObjectPath(proxyPath), dbus.WithMatchInterface(propsInterface), dbus.WithMatchMember("PropertiesChanged")},
		{dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, proxyService)},
	}
	for _, match := range matches {
		if err := conn.AddMatchSignal(match...); err != nil {
			conn.RemoveSignal(signals)
			_ = conn.Close()
			close(readings)
			return readings
		}
	}

	proxy := conn.Object(proxyService, proxyPath)
	go func() {
		defer close(readings)
		defer conn.Close()
		defer conn.RemoveSignal(signals)
		claimed := false
		defer func() {
			if claimed {
				release, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = proxy.CallWithContext(release, proxyInterface+".ReleaseAccelerometer", 0).Err
			}
		}()

		send := func(reading Reading) bool {
			select {
			case <-ctx.Done():
				return false
			case readings <- reading:
				return true
			}
		}
		claim := func() Reading {
			call, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := proxy.CallWithContext(call, proxyInterface+".ClaimAccelerometer", 0).Err; err != nil {
				claimed = false
				return Reading{}
			}
			claimed = true
			return current(proxy)
		}

		if !send(claim()) {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				var reading Reading
				switch {
				case signal.Name == "org.freedesktop.DBus.NameOwnerChanged":
					if owner, ok := newOwner(signal); !ok {
						continue
					} else if owner == "" {
						claimed = false
						reading = Reading{}
					} else {
						reading = claim()
					}
				case changesOrientation(signal):
					reading = current(proxy)
				default:
					continue
				}
				if !send(reading) {
					return
				}
			}
		}
	}()
	return readings
}

func current(proxy dbus.BusObject) Reading {
	has, err := proxy.GetProperty(proxyInterface + ".HasAccelerometer")
	if err != nil {
		return Reading{}
	}
	available, _ := has.Value().(bool)
	if !available {
		return Reading{}
	}
	value, err := proxy.GetProperty(proxyInterface + ".AccelerometerOrientation")
	if err != nil {
		return Reading{}
	}
	orientation, _ := value.Value().(string)
	return Reading{Available: true, Orientation: orientation}
}

// changesOrientation reports a PropertiesChanged signal that touches the
// accelerometer.
func changesOrientation(signal *dbus.Signal) bool {
	if signal == nil || signal.Path != proxyPath || signal.Name != propsInterface+".PropertiesChanged" || len(signal.Body) < 3 {
		return false
	}
	if iface, _ := signal.Body[0].(string); iface != proxyInterface {
		return false
	}
	changed, _ := signal.Body[1].(map[string]dbus.Variant)
	_, orientation := changed["AccelerometerOrientation"]
	_, has := changed["HasAccelerometer"]
	invalidated, _ := signal.Body[2].([]string)
	for _, name := range invalidated {
		if name == "AccelerometerOrientation" || name == "HasAccelerometer" {
			return true
		}
	}
	return orientation || has
}

func newOwner(signal *dbus.Signal) (string, bool) {
	if len(signal.Body) != 3 {
		return "", false
	}
	if name, _ := signal.Body[0].(string); name != proxyService {
		return "", false
	}
	owner, ok := signal.Body[2].(string)
	return owner, ok
}
