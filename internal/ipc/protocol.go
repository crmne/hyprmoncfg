package ipc

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// ProtocolVersion is the newest protocol this build speaks. Bump it when the
// wire format gains something a client has to know about.
//
// MinProtocolVersion is the oldest one it still answers. Clients update on
// their own schedule -- the Omarchy panel is a separate git checkout, and the
// daemon keeps running its old binary until someone restarts the service -- so
// a new daemon has to keep serving older clients. The server answers in the
// version the client asked in, which leaves an older client seeing exactly the
// replies it expects, and reports its own newest version separately so a client
// that cares can adapt.
const (
	ProtocolVersion    = 1
	MinProtocolVersion = 1
)

const (
	MethodStatus      = "status"
	MethodSubscribe   = "subscribe"
	MethodEditor      = "editor_state"
	MethodEdit        = "edit_profile"
	MethodPreview     = "preview"
	MethodConfirm     = "confirm"
	MethodCommit      = "commit"
	MethodRevert      = "revert"
	MethodSave        = "save"
	MethodDelete      = "delete"
	MethodManage      = "manage"
	MethodUnmanage    = "unmanage"
	MethodProfileAuto = "set_profile_auto"
)

const EventStatus = "status"

var ErrCompositorBusy = errors.New("Displays are still connecting; try again shortly.")

var ErrTransactionUnavailable = errors.New("interactive preview is no longer available")

type Request struct {
	Type            string          `json:"type"`
	ProtocolVersion int             `json:"protocol_version"`
	ID              string          `json:"id"`
	Method          string          `json:"method"`
	Params          json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocol_version"`
	// ServerProtocolVersion is the newest version the daemon speaks, whatever
	// version this reply is written in. A client reads it to feature-detect
	// instead of guessing from the daemon's release number.
	ServerProtocolVersion int             `json:"server_protocol_version,omitempty"`
	ID                    string          `json:"id"`
	Result                json.RawMessage `json:"result,omitempty"`
	Error                 *ResponseError  `json:"error,omitempty"`
}

type Event struct {
	Type            string          `json:"type"`
	ProtocolVersion int             `json:"protocol_version"`
	Event           string          `json:"event"`
	Data            json.RawMessage `json:"data,omitempty"`
}

type ResponseError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

type PreviewParams struct {
	Profile        *profile.Profile `json:"profile,omitempty"`
	ProfileName    string           `json:"profile_name,omitempty"`
	TimeoutSeconds int              `json:"timeout_seconds,omitempty"`
	SaveOnCommit   bool             `json:"save_on_commit,omitempty"`
}

type EditParams struct {
	Profile profile.Profile    `json:"profile"`
	Edit    profile.EditorEdit `json:"edit"`
}

type TransactionParams struct {
	TransactionID string `json:"transaction_id"`
}

type CommitParams struct {
	TransactionID string `json:"transaction_id"`
	Save          bool   `json:"save"`
}

type SaveParams struct {
	Profile profile.Profile `json:"profile"`
}

type DeleteParams struct {
	Name string `json:"name"`
}

type ProfileAutoParams struct {
	Enabled bool `json:"enabled"`
}

type Transaction struct {
	ID       string          `json:"id"`
	Profile  profile.Profile `json:"profile"`
	Deadline time.Time       `json:"deadline"`
}

type Handler interface {
	Status() (appstatus.Document, error)
	EditorState() (appstatus.EditorDocument, error)
	EditProfile(params EditParams) (appstatus.EditorDraft, error)
	Preview(owner string, params PreviewParams) (Transaction, error)
	Confirm(owner string, params TransactionParams) error
	Commit(owner string, params CommitParams) error
	Revert(owner string, params TransactionParams) error
	Save(params SaveParams) error
	Delete(params DeleteParams) error
	// Manage and Unmanage move monitor configuration between hyprmoncfg and
	// Hyprland's own config. Unmanage has to stop the daemon applying as well as
	// take the include out, or the next monitor event puts it straight back.
	Manage() error
	Unmanage() error
	SetProfileAuto(params ProfileAutoParams) error
	Disconnect(owner string)
}
