package ipc

import (
	"context"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestPersistenceRequiresDaemonCapability(t *testing.T) {
	for _, supported := range []bool{false, true} {
		handler := &testHandler{editor: appstatus.EditorDocument{WorkspacePersistenceSupported: supported}}
		_, path, _ := runTestServer(t, handler)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		client, err := Dial(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		p := profile.Profile{Workspaces: profile.WorkspaceSettings{PersistAll: true}}
		err = client.Save(ctx, SaveParams{Profile: p})
		if (err == nil) != supported {
			t.Fatalf("save supported=%v err=%v", supported, err)
		}
		_, err = client.Preview(ctx, PreviewParams{Profile: &p})
		if (err == nil) != supported {
			t.Fatalf("preview supported=%v err=%v", supported, err)
		}
		_, err = client.EditProfile(ctx, EditParams{Edit: profile.EditorEdit{Workspaces: &p.Workspaces}})
		if (err == nil) != supported {
			t.Fatalf("edit supported=%v err=%v", supported, err)
		}
		p.Workspaces.PersistAll = false
		if err := client.Save(ctx, SaveParams{Profile: p}); err != nil {
			t.Fatalf("legacy policy blocked: %v", err)
		}
	}
}
