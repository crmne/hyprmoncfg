package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/buildinfo"
	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/profileio"
)

type pendingTransaction struct {
	id           string
	owner        string
	requested    profile.Profile
	profile      profile.Profile
	snapshot     apply.RevertState
	deadline     time.Time
	monitorSet   string
	saveOnCommit bool
	wasManual    bool
	timer        *time.Timer
}

func (s *Service) Status() (appstatus.Document, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	profiles, err := s.store.List()
	if err != nil {
		return appstatus.Document{}, err
	}
	monitors, err := s.client.Monitors(ctx)
	if err != nil {
		return appstatus.Document{}, err
	}
	rules, err := s.client.WorkspaceRules(ctx)
	if err != nil {
		return appstatus.Document{}, err
	}
	document := appstatus.Build(buildinfo.Version, true, profiles, monitors, rules)
	document.Daemon.Unmanaged = !config.IsManaged(s.cfg.ConfigDir)
	if manual, ok := s.manualOverride(profile.MonitorSetHash(monitors)); ok {
		document.Daemon.ProfileOverride = manual.Name
	}
	s.pendingMu.Lock()
	if pending := s.pending; pending != nil {
		document.Daemon.Preview = &appstatus.PreviewReference{
			TransactionID: pending.id,
			Reclaimable:   pending.owner == "",
			ProfileName:   pending.profile.Name,
			Deadline:      pending.deadline,
			SaveOnCommit:  pending.saveOnCommit,
			Profile:       pending.profile,
		}
	}
	s.pendingMu.Unlock()
	return document, nil
}

func (s *Service) EditorState() (appstatus.EditorDocument, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	profiles, err := s.store.List()
	if err != nil {
		return appstatus.EditorDocument{}, err
	}
	monitors, err := s.client.Monitors(ctx)
	if err != nil {
		return appstatus.EditorDocument{}, err
	}
	rules, err := s.client.WorkspaceRules(ctx)
	if err != nil {
		return appstatus.EditorDocument{}, err
	}
	return appstatus.BuildEditor(profiles, monitors, rules), nil
}

func (s *Service) EditProfile(params ipc.EditParams) (appstatus.EditorDraft, error) {
	edited, err := profile.ApplyEditorEdit(params.Profile, params.Edit)
	if err != nil {
		return appstatus.EditorDraft{}, err
	}
	return appstatus.BuildEditorDraft(edited), nil
}

// Manage hands monitor configuration to hyprmoncfg: Omarchy's watcher steps
// aside, automatic switching resumes, and the next apply puts the include back.
func (s *Service) Manage() error {
	if err := config.SetManaged(s.cfg.ConfigDir, true); err != nil {
		return err
	}
	if s.cfg.ClaimWatcher != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		s.cfg.ClaimWatcher(ctx)
		cancel()
	}
	s.cfg.Logf("monitor management on")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.applyBest(ctx); err != nil && !errors.Is(err, errDisplaysSleeping) {
		s.cfg.Logf("could not apply a profile after turning management on: %v", err)
	}
	s.signalChange()
	return nil
}

// Unmanage hands it back to Hyprland: automatic switching stops, the include
// comes out so the user's own monitor config has the last word again, and
// Omarchy's watcher resumes.
//
// The order matters. Recording the choice first means an apply racing this call
// bails out; taking the write lock afterwards waits for one already running.
// Removing the include while an apply could still land would just see it added
// straight back.
func (s *Service) Unmanage() error {
	if err := config.SetManaged(s.cfg.ConfigDir, false); err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s.pendingMu.Lock()
	pending := s.pending
	s.pendingMu.Unlock()
	if pending != nil {
		if err := s.restorePending(pending); err != nil {
			return err
		}
	}
	s.applied = nil
	s.lastProfile = profile.Profile{}
	s.lastMonitorSet = ""

	if resolved, err := s.resolveHyprConfig(ctx); err != nil {
		s.cfg.Logf("could not resolve the Hyprland config: %v", err)
	} else {
		result, err := config.RemoveInclude(resolved.RootPath, resolved.Format)
		switch {
		case err != nil:
			s.cfg.Logf("could not remove hyprmoncfg's include from %s: %v", resolved.RootPath, err)
		case result.ReadOnly:
			s.cfg.Logf("%s is read-only, so its hyprmoncfg include is yours to remove", resolved.RootPath)
		case result.Removed:
			s.cfg.Logf("removed hyprmoncfg's include from %s", resolved.RootPath)
		}
	}

	if s.cfg.ReleaseWatcher != nil {
		if err := s.cfg.ReleaseWatcher(ctx); err != nil {
			s.cfg.Logf("could not restore Omarchy monitor watcher: %v", err)
		}
	}
	// Hyprland is still running the layout it already loaded, so without this
	// the hand-back does not show up until something else triggers a reload.
	if err := s.client.Reload(ctx); err != nil {
		s.cfg.Logf("could not reload Hyprland: %v", err)
	}

	s.cfg.Logf("monitor management off")
	s.signalChange()
	return nil
}

// SetProfileAuto separates ownership of display configuration from profile
// choice. The daemon remains the monitor manager in both modes: automatic mode
// picks the best hardware match, while manual mode pins the exact saved profile
// currently on screen until the connected monitor set changes.
func (s *Service) SetProfileAuto(params ipc.ProfileAutoParams) error {
	s.pendingMu.Lock()
	previewActive := s.pending != nil
	s.pendingMu.Unlock()
	if previewActive {
		return errors.New("finish the active preview before changing profile selection mode")
	}

	if params.Enabled {
		s.writeMu.Lock()
		s.clearManualOverride()
		s.applied = nil
		s.writeMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.applyBest(ctx); err != nil && !errors.Is(err, errDisplaysSleeping) {
			return err
		}
		s.cfg.Logf("automatic profile selection on")
		s.signalChange()
		return nil
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	profiles, err := s.store.List()
	if err != nil {
		return err
	}
	monitors, err := s.client.Monitors(ctx)
	if err != nil {
		return err
	}
	rules, err := s.client.WorkspaceRules(ctx)
	if err != nil {
		return err
	}
	active, ok := profile.ExactStateMatch(profiles, monitors, rules)
	if !ok {
		return errors.New("save or select a profile before turning automatic selection off")
	}
	s.setManualOverride(profile.MonitorSetHash(monitors), active)
	s.applied = rememberApplied(active, monitors)
	s.lastProfile, s.lastMonitorSet = active, profile.MonitorSetHash(monitors)
	s.lastLidState = s.lidState
	s.cfg.Logf("automatic profile selection off; pinned %q for this monitor set", active.Name)
	s.signalChange()
	return nil
}

func (s *Service) Preview(owner string, params ipc.PreviewParams) (ipc.Transaction, error) {
	target, err := s.resolvePreviewProfile(params)
	if err != nil {
		return ipc.Transaction{}, err
	}
	timeout := time.Duration(params.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if timeout > 24*time.Hour {
		timeout = 24 * time.Hour
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.pendingMu.Lock()
	if s.pending != nil {
		activeOwner := s.pending.owner
		s.pendingMu.Unlock()
		return ipc.Transaction{}, fmt.Errorf("another interactive preview is active (%s)", activeOwner)
	}
	s.pendingMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	monitors, err := s.client.Monitors(ctx)
	if err != nil {
		return ipc.Transaction{}, err
	}
	effective := target
	if state, stateErr := lid.ReadState(ctx); stateErr == nil && state == lid.Closed {
		effective, _ = profile.ApplyClosedLidPolicy(target, monitors)
	}
	snapshot, err := s.engine.Apply(ctx, effective, monitors, apply.ApplyModeInteractive)
	if err != nil {
		return ipc.Transaction{}, err
	}

	id, err := transactionID()
	if err != nil {
		revertCtx, revertCancel := context.WithTimeout(context.Background(), 8*time.Second)
		_ = s.engine.Revert(revertCtx, snapshot)
		revertCancel()
		return ipc.Transaction{}, err
	}
	monitorSet := profile.MonitorSetHash(monitors)
	_, wasManual := s.manualOverride(monitorSet)
	deadline := time.Now().Add(timeout)
	pending := &pendingTransaction{
		id:           id,
		owner:        owner,
		requested:    target,
		profile:      effective,
		snapshot:     snapshot,
		deadline:     deadline,
		monitorSet:   monitorSet,
		saveOnCommit: params.SaveOnCommit,
		wasManual:    wasManual,
	}
	s.pendingMu.Lock()
	s.pending = pending
	pending.timer = time.AfterFunc(timeout, func() { s.expirePreview(id) })
	s.pendingMu.Unlock()

	s.cfg.Logf("previewing profile %q for %s", effective.Name, timeout)
	s.signalChange()
	return ipc.Transaction{ID: id, Profile: effective, Deadline: deadline}, nil
}

func (s *Service) Confirm(owner string, params ipc.TransactionParams) error {
	return s.commitPreview(owner, params.TransactionID, false)
}

func (s *Service) Commit(owner string, params ipc.CommitParams) error {
	return s.commitPreview(owner, params.TransactionID, params.Save)
}

func (s *Service) commitPreview(owner string, transactionID string, save bool) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	pending, err := s.ownedPending(owner, transactionID)
	if err != nil {
		return err
	}
	if !time.Now().Before(pending.deadline) {
		return errors.Join(ipc.ErrTransactionUnavailable, s.restorePending(pending))
	}
	if save || pending.saveOnCommit {
		if err := profileio.SaveWithSidecars(s.store, pending.requested); err != nil {
			return err
		}
	}
	s.clearPending(pending.id)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.engine.PostApply(ctx, pending.profile); err != nil {
		s.cfg.Logf("post apply for %q failed: %v", pending.profile.Name, err)
	}
	// Activating a saved profile is an explicit manual choice. Saving an edit,
	// however, must preserve the selection mode from before the preview: an
	// automatic setup stays automatic, while an already pinned profile keeps
	// its pin updated to the newly saved layout. Doing this here keeps save and
	// selection-mode changes atomic even if the panel is rebuilt mid-preview.
	if !pending.saveOnCommit || pending.wasManual {
		s.setManualOverride(pending.monitorSet, pending.profile)
	} else {
		s.clearManualOverride()
	}
	// Record what the confirmed profile left on screen, so the next automatic
	// pass recognizes the current state instead of applying it a second time.
	if monitors, err := s.client.Monitors(ctx); err != nil {
		s.applied = nil
		s.cfg.Logf("refresh monitors after confirm failed: %v", err)
	} else {
		s.applied = rememberApplied(pending.profile, monitors)
	}
	s.cfg.Logf("kept profile preview %q", pending.profile.Name)
	s.lastProfile, s.lastMonitorSet = pending.requested, pending.monitorSet
	s.lastLidState = s.lidState
	s.signalChange()
	return nil
}

func (s *Service) Revert(owner string, params ipc.TransactionParams) error {
	return s.revertOwned(owner, params.TransactionID)
}

func (s *Service) Save(params ipc.SaveParams) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := profileio.SaveWithSidecars(s.store, params.Profile); err != nil {
		return err
	}
	s.signalChange()
	return nil
}

func (s *Service) Delete(params ipc.DeleteParams) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if strings.TrimSpace(params.Name) == "" {
		return errors.New("profile name is required")
	}
	if err := s.store.Delete(params.Name); err != nil {
		return err
	}
	s.signalChange()
	return nil
}

func (s *Service) Disconnect(owner string) {
	s.pendingMu.Lock()
	pending := s.pending
	if pending == nil || pending.owner != owner {
		s.pendingMu.Unlock()
		return
	}
	// A monitor profile can rebuild Omarchy's per-screen bar components. That
	// destroys the initiating socket even though the person is still looking at
	// the changed layout. Leave the safety timer armed and make the transaction
	// reclaimable by the replacement panel instead of reverting immediately.
	pending.owner = ""
	s.pendingMu.Unlock()
	s.cfg.Logf("profile preview %q is waiting for a replacement panel", pending.profile.Name)
	s.signalChange()
}

// Shutdown restores any unconfirmed layout before the daemon exits. Ordinary
// client disconnects can be a harmless bar rebuild, but once the daemon itself
// is going away there will be neither a replacement panel nor a safety timer.
func (s *Service) Shutdown() error {
	if err := s.revertPending(""); err != nil {
		return fmt.Errorf("restore unconfirmed profile during shutdown: %w", err)
	}
	s.cfg.Logf("restored unconfirmed profile during shutdown")
	return nil
}

func (s *Service) resolvePreviewProfile(params ipc.PreviewParams) (profile.Profile, error) {
	if params.Profile != nil {
		target := *params.Profile
		target.Normalize()
		if err := target.Validate(); err != nil {
			return profile.Profile{}, err
		}
		return target, nil
	}
	if strings.TrimSpace(params.ProfileName) == "" {
		return profile.Profile{}, errors.New("preview requires profile or profile_name")
	}
	return s.store.Load(params.ProfileName)
}

func (s *Service) ownedPending(owner string, id string) (*pendingTransaction, error) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if s.pending == nil {
		return nil, ipc.ErrTransactionUnavailable
	}
	if s.pending.id != id {
		return nil, fmt.Errorf("%w: unknown transaction %q", ipc.ErrTransactionUnavailable, id)
	}
	if s.pending.owner == "" {
		s.pending.owner = owner
		s.cfg.Logf("profile preview %q reclaimed by replacement client", s.pending.profile.Name)
	}
	if s.pending.owner != owner {
		return nil, errors.New("interactive preview belongs to another client")
	}
	return s.pending, nil
}

func (s *Service) clearPending(id string) {
	s.pendingMu.Lock()
	if s.pending != nil && s.pending.id == id {
		if s.pending.timer != nil {
			s.pending.timer.Stop()
		}
		s.pending = nil
	}
	s.pendingMu.Unlock()
}

func (s *Service) revertOwned(owner string, id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	pending, err := s.ownedPending(owner, id)
	if err != nil {
		return err
	}
	return s.restorePending(pending)
}

// The safety timer and shutdown belong to the daemon, not to a connection
// which may have been replaced since the preview started.
func (s *Service) revertPending(id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.pendingMu.Lock()
	pending := s.pending
	s.pendingMu.Unlock()
	if pending == nil || (id != "" && pending.id != id) {
		return nil
	}
	return s.restorePending(pending)
}

func (s *Service) restorePending(pending *pendingTransaction) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.engine.Revert(ctx, pending.snapshot); err != nil {
		return err
	}
	s.clearPending(pending.id)
	s.signalChange()
	return nil
}

func (s *Service) expirePreview(id string) {
	if err := s.revertPending(id); err != nil {
		s.cfg.Logf("auto-revert IPC preview: %v", err)
	} else if err == nil {
		s.cfg.Logf("profile preview expired and was reverted")
	}
}

func transactionID() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate transaction id: %w", err)
	}
	return hex.EncodeToString(token[:]), nil
}
