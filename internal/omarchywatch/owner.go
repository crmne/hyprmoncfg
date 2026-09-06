package omarchywatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

const (
	watcherCommand      = "omarchy-hyprland-monitor-watch"
	watcherDescription  = "omarchy-hyprland-monitor-watch"
	defaultPollInterval = time.Second
)

type systemdUnit struct {
	Unit        string `json:"unit"`
	Active      string `json:"active"`
	Description string `json:"description"`
}

// Owner prevents Omarchy's monitor watcher from changing monitor state while
// hyprmoncfgd is responsible for applying monitor profiles.
type Owner struct {
	lifecycle    sync.Mutex
	systemctl    string
	launcher     string
	watcher      string
	pollInterval time.Duration
	logf         func(format string, args ...any)
	sessionAlive func() bool

	mu        sync.Mutex
	claimed   bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	lastError string
}

// New returns a disabled owner on systems that do not provide Omarchy's
// watcher, systemctl, or uwsm-app.
func New(logf func(format string, args ...any)) *Owner {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	systemctl, _ := exec.LookPath("systemctl")
	launcher, _ := exec.LookPath("uwsm-app")
	watcher, _ := exec.LookPath(watcherCommand)

	return &Owner{
		systemctl:    systemctl,
		launcher:     launcher,
		watcher:      watcher,
		pollInterval: defaultPollInterval,
		logf:         logf,
		sessionAlive: hyprlandSessionAlive,
	}
}

// Start synchronously suppresses an existing watcher, then continues checking
// in the background. The background check covers the common startup ordering
// where hyprmoncfgd starts just before Omarchy launches its watcher.
func (o *Owner) Start(ctx context.Context) {
	o.lifecycle.Lock()
	defer o.lifecycle.Unlock()
	if !o.enabled() {
		return
	}
	if o.cancel != nil {
		return
	}

	o.recordSuppressResult(o.suppress(ctx))

	// A manage request has a short-lived context; ownership lasts until Release.
	watchCtx, cancel := context.WithCancel(context.Background())
	o.mu.Lock()
	o.cancel = cancel
	o.mu.Unlock()

	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		ticker := time.NewTicker(o.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				stopped, err := o.suppress(watchCtx)
				if watchCtx.Err() != nil {
					// Shutdown kills the systemctl child mid-flight; that is
					// not a suppression failure worth logging.
					continue
				}
				o.recordSuppressResult(stopped, err)
			}
		}
	}()
}

// Release stops background suppression and restores the Omarchy watcher when
// this owner actually stopped it and the Hyprland session is still running.
func (o *Owner) Release(ctx context.Context) error {
	o.lifecycle.Lock()
	defer o.lifecycle.Unlock()
	o.mu.Lock()
	cancel := o.cancel
	o.cancel = nil
	o.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	o.wg.Wait()

	if !o.wasClaimed() || !o.enabled() || !o.sessionAlive() {
		return nil
	}

	// Close the race with a watcher launched after the last polling tick, then
	// start exactly one replacement.
	stopped, err := o.suppress(ctx)
	if err != nil {
		return fmt.Errorf("final Omarchy monitor watcher suppression: %w", err)
	}
	if stopped {
		o.markClaimed()
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// uwsm-app becomes systemd-run --scope and then the watcher. Do not attach
	// CommandContext cancellation to it: Release's short-lived context is only
	// for setup, while the watcher must outlive hyprmoncfgd.
	cmd := exec.Command(o.launcher, "--", o.watcher)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("restart %s: %w", watcherCommand, err)
	}

	// Wait until systemd has moved the launcher into its new scope. Returning
	// before then would leave it in hyprmoncfgd's cgroup, where systemd could
	// terminate it as the daemon service finishes stopping.
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		units, err := o.activeWatcherScopes(ctx)
		if err == nil && len(units) > 0 {
			if err := cmd.Process.Release(); err != nil {
				return fmt.Errorf("release %s launcher: %w", watcherCommand, err)
			}
			o.logf("restarted Omarchy monitor watcher")
			o.mu.Lock()
			o.claimed = false
			o.mu.Unlock()
			return nil
		}

		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			return fmt.Errorf("restart %s: %w", watcherCommand, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (o *Owner) enabled() bool {
	return o.systemctl != "" && o.launcher != "" && o.watcher != ""
}

func (o *Owner) suppress(ctx context.Context) (bool, error) {
	units, err := o.activeWatcherScopes(ctx)
	if err != nil {
		return false, err
	}
	if len(units) == 0 {
		return false, nil
	}

	args := []string{"--user", "stop"}
	args = append(args, units...)
	stop := exec.CommandContext(ctx, o.systemctl, args...)
	if output, err := stop.CombinedOutput(); err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return false, fmt.Errorf("stop Omarchy monitor watcher scope: %w", err)
		}
		return false, fmt.Errorf("stop Omarchy monitor watcher scope: %w (%s)", err, detail)
	}

	return true, nil
}

func (o *Owner) activeWatcherScopes(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(
		ctx,
		o.systemctl,
		"--user",
		"list-units",
		"--all",
		"--type=scope",
		"--output=json",
		"--no-pager",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list user scopes: %w", err)
	}

	units, err := matchingUnits(out)
	if err != nil {
		return nil, err
	}
	return units, nil
}

func matchingUnits(data []byte) ([]string, error) {
	var units []systemdUnit
	if err := json.Unmarshal(data, &units); err != nil {
		return nil, fmt.Errorf("decode user scopes: %w", err)
	}

	matches := make([]string, 0, 1)
	for _, unit := range units {
		if unit.Description != watcherDescription || !strings.HasSuffix(unit.Unit, ".scope") {
			continue
		}
		if unit.Active != "active" && unit.Active != "activating" {
			continue
		}
		matches = append(matches, unit.Unit)
	}
	return matches, nil
}

func (o *Owner) recordSuppressResult(stopped bool, err error) {
	if err != nil {
		o.mu.Lock()
		message := err.Error()
		changed := message != o.lastError
		o.lastError = message
		o.mu.Unlock()
		if changed && !errors.Is(err, context.Canceled) {
			o.logf("could not suppress Omarchy monitor watcher: %v", err)
		}
		return
	}

	o.mu.Lock()
	o.lastError = ""
	alreadyClaimed := o.claimed
	if stopped {
		o.claimed = true
	}
	o.mu.Unlock()

	if stopped && !alreadyClaimed {
		o.logf("stopped Omarchy monitor watcher while hyprmoncfgd owns monitor profiles")
	}
}

func (o *Owner) markClaimed() {
	o.mu.Lock()
	o.claimed = true
	o.mu.Unlock()
}

func (o *Owner) wasClaimed() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.claimed
}

func hyprlandSessionAlive() bool {
	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	signature := strings.TrimSpace(os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"))
	if runtimeDir == "" {
		return false
	}
	if signature == "" {
		client, err := hypr.NewClient()
		if err != nil {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		signature, err = client.InstanceSignature(ctx)
		if err != nil {
			return false
		}
	}
	_, err := os.Stat(filepath.Join(runtimeDir, "hypr", signature, ".socket.sock"))
	return err == nil
}
