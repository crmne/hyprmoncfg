package ipc

import (
	"errors"
	"fmt"
	"testing"
)

func TestCompositorBusyHasStableWireError(t *testing.T) {
	response := encodeResponseError(fmt.Errorf("monitor read: %w", ErrCompositorBusy))
	if response.Code != "compositor_busy" || response.Message != ErrCompositorBusy.Error() {
		t.Fatalf("invalid timeout response: %+v", response)
	}
	decoded := decodeResponseError(response)
	if !errors.Is(decoded, ErrCompositorBusy) || decoded.Error() != ErrCompositorBusy.Error() {
		t.Fatalf("lost typed busy error or duplicated its message: %v", decoded)
	}
}

func TestCompositorBusyDecodingPreservesOptionalDetail(t *testing.T) {
	for _, message := range []string{"", ErrCompositorBusy.Error(), "monitor read timed out"} {
		err := decodeResponseError(&ResponseError{Code: "compositor_busy", Message: message})
		if !errors.Is(err, ErrCompositorBusy) {
			t.Fatalf("%q lost the retryable error: %v", message, err)
		}
	}
}
