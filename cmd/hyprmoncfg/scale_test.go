package main

import (
	"context"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestNextScaleStepsLikeOmarchy(t *testing.T) {
	for _, tc := range []struct {
		width, height int
		current       float64
		request       string
		want          float64
		ok            bool
	}{
		{2880, 1800, 1.6, "up", 2, true},
		{2880, 1800, 1.6, "down", 1.25, true},
		{2880, 1800, 1, "down", 0, false},
		{2560, 1600, 1.25, "up", 1.6, true},
		{1920, 1080, 4, "up", 0, false},
		{2560, 1440, 0, "1.5", 1.6, true}, // 1.5 is blurry here; it rounds up to a sharp scale
		{2560, 1440, 1, "nope", 0, false},
	} {
		got, ok := nextScale(tc.width, tc.height, tc.current, tc.request)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Fatalf("%dx%d at %v %s = %v,%v want %v,%v", tc.width, tc.height, tc.current, tc.request, got, ok, tc.want, tc.ok)
		}
	}
}

type fakeScaleClient struct {
	document  appstatus.EditorDocument
	edit      profile.EditorEdit
	previewed profile.Profile
	saveOn    bool
	committed bool
	confirmed bool
}

func (f *fakeScaleClient) EditorState(context.Context) (appstatus.EditorDocument, error) {
	return f.document, nil
}

func (f *fakeScaleClient) EditProfile(_ context.Context, params ipc.EditParams) (appstatus.EditorDraft, error) {
	f.edit = params.Edit
	p := params.Profile
	for i := range p.Outputs {
		if p.Outputs[i].Key == params.Edit.OutputKey {
			p.Outputs[i].Scale = *params.Edit.Scale
		}
	}
	return appstatus.EditorDraft{Profile: p}, nil
}

func (f *fakeScaleClient) Preview(_ context.Context, params ipc.PreviewParams) (ipc.Transaction, error) {
	f.previewed, f.saveOn = *params.Profile, params.SaveOnCommit
	return ipc.Transaction{ID: "t1"}, nil
}

func (f *fakeScaleClient) Commit(_ context.Context, id string, save bool) error {
	f.committed = id == "t1" && save
	return nil
}

func (f *fakeScaleClient) Confirm(_ context.Context, id string) error {
	f.confirmed = id == "t1"
	return nil
}

func scaleDocument(source string) appstatus.EditorDocument {
	return appstatus.EditorDocument{
		SourceProfile: source,
		Profile: profile.Profile{Name: source, Outputs: []profile.OutputConfig{
			{Key: "laptop", Name: "eDP-1", Enabled: true, Width: 2880, Height: 1800, Scale: 1.6},
			{Key: "desk", Name: "DP-1", Enabled: true, Width: 2560, Height: 1440, Scale: 1},
		}},
		Displays: []appstatus.EditorDisplay{{Key: "laptop", Focused: true}, {Key: "desk"}},
	}
}

// The scale keys must change the profile in use, or the generated config
// puts the old scale back on the next reload (#131).
func TestScaleSavesTheFocusedDisplayToTheActiveProfile(t *testing.T) {
	client := &fakeScaleClient{document: scaleDocument("Laptop")}
	scale, saved, err := runScale(context.Background(), client, "up")
	if err != nil {
		t.Fatal(err)
	}
	if scale != 2 || saved != "Laptop" || client.edit.OutputKey != "laptop" {
		t.Fatalf("scale=%v saved=%q edit=%+v", scale, saved, client.edit)
	}
	if client.previewed.Name != "Laptop" || !client.saveOn || !client.committed || client.confirmed {
		t.Fatalf("not committed and saved: %+v", client)
	}
}

func TestScaleOnAnUnsavedLayoutOnlyApplies(t *testing.T) {
	client := &fakeScaleClient{document: scaleDocument("")}
	if _, saved, err := runScale(context.Background(), client, "down"); err != nil || saved != "" {
		t.Fatalf("saved=%q err=%v", saved, err)
	}
	if client.saveOn || client.committed || !client.confirmed || client.previewed.Name == "" {
		t.Fatalf("an unsaved layout must be applied, not saved: %+v", client)
	}
}

func TestScaleAtTheEndOfTheLadderChangesNothing(t *testing.T) {
	document := scaleDocument("Laptop")
	document.Profile.Outputs[0].Scale = 1
	client := &fakeScaleClient{document: document}
	if scale, _, err := runScale(context.Background(), client, "down"); err != nil || scale != 1 || client.edit.OutputKey != "" {
		t.Fatalf("scale=%v err=%v edit=%+v", scale, err, client.edit)
	}
}
