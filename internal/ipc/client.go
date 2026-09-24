package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type Client struct {
	conn    net.Conn
	encoder *json.Encoder
	decoder *json.Decoder
	mu      sync.Mutex
	nextID  atomic.Uint64
}

func Dial(ctx context.Context, path string) (*Client, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	return NewClient(conn), nil
}

func NewClient(conn net.Conn) *Client {
	return &Client{
		conn:    conn,
		encoder: json.NewEncoder(conn),
		decoder: json.NewDecoder(conn),
	}
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Status(ctx context.Context) (appstatus.Document, error) {
	var result appstatus.Document
	err := c.call(ctx, MethodStatus, nil, &result)
	return result, err
}

func (c *Client) Subscribe(ctx context.Context) (appstatus.Document, error) {
	var result appstatus.Document
	err := c.call(ctx, MethodSubscribe, nil, &result)
	return result, err
}

func (c *Client) EditorState(ctx context.Context) (appstatus.EditorDocument, error) {
	var result appstatus.EditorDocument
	err := c.call(ctx, MethodEditor, nil, &result)
	return result, err
}

func (c *Client) EditProfile(ctx context.Context, params EditParams) (appstatus.EditorDraft, error) {
	var result appstatus.EditorDraft
	if err := c.checkWorkspacePersistence(ctx, params.Profile); err != nil {
		return result, err
	}
	if params.Edit.Workspaces != nil && params.Edit.Workspaces.PersistAll {
		if err := c.checkWorkspacePersistence(ctx, profile.Profile{Workspaces: *params.Edit.Workspaces}); err != nil {
			return result, err
		}
	}
	err := c.call(ctx, MethodEdit, params, &result)
	return result, err
}

func (c *Client) Preview(ctx context.Context, params PreviewParams) (Transaction, error) {
	var result Transaction
	if params.Profile != nil {
		if err := c.checkWorkspacePersistence(ctx, *params.Profile); err != nil {
			return result, err
		}
	}
	err := c.call(ctx, MethodPreview, params, &result)
	return result, err
}

func (c *Client) Confirm(ctx context.Context, transactionID string) error {
	return c.call(ctx, MethodConfirm, TransactionParams{TransactionID: transactionID}, nil)
}

func (c *Client) Commit(ctx context.Context, transactionID string, save bool) error {
	return c.call(ctx, MethodCommit, CommitParams{TransactionID: transactionID, Save: save}, nil)
}

func (c *Client) Revert(ctx context.Context, transactionID string) error {
	return c.call(ctx, MethodRevert, TransactionParams{TransactionID: transactionID}, nil)
}

func (c *Client) Save(ctx context.Context, params SaveParams) error {
	if err := c.checkWorkspacePersistence(ctx, params.Profile); err != nil {
		return err
	}
	return c.call(ctx, MethodSave, params, nil)
}

// An older daemon ignores unknown JSON fields. Refuse to send a policy it
// cannot preserve instead of appearing to save it successfully.
func (c *Client) checkWorkspacePersistence(ctx context.Context, p profile.Profile) error {
	if !p.Workspaces.PersistAll {
		return nil
	}
	document, err := c.EditorState(ctx)
	if err != nil {
		return err
	}
	if !document.WorkspacePersistenceSupported {
		return errors.New("workspace persistence requires a newer daemon; restart the updated hyprmoncfgd")
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, name string) error {
	return c.call(ctx, MethodDelete, DeleteParams{Name: name}, nil)
}

// Manage hands monitor configuration to hyprmoncfg: the include goes back into
// the Hyprland config and the daemon resumes applying profiles.
func (c *Client) Manage(ctx context.Context) error {
	return c.call(ctx, MethodManage, nil, nil)
}

// Unmanage hands it back to Hyprland: the daemon stops applying and takes its
// include out, so whatever the user or their distro configured wins again.
func (c *Client) Unmanage(ctx context.Context) error {
	return c.call(ctx, MethodUnmanage, nil, nil)
}

func (c *Client) SetProfileAuto(ctx context.Context, enabled bool) error {
	return c.call(ctx, MethodProfileAuto, ProfileAutoParams{Enabled: enabled}, nil)
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	if c == nil || c.conn == nil {
		return errors.New("IPC client is not connected")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	id := fmt.Sprintf("%d", c.nextID.Add(1))
	request := Request{
		Type:            "request",
		ProtocolVersion: ProtocolVersion,
		ID:              id,
		Method:          method,
	}
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return err
		}
		request.Params = encoded
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetDeadline(deadline); err != nil {
			return err
		}
		defer c.conn.SetDeadline(time.Time{})
	}
	if err := c.encoder.Encode(request); err != nil {
		return err
	}

	for {
		var envelope struct {
			Type string `json:"type"`
		}
		var raw json.RawMessage
		if err := c.decoder.Decode(&raw); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		if envelope.Type == "event" {
			continue
		}

		var response Response
		if err := json.Unmarshal(raw, &response); err != nil {
			return err
		}
		if response.ID != id {
			// A late reply to a call that already hit its deadline. Drop it and
			// keep reading: failing here would leave the connection one reply
			// out of step and break every later call on it.
			continue
		}
		// A daemon answers in the version we asked in, so a reply from the
		// future means it is newer than this build and did not down-negotiate.
		// Anything at or below what we speak is ours to read.
		if response.ProtocolVersion > ProtocolVersion {
			return fmt.Errorf("daemon replied in IPC protocol version %d, this build speaks %d", response.ProtocolVersion, ProtocolVersion)
		}
		if response.Error != nil {
			return decodeResponseError(response.Error)
		}
		if result != nil && len(response.Result) > 0 {
			if err := json.Unmarshal(response.Result, result); err != nil {
				return err
			}
		}
		return nil
	}
}

func decodeResponseError(responseErr *ResponseError) error {
	if responseErr == nil {
		return nil
	}
	if responseErr.Code == "transaction_unavailable" {
		return fmt.Errorf("%w: %s", ErrTransactionUnavailable, responseErr.Message)
	}
	if responseErr.Code == "compositor_busy" {
		if responseErr.Message == "" || responseErr.Message == ErrCompositorBusy.Error() {
			return ErrCompositorBusy
		}
		return fmt.Errorf("%w: %s", ErrCompositorBusy, responseErr.Message)
	}
	return fmt.Errorf("%s", responseErr.Message)
}
