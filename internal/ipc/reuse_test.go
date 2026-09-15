package ipc

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
)

type reuseHandler struct {
	testHandler
	params ReuseParams
}

func (h *reuseHandler) ReuseProfile(params ReuseParams) (appstatus.EditorDraft, error) {
	h.params = params
	return appstatus.EditorDraft{Warnings: []string{"Skipped a saved role."}}, nil
}

func TestReuseIPCDecodesMappingAndReturnsWarnings(t *testing.T) {
	handler := &reuseHandler{}
	server := &Server{Handler: handler}
	response := server.dispatch("test", &serverClient{}, Request{
		Type: "request", ProtocolVersion: 1, ID: "reuse", Method: MethodReuse,
		Params: json.RawMessage(`{"name":"Office","mapping":{"old-left":"new-right","old-right":""}}`),
	})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	if handler.params.Name != "Office" || handler.params.Mapping["old-left"] != "new-right" {
		t.Fatalf("bad decoded request: %+v", handler.params)
	}
	if value, ok := handler.params.Mapping["old-right"]; !ok || value != "" {
		t.Fatal("explicit skip was lost")
	}
	var result appstatus.EditorDraft
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Profile.Name != "" || len(result.Warnings) != 1 {
		t.Fatalf("invalid draft response: %+v", result)
	}
}

func TestCompositorBusyHasStableWireError(t *testing.T) {
	response := encodeResponseError(fmt.Errorf("monitor read: %w", ErrCompositorBusy))
	if response.Code != "compositor_busy" || response.Message != ErrCompositorBusy.Error() {
		t.Fatalf("invalid timeout response: %+v", response)
	}
}
