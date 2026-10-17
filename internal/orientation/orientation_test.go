package orientation

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestQuartersFollowHyprlandTransforms(t *testing.T) {
	for orientation, want := range map[string]int{"normal": 0, "left-up": 1, "bottom-up": 2, "right-up": 3} {
		if got, ok := Quarters(orientation); !ok || got != want {
			t.Errorf("Quarters(%q) = %d, %v; want %d", orientation, got, ok, want)
		}
	}
	if _, ok := Quarters("undefined"); ok {
		t.Error("a device lying flat has no orientation")
	}
}

func propertiesChanged(iface string, changed map[string]dbus.Variant, invalidated []string) *dbus.Signal {
	return &dbus.Signal{Path: proxyPath, Name: propsInterface + ".PropertiesChanged", Body: []any{iface, changed, invalidated}}
}

func TestChangesOrientationOnlyForTheAccelerometer(t *testing.T) {
	if !changesOrientation(propertiesChanged(proxyInterface, map[string]dbus.Variant{"AccelerometerOrientation": dbus.MakeVariant("left-up")}, nil)) {
		t.Error("an orientation change was missed")
	}
	if !changesOrientation(propertiesChanged(proxyInterface, nil, []string{"HasAccelerometer"})) {
		t.Error("an invalidated accelerometer was missed")
	}
	if changesOrientation(propertiesChanged(proxyInterface, map[string]dbus.Variant{"LightLevel": dbus.MakeVariant(3.0)}, nil)) {
		t.Error("a light level change counted as an orientation")
	}
	if changesOrientation(propertiesChanged("net.hadess.SensorProxy.Compass", map[string]dbus.Variant{"AccelerometerOrientation": dbus.MakeVariant("normal")}, nil)) {
		t.Error("another interface counted")
	}
	if changesOrientation(&dbus.Signal{Path: proxyPath, Name: propsInterface + ".PropertiesChanged", Body: []any{proxyInterface}}) {
		t.Error("a short signal counted")
	}
}

func TestNewOwnerTracksTheSensorService(t *testing.T) {
	owner, ok := newOwner(&dbus.Signal{Body: []any{proxyService, "", ":1.42"}})
	if !ok || owner != ":1.42" {
		t.Fatalf("newOwner = %q, %v", owner, ok)
	}
	if _, ok := newOwner(&dbus.Signal{Body: []any{"org.example.Other", "", ":1.1"}}); ok {
		t.Fatal("another service's owner change counted")
	}
}
