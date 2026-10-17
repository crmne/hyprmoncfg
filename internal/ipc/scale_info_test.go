package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestScaleInfoComesFromInternalScaling(t *testing.T) {
	_, path, _ := runTestServer(t, &testHandler{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := Dial(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	info, err := client.ScaleInfo(ctx, ScaleInfoParams{Width: 3840, Height: 2160, Scale: 1.33})
	if err != nil {
		t.Fatal(err)
	}
	if info.Sharp || info.Apply != 1.33333 || len(info.Presets) != 6 || len(info.Options) == 0 {
		t.Fatalf("scale_info = %+v", info)
	}
	if _, err := client.ScaleInfo(ctx, ScaleInfoParams{Width: 3840, Height: 2160, Scale: 9}); err == nil ||
		!strings.Contains(err.Error(), "scale must be between 0.25 and 4") {
		t.Fatalf("out of range: %v", err)
	}
	if _, err := client.ScaleInfo(ctx, ScaleInfoParams{Scale: 1}); err == nil {
		t.Fatal("a mode is required")
	}

	// The wire shape the panel reads.
	raw, _ := json.Marshal(info)
	for _, key := range []string{`"presets"`, `"options"`, `"sharp":false`, `"closest":1.33333`, `"closest_logical":[2880,1620]`, `"apply":1.33333`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("%s missing from %s", key, raw)
		}
	}
}

// An older daemon would ignore snap_scale and store the typed blurry scale,
// so the typed client refuses to send it there.
func TestSnapScaleRequiresADaemonWithScaleInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), SocketName)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			var request Request
			_ = json.Unmarshal(scanner.Bytes(), &request)
			reply, _ := json.Marshal(Response{Type: "response", ProtocolVersion: 1, ID: request.ID,
				Error: &ResponseError{Code: "operation_failed", Message: `unknown IPC method "` + request.Method + `"`}})
			_, _ = conn.Write(append(reply, '\n'))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := Dial(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	scale := 1.33
	_, err = client.EditProfile(ctx, EditParams{Edit: profile.EditorEdit{OutputKey: "k", Scale: &scale, SnapScale: true}})
	if err == nil || !strings.Contains(err.Error(), "requires a newer daemon") {
		t.Fatalf("snap_scale on an old daemon: %v", err)
	}
}
