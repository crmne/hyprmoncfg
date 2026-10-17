package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type testHandler struct {
	mu           sync.Mutex
	document     appstatus.Document
	preview      Transaction
	previewErr   error
	revertErr    error
	managed      bool
	profileAuto  bool
	rotationLock bool
	disconnected []string
	editor       appstatus.EditorDocument
	edited       appstatus.EditorDraft
}

func (h *testHandler) Status() (appstatus.Document, error)            { return h.document, nil }
func (h *testHandler) EditorState() (appstatus.EditorDocument, error) { return h.editor, nil }
func (h *testHandler) EditProfile(_ EditParams) (appstatus.EditorDraft, error) {
	return h.edited, nil
}

func (h *testHandler) Preview(_ string, _ PreviewParams) (Transaction, error) {
	return h.preview, h.previewErr
}

func (h *testHandler) Confirm(_ string, _ TransactionParams) error { return nil }
func (h *testHandler) Commit(_ string, _ CommitParams) error       { return nil }
func (h *testHandler) Revert(_ string, _ TransactionParams) error  { return h.revertErr }
func (h *testHandler) Save(_ SaveParams) error                     { return nil }
func (h *testHandler) Delete(_ DeleteParams) error                 { return nil }

func (h *testHandler) Manage() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.managed = true
	return nil
}

func (h *testHandler) Unmanage() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.managed = false
	return nil
}

func (h *testHandler) SetProfileAuto(params ProfileAutoParams) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.profileAuto = params.Enabled
	return nil
}

func (h *testHandler) SetRotationLock(params RotationLockParams) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rotationLock = params.Locked
	return nil
}

func (h *testHandler) Disconnect(owner string) {
	h.mu.Lock()
	h.disconnected = append(h.disconnected, owner)
	h.mu.Unlock()
}

func (h *testHandler) disconnectCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.disconnected)
}

func runTestServer(t *testing.T, handler Handler) (*Server, string, context.CancelFunc) {
	t.Helper()
	path := filepath.Join(t.TempDir(), SocketName)
	server := &Server{Path: path, Handler: handler}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("IPC socket was not created")
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("server shutdown: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("server did not shut down")
		}
	})
	return server, path, cancel
}

func TestClientStatusAndPreview(t *testing.T) {
	handler := &testHandler{
		document: appstatus.Document{
			SchemaVersion: 1,
			Version:       "test",
			Daemon:        appstatus.Daemon{Running: true},
		},
		preview: Transaction{
			ID:       "transaction-1",
			Profile:  profile.New("desk", nil),
			Deadline: time.Now().Add(10 * time.Second).UTC(),
		},
		editor: appstatus.EditorDocument{Profile: profile.Profile{Name: "desk"}},
		edited: appstatus.EditorDraft{Profile: profile.Profile{Name: "edited"}},
	}
	_, path, _ := runTestServer(t, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := Dial(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	document, err := client.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if document.Version != "test" || !document.Daemon.Running {
		t.Fatalf("unexpected status: %+v", document)
	}
	editor, err := client.EditorState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if editor.Profile.Name != "desk" {
		t.Fatalf("unexpected editor state: %+v", editor)
	}
	edited, err := client.EditProfile(ctx, EditParams{})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Profile.Name != "edited" {
		t.Fatalf("unexpected edited profile: %+v", edited)
	}
	transaction, err := client.Preview(ctx, PreviewParams{ProfileName: "desk", TimeoutSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	if transaction.ID != "transaction-1" || transaction.Profile.Name != "desk" {
		t.Fatalf("unexpected transaction: %+v", transaction)
	}
}

func TestClientPreservesUnavailableTransactionError(t *testing.T) {
	handler := &testHandler{revertErr: ErrTransactionUnavailable}
	_, path, _ := runTestServer(t, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := Dial(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	err = client.Revert(ctx, "expired")
	if !errors.Is(err, ErrTransactionUnavailable) {
		t.Fatalf("expected transaction unavailable error, got %v", err)
	}
}

func TestSubscribeReceivesStatusEvents(t *testing.T) {
	handler := &testHandler{document: appstatus.Document{SchemaVersion: 1, Version: "event"}}
	server, path, _ := runTestServer(t, handler)

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)
	if err := encoder.Encode(Request{Type: "request", ProtocolVersion: 1, ID: "1", Method: MethodSubscribe}); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := decoder.Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Error != nil {
		t.Fatalf("subscribe failed: %+v", response.Error)
	}

	server.Notify()
	var event Event
	if err := decoder.Decode(&event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "event" || event.Event != EventStatus || event.ProtocolVersion != ProtocolVersion {
		t.Fatalf("unexpected event: %+v", event)
	}
	var document appstatus.Document
	if err := json.Unmarshal(event.Data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Version != "event" {
		t.Fatalf("unexpected event document: %+v", document)
	}
}

func TestShutdownClosesClientsAndWaitsForDisconnect(t *testing.T) {
	handler := &testHandler{}
	_, path, cancel := runTestServer(t, handler)
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(conn).Encode(Request{Type: "request", ProtocolVersion: 1, ID: "1", Method: MethodStatus}); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatal(err)
	}

	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for handler.disconnectCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if handler.disconnectCount() != 1 {
		t.Fatalf("disconnect calls = %d, want 1", handler.disconnectCount())
	}
}

// dispatchRaw drives the server the way a non-Go client does, one JSON request
// at a time, so a request can carry a version this build would never send.
func dispatchRaw(t *testing.T, request Request) Response {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close(); _ = clientConn.Close() })

	server := &Server{Handler: &testHandler{}}
	client := &serverClient{conn: serverConn, encoder: json.NewEncoder(serverConn)}
	return server.dispatch("test", client, request)
}

func TestDispatchAnswersInTheVersionTheClientAsked(t *testing.T) {
	// Every version this build still serves has to come back in that same
	// version, because an older client compares it against the only one it
	// knows. This grows teeth the moment ProtocolVersion moves past the min.
	for version := MinProtocolVersion; version <= ProtocolVersion; version++ {
		request := Request{Type: "request", ProtocolVersion: version, ID: "1", Method: MethodStatus}
		response := dispatchRaw(t, request)

		if response.Error != nil {
			t.Fatalf("version %d was refused: %+v", version, response.Error)
		}
		if response.ProtocolVersion != version {
			t.Fatalf("version %d answered in %d", version, response.ProtocolVersion)
		}
		if response.ServerProtocolVersion != ProtocolVersion {
			t.Fatalf("server version = %d, want %d", response.ServerProtocolVersion, ProtocolVersion)
		}
	}
}

func TestDispatchRefusesVersionsOutsideTheSupportedRange(t *testing.T) {
	for _, version := range []int{MinProtocolVersion - 1, ProtocolVersion + 1} {
		request := Request{Type: "request", ProtocolVersion: version, ID: "1", Method: MethodStatus}
		response := dispatchRaw(t, request)

		if response.Error == nil || response.Error.Code != "unsupported_protocol" {
			t.Fatalf("version %d was accepted: %+v", version, response.Error)
		}
		// The refusal is echoed in the caller's version too. Answering in ours
		// would trip the client's own version check and hide this message.
		if response.ProtocolVersion != version {
			t.Fatalf("refusal for version %d came back in %d", version, response.ProtocolVersion)
		}
		if response.Error.Data["min"] != MinProtocolVersion || response.Error.Data["max"] != ProtocolVersion {
			t.Fatalf("refusal should name the supported range, got %+v", response.Error.Data)
		}
	}
}

func TestEventsGoOutInTheVersionTheClientNegotiated(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close(); _ = clientConn.Close() })

	client := &serverClient{conn: serverConn, encoder: json.NewEncoder(serverConn)}
	if got := client.eventProtocolVersion(); got != ProtocolVersion {
		t.Fatalf("un-negotiated client should fall back to %d, got %d", ProtocolVersion, got)
	}

	server := &Server{Handler: &testHandler{}}
	server.dispatch("test", client, Request{
		Type: "request", ProtocolVersion: MinProtocolVersion, ID: "1", Method: MethodSubscribe,
	})

	if got := client.eventProtocolVersion(); got != MinProtocolVersion {
		t.Fatalf("event version = %d, want the negotiated %d", got, MinProtocolVersion)
	}
	if !client.subscribed.Load() {
		t.Fatal("subscribe should have marked the client subscribed")
	}
}

func TestDispatchRoutesManageAndUnmanage(t *testing.T) {
	handler := &testHandler{}
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close(); _ = clientConn.Close() })

	server := &Server{Handler: handler}
	client := &serverClient{conn: serverConn, encoder: json.NewEncoder(serverConn)}

	for _, test := range []struct {
		method string
		want   bool
	}{{MethodManage, true}, {MethodUnmanage, false}} {
		response := server.dispatch("test", client, Request{
			Type: "request", ProtocolVersion: ProtocolVersion, ID: "1", Method: test.method,
		})
		if response.Error != nil {
			t.Fatalf("%s: %+v", test.method, response.Error)
		}
		handler.mu.Lock()
		got := handler.managed
		handler.mu.Unlock()
		if got != test.want {
			t.Fatalf("after %s, managed = %v, want %v", test.method, got, test.want)
		}
	}
}

func TestDispatchRoutesProfileAutomaticMode(t *testing.T) {
	handler := &testHandler{}
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close(); _ = clientConn.Close() })
	server := &Server{Handler: handler}
	client := &serverClient{conn: serverConn, encoder: json.NewEncoder(serverConn)}
	params, err := json.Marshal(ProfileAutoParams{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	response := server.dispatch("test", client, Request{
		Type: "request", ProtocolVersion: ProtocolVersion, ID: "1", Method: MethodProfileAuto, Params: params,
	})
	if response.Error != nil {
		t.Fatalf("%s: %+v", MethodProfileAuto, response.Error)
	}
	handler.mu.Lock()
	got := handler.profileAuto
	handler.mu.Unlock()
	if !got {
		t.Fatal("profile automatic mode was not routed to the handler")
	}
}

func TestDispatchRoutesRotationLock(t *testing.T) {
	handler := &testHandler{}
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close(); _ = clientConn.Close() })
	server := &Server{Handler: handler}
	client := &serverClient{conn: serverConn, encoder: json.NewEncoder(serverConn)}
	params, err := json.Marshal(RotationLockParams{Locked: true})
	if err != nil {
		t.Fatal(err)
	}
	response := server.dispatch("test", client, Request{
		Type: "request", ProtocolVersion: ProtocolVersion, ID: "1", Method: MethodRotationLock, Params: params,
	})
	if response.Error != nil {
		t.Fatalf("%s: %+v", MethodRotationLock, response.Error)
	}
	handler.mu.Lock()
	got := handler.rotationLock
	handler.mu.Unlock()
	if !got {
		t.Fatal("the rotation lock was not routed to the handler")
	}
}
