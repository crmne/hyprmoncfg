package ipc

import (
	"context"
	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"testing"
)

func TestCalibrationCapabilityAndWireRouting(t *testing.T) {
	for _, supported := range []bool{false, true} {
		handler := &testHandler{document: appstatus.Document{Rotation: &appstatus.Rotation{CalibrationSupported: supported}}}
		_, path, _ := runTestServer(t, handler)
		client, err := Dial(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		err = client.SetRotation(context.Background(), RotationParams{Calibrate: true})
		client.Close()
		if (err == nil) != supported {
			t.Fatalf("supported %v: %v", supported, err)
		}
		handler.mu.Lock()
		got := handler.rotation.Calibrate
		handler.mu.Unlock()
		if got != supported {
			t.Fatalf("old daemon received calibration: %v", got)
		}
	}
}
