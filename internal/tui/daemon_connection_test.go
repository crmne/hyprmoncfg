package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/ipc"
)

func replyBusyThenReady(conn net.Conn) error {
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	decoder, encoder := json.NewDecoder(conn), json.NewEncoder(conn)
	for _, busy := range []bool{true, false} {
		var request ipc.Request
		if err := decoder.Decode(&request); err != nil {
			return err
		}
		if request.Method != ipc.MethodStatus {
			return fmt.Errorf("expected status request, got %q", request.Method)
		}
		response := ipc.Response{Type: "response", ProtocolVersion: 1, ID: request.ID}
		if busy {
			response.Error = &ipc.ResponseError{Code: "compositor_busy", Message: ipc.ErrCompositorBusy.Error()}
		} else {
			response.Result = json.RawMessage(`{"version":"test","daemon":{"running":true}}`)
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return nil
}

func TestDaemonBusyRetainsConnectionUntilStatusRecovers(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	completed := make(chan error, 1)
	go func() { completed <- replyBusyThenReady(serverConn) }()
	client := ipc.NewClient(clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if running, unknown, _, _ := daemonReachable(ctx, client); running || !unknown {
		t.Fatalf("busy daemon was classified as disconnected: running=%t unknown=%t", running, unknown)
	}
	if running, unknown, version, _ := daemonReachable(ctx, client); !running || unknown || version != "test" {
		t.Fatalf("same connection did not recover: running=%t unknown=%t version=%q", running, unknown, version)
	}
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
}

func TestRedialKeepsDaemonThatRepliesBusy(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	listener, err := net.Listen("unix", filepath.Join(runtimeDir, ipc.SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	completed := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			completed <- err
			return
		}
		completed <- replyBusyThenReady(conn)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := redialDaemon(ctx)
	if client == nil {
		t.Fatal("redial discarded a live daemon because its compositor was busy")
	}
	defer client.Close()
	if running, unknown, _, _ := daemonReachable(ctx, client); !running || unknown {
		t.Fatalf("redialed connection did not recover: running=%t unknown=%t", running, unknown)
	}
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
}
