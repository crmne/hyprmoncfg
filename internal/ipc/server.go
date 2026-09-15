package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

type Server struct {
	Path    string
	Handler Handler
	Logf    func(format string, args ...any)

	nextConn atomic.Uint64
	mu       sync.Mutex
	clients  map[*serverClient]struct{}
	clientWG sync.WaitGroup
	notifyCh chan struct{}
	initOnce sync.Once
}

type serverClient struct {
	conn    net.Conn
	encoder *json.Encoder
	writeMu sync.Mutex
	// protocolVersion is the version this connection negotiated on its first
	// accepted request. Events go out in that version so a client that only
	// knows an older one keeps recognizing them.
	protocolVersion atomic.Int64
	subscribed      atomic.Bool
}

func (c *serverClient) eventProtocolVersion() int {
	if version := int(c.protocolVersion.Load()); version >= MinProtocolVersion {
		return version
	}
	return ProtocolVersion
}

func (s *Server) Run(ctx context.Context) error {
	if s.Handler == nil {
		return errors.New("IPC server has no handler")
	}
	if s.Path == "" {
		return errors.New("IPC server has no socket path")
	}
	s.initialize()

	listener, err := listenUnix(s.Path)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
		s.closeClients()
		s.clientWG.Wait()
		_ = os.Remove(s.Path)
	}()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	go s.publishLoop(ctx)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		client := &serverClient{conn: conn, encoder: json.NewEncoder(conn)}
		s.mu.Lock()
		s.clients[client] = struct{}{}
		s.mu.Unlock()
		owner := fmt.Sprintf("connection-%d", s.nextConn.Add(1))
		s.clientWG.Add(1)
		go func() {
			defer s.clientWG.Done()
			s.serveClient(ctx, owner, client)
		}()
	}
}

func (s *Server) Notify() {
	if s == nil {
		return
	}
	s.initialize()
	select {
	case s.notifyCh <- struct{}{}:
	default:
	}
}

func (s *Server) initialize() {
	s.initOnce.Do(func() {
		if s.Logf == nil {
			s.Logf = func(string, ...any) {}
		}
		s.notifyCh = make(chan struct{}, 1)
		s.clients = make(map[*serverClient]struct{})
	})
}

func (s *Server) publishLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.notifyCh:
			document, err := s.Handler.Status()
			if err != nil {
				s.Logf("IPC status event failed: %v", err)
				continue
			}
			data, err := json.Marshal(document)
			if err != nil {
				continue
			}
			s.broadcast(Event{
				Type:  "event",
				Event: EventStatus,
				Data:  data,
			})
		}
	}
}

func (s *Server) serveClient(ctx context.Context, owner string, client *serverClient) {
	defer func() {
		s.Handler.Disconnect(owner)
		s.mu.Lock()
		delete(s.clients, client)
		s.mu.Unlock()
		_ = client.conn.Close()
	}()

	decoder := json.NewDecoder(client.conn)
	for {
		var request Request
		if err := decoder.Decode(&request); err != nil {
			return
		}
		response := s.dispatch(owner, client, request)
		if err := client.write(response); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (s *Server) dispatch(owner string, client *serverClient, request Request) Response {
	// Reply in the version the client asked in. An older client checks the
	// version it gets back against the only one it knows, so answering in ours
	// would read as a protocol error to a client we are perfectly able to serve.
	response := Response{
		Type:                  "response",
		ProtocolVersion:       request.ProtocolVersion,
		ServerProtocolVersion: ProtocolVersion,
		ID:                    request.ID,
	}
	if request.Type != "request" {
		response.Error = &ResponseError{Code: "invalid_request", Message: "message type must be request"}
		return response
	}
	if request.ProtocolVersion < MinProtocolVersion || request.ProtocolVersion > ProtocolVersion {
		response.Error = &ResponseError{
			Code:    "unsupported_protocol",
			Message: fmt.Sprintf("unsupported IPC protocol version %d, this daemon speaks %d to %d", request.ProtocolVersion, MinProtocolVersion, ProtocolVersion),
			Data:    map[string]any{"min": MinProtocolVersion, "max": ProtocolVersion},
		}
		return response
	}
	client.protocolVersion.Store(int64(request.ProtocolVersion))

	var result any
	var err error
	switch request.Method {
	case MethodStatus:
		result, err = s.Handler.Status()
	case MethodSubscribe:
		client.subscribed.Store(true)
		result, err = s.Handler.Status()
	case MethodEditor:
		result, err = s.Handler.EditorState()
	case MethodEdit:
		var params EditParams
		if err = decodeParams(request.Params, &params); err == nil {
			result, err = s.Handler.EditProfile(params)
		}
	case MethodReuse:
		var params ReuseParams
		if err = decodeParams(request.Params, &params); err == nil {
			if handler, ok := s.Handler.(interface {
				ReuseProfile(ReuseParams) (appstatus.EditorDraft, error)
			}); ok {
				result, err = handler.ReuseProfile(params)
			} else {
				err = fmt.Errorf("layout reuse is not supported by this backend")
			}
		}
	case MethodPreview:
		var params PreviewParams
		if err = decodeParams(request.Params, &params); err == nil {
			result, err = s.Handler.Preview(owner, params)
		}
	case MethodConfirm:
		var params TransactionParams
		if err = decodeParams(request.Params, &params); err == nil {
			err = s.Handler.Confirm(owner, params)
		}
	case MethodCommit:
		var params CommitParams
		if err = decodeParams(request.Params, &params); err == nil {
			err = s.Handler.Commit(owner, params)
		}
	case MethodRevert:
		var params TransactionParams
		if err = decodeParams(request.Params, &params); err == nil {
			err = s.Handler.Revert(owner, params)
		}
	case MethodSave:
		var params SaveParams
		if err = decodeParams(request.Params, &params); err == nil {
			err = s.Handler.Save(params)
		}
	case MethodDelete:
		var params DeleteParams
		if err = decodeParams(request.Params, &params); err == nil {
			err = s.Handler.Delete(params)
		}
	case MethodManage:
		err = s.Handler.Manage()
	case MethodUnmanage:
		err = s.Handler.Unmanage()
	case MethodProfileAuto:
		var params ProfileAutoParams
		if err = decodeParams(request.Params, &params); err == nil {
			err = s.Handler.SetProfileAuto(params)
		}
	default:
		err = fmt.Errorf("unknown IPC method %q", request.Method)
	}
	if err != nil {
		response.Error = encodeResponseError(err)
		return response
	}
	if result != nil {
		response.Result, err = json.Marshal(result)
		if err != nil {
			response.Error = encodeResponseError(err)
		}
	}
	return response
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return errors.New("missing request parameters")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("invalid request parameters: %w", err)
	}
	return nil
}

func encodeResponseError(err error) *ResponseError {
	if errors.Is(err, ErrCompositorBusy) {
		return &ResponseError{Code: "compositor_busy", Message: ErrCompositorBusy.Error()}
	}
	if errors.Is(err, ErrTransactionUnavailable) {
		return &ResponseError{Code: "transaction_unavailable", Message: err.Error()}
	}
	return &ResponseError{Code: "operation_failed", Message: err.Error()}
}

func (s *Server) broadcast(event Event) {
	s.mu.Lock()
	clients := make([]*serverClient, 0, len(s.clients))
	for client := range s.clients {
		if client.subscribed.Load() {
			clients = append(clients, client)
		}
	}
	s.mu.Unlock()
	for _, client := range clients {
		stamped := event
		stamped.ProtocolVersion = client.eventProtocolVersion()
		if err := client.write(stamped); err != nil {
			_ = client.conn.Close()
		}
	}
}

func (s *Server) closeClients() {
	s.mu.Lock()
	clients := make([]*serverClient, 0, len(s.clients))
	for client := range s.clients {
		clients = append(clients, client)
	}
	s.mu.Unlock()
	for _, client := range clients {
		_ = client.conn.Close()
	}
}

func (c *serverClient) write(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.encoder.Encode(value)
}

func listenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing to replace non-socket IPC path %s", path)
		}
		probe, dialErr := net.Dial("unix", path)
		if dialErr == nil {
			_ = probe.Close()
			return nil, fmt.Errorf("IPC socket already in use: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return listener, nil
}
