package orientation

import (
	"context"
	"testing"
	"time"

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

func TestRefusedClaimsAreRetriedButMissingServicesWait(t *testing.T) {
	if !retryableClaimError(dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"}) {
		t.Error("a polkit refusal before the session is active was not retried")
	}
	if !retryableClaimError(context.DeadlineExceeded) {
		t.Error("a slow claim was not retried")
	}
	for _, name := range []string{"org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner"} {
		if retryableClaimError(dbus.Error{Name: name}) {
			t.Errorf("%s retried; NameOwnerChanged announces the service", name)
		}
	}
	if retryableClaimError(context.Canceled) {
		t.Error("a claim cut short by shutdown was retried")
	}
}

// fakeProxy stands in for iio-sensor-proxy on the bus. The embedded nil
// BusObject panics on anything follow is not expected to call.
type fakeProxy struct {
	dbus.BusObject
	claimErrs chan error
	claims    chan struct{}
}

func (p *fakeProxy) CallWithContext(_ context.Context, method string, _ dbus.Flags, _ ...any) *dbus.Call {
	if method != proxyInterface+".ClaimAccelerometer" {
		return &dbus.Call{}
	}
	p.claims <- struct{}{}
	return &dbus.Call{Err: <-p.claimErrs}
}

func (p *fakeProxy) GetProperty(name string) (dbus.Variant, error) {
	if name == proxyInterface+".HasAccelerometer" {
		return dbus.MakeVariant(true), nil
	}
	return dbus.MakeVariant("right-up"), nil
}

func nextReading(t *testing.T, readings <-chan Reading) Reading {
	t.Helper()
	select {
	case reading := <-readings:
		return reading
	case <-time.After(5 * time.Second):
		t.Fatal("no reading")
		return Reading{}
	}
}

func TestARefusedClaimReportsPendingUntilTheSessionIsActive(t *testing.T) {
	restoreMin, restoreMax := claimRetryMin, claimRetryMax
	claimRetryMin, claimRetryMax = time.Millisecond, 2*time.Millisecond
	t.Cleanup(func() { claimRetryMin, claimRetryMax = restoreMin, restoreMax })

	proxy := &fakeProxy{claimErrs: make(chan error, 3), claims: make(chan struct{}, 3)}
	refused := dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"}
	proxy.claimErrs <- refused
	proxy.claimErrs <- refused
	proxy.claimErrs <- nil

	ctx, cancel := context.WithCancel(context.Background())
	readings := make(chan Reading, 4)
	signals := make(chan *dbus.Signal, 1)
	done := make(chan struct{})
	go func() { defer close(done); follow(ctx, proxy, signals, readings) }()

	for range 2 {
		if got := nextReading(t, readings); got != (Reading{ClaimPending: true}) {
			t.Fatalf("refused claim read %+v", got)
		}
	}
	if got := nextReading(t, readings); got != (Reading{Available: true, Orientation: "right-up"}) {
		t.Fatalf("granted claim read %+v", got)
	}
	cancel()
	<-done
}

func TestAMissingServiceIsNotRetriedOrReportedPending(t *testing.T) {
	proxy := &fakeProxy{claimErrs: make(chan error, 1), claims: make(chan struct{}, 2)}
	proxy.claimErrs <- dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"}

	ctx, cancel := context.WithCancel(context.Background())
	readings := make(chan Reading, 4)
	signals := make(chan *dbus.Signal, 1)
	done := make(chan struct{})
	go func() { defer close(done); follow(ctx, proxy, signals, readings) }()

	if got := nextReading(t, readings); got != (Reading{}) {
		t.Fatalf("missing service read %+v", got)
	}
	cancel()
	<-done
	if len(proxy.claims) != 1 {
		t.Fatalf("claimed %d times", len(proxy.claims))
	}
}

func TestPropertyChangesAreIgnoredUntilClaimed(t *testing.T) {
	restoreMin := claimRetryMin
	claimRetryMin = time.Hour
	t.Cleanup(func() { claimRetryMin = restoreMin })

	proxy := &fakeProxy{claimErrs: make(chan error, 1), claims: make(chan struct{}, 2)}
	proxy.claimErrs <- dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"}

	ctx, cancel := context.WithCancel(context.Background())
	readings := make(chan Reading, 4)
	signals := make(chan *dbus.Signal, 1)
	done := make(chan struct{})
	go func() { defer close(done); follow(ctx, proxy, signals, readings) }()

	nextReading(t, readings)
	signals <- &dbus.Signal{
		Path: proxyPath,
		Name: propsInterface + ".PropertiesChanged",
		Body: []any{proxyInterface, map[string]dbus.Variant{"HasAccelerometer": dbus.MakeVariant(true)}, []string{}},
	}
	// Closing the stream ends follow, after it has handled the signal above.
	close(signals)
	<-done
	cancel()
	if len(readings) != 0 {
		t.Fatalf("unclaimed sensor reported %+v", <-readings)
	}
}
