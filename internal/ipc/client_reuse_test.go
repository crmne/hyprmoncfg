package ipc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type clientReuseHandler struct {
	testHandler
	requests chan ReuseParams
	err      error
}

func (h *clientReuseHandler) ReuseProfile(params ReuseParams) (appstatus.EditorDraft, error) {
	h.requests <- params
	return appstatus.EditorDraft{
		Profile:  profile.Profile{Outputs: []profile.OutputConfig{{Key: "current-left", Name: "DP-2"}}},
		Warnings: []string{"Skipped a saved role."},
	}, h.err
}

func TestClientReuseProfileRoundTrip(t *testing.T) {
	for _, busy := range []bool{false, true} {
		t.Run(fmt.Sprintf("busy=%t", busy), func(t *testing.T) {
			handler := &clientReuseHandler{requests: make(chan ReuseParams, 1)}
			if busy {
				handler.err = fmt.Errorf("monitor read: %w", ErrCompositorBusy)
			}
			_, path, _ := runTestServer(t, handler)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			client, err := Dial(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			result, err := client.ReuseProfile(ctx, ReuseParams{
				Name: "Office", Mapping: map[string]string{"old-left": "current-left", "old-right": ""},
			})
			if busy {
				if !errors.Is(err, ErrCompositorBusy) || err.Error() != ErrCompositorBusy.Error() {
					t.Fatalf("expected retryable busy error without duplicated text, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if result.Profile.Name != "" || len(result.Profile.Outputs) != 1 || result.Profile.Outputs[0].Key != "current-left" || len(result.Warnings) != 1 {
					t.Fatalf("lost reused draft or warnings: %+v", result)
				}
			}
			select {
			case params := <-handler.requests:
				if params.Name != "Office" || params.Mapping["old-left"] != "current-left" {
					t.Fatalf("wrong reuse request: %+v", params)
				}
				if value, ok := params.Mapping["old-right"]; !ok || value != "" {
					t.Fatal("explicit skipped role was lost")
				}
			case <-ctx.Done():
				t.Fatal("reuse request did not reach server")
			}
		})
	}
}
