package hypr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type EventType string

const (
	EventMonitorAdded   EventType = "monitoradded"
	EventMonitorRemoved EventType = "monitorremoved"
)

type Event struct {
	Type  EventType
	Value string
	Raw   string
}

type VersionInfo struct {
	Version string `json:"version"`
	Tag     string `json:"tag"`
}

type Client struct {
	hyprctl string
}

func NewClient() (*Client, error) {
	path, err := exec.LookPath("hyprctl")
	if err != nil {
		return nil, fmt.Errorf("hyprctl not found in PATH")
	}
	return &Client{hyprctl: path}, nil
}

func (c *Client) Monitors(ctx context.Context) ([]Monitor, error) {
	cmd := exec.CommandContext(ctx, c.hyprctl, "-j", "monitors", "all")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to query monitors: %w", err)
	}
	var monitors []Monitor
	if err := json.Unmarshal(out, &monitors); err != nil {
		return nil, fmt.Errorf("failed to decode hyprctl monitors JSON: %w", err)
	}
	for i := range monitors {
		if monitors[i].MirrorOf == "none" {
			monitors[i].MirrorOf = ""
		}
	}
	return monitors, nil
}

func (c *Client) Workspaces(ctx context.Context) ([]WorkspaceState, error) {
	cmd := exec.CommandContext(ctx, c.hyprctl, "-j", "workspaces")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	var workspaces []WorkspaceState
	if err := json.Unmarshal(out, &workspaces); err != nil {
		return nil, fmt.Errorf("failed to decode hyprctl workspaces JSON: %w", err)
	}
	return workspaces, nil
}

func (c *Client) WorkspaceRules(ctx context.Context) ([]WorkspaceRule, error) {
	cmd := exec.CommandContext(ctx, c.hyprctl, "-j", "workspacerules")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to query workspace rules: %w", err)
	}
	var rules []WorkspaceRule
	if err := json.Unmarshal(out, &rules); err != nil {
		return nil, fmt.Errorf("failed to decode hyprctl workspace rules JSON: %w", err)
	}
	return rules, nil
}

func (c *Client) Version(ctx context.Context) (VersionInfo, error) {
	cmd := exec.CommandContext(ctx, c.hyprctl, "-j", "version")
	out, err := cmd.Output()
	if err != nil {
		return VersionInfo{}, fmt.Errorf("failed to query hyprctl version: %w", err)
	}
	var version VersionInfo
	if err := json.Unmarshal(out, &version); err != nil {
		return VersionInfo{}, fmt.Errorf("failed to decode hyprctl version JSON: %w", err)
	}
	return version, nil
}

func (c *Client) SupportsMonitorV2(ctx context.Context) (bool, error) {
	version, err := c.Version(ctx)
	if err != nil {
		return false, err
	}
	return versionAtLeast(firstNonEmpty(version.Version, version.Tag), 0, 50, 0), nil
}

func (c *Client) Reload(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.hyprctl, "reload")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload Hyprland: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *Client) KeywordMonitor(ctx context.Context, value string) error {
	cmd := exec.CommandContext(ctx, c.hyprctl, "keyword", "monitor", value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed applying monitor keyword %q: %w (%s)", value, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *Client) KeywordWorkspace(ctx context.Context, value string) error {
	cmd := exec.CommandContext(ctx, c.hyprctl, "keyword", "workspace", value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed applying workspace keyword %q: %w (%s)", value, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *Client) Dispatch(ctx context.Context, dispatcher string, args ...string) error {
	allArgs := append([]string{"dispatch", dispatcher}, args...)
	cmd := exec.CommandContext(ctx, c.hyprctl, allArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed dispatch %q: %w (%s)", dispatcher, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *Client) BatchKeywordMonitor(ctx context.Context, values []string) error {
	if len(values) == 0 {
		return nil
	}
	commands := make([]string, 0, len(values))
	for _, v := range values {
		commands = append(commands, "keyword monitor "+v)
	}
	return c.Batch(ctx, commands)
}

func (c *Client) BatchKeywordWorkspace(ctx context.Context, values []string) error {
	if len(values) == 0 {
		return nil
	}
	commands := make([]string, 0, len(values))
	for _, v := range values {
		commands = append(commands, "keyword workspace "+v)
	}
	return c.Batch(ctx, commands)
}

func (c *Client) Batch(ctx context.Context, commands []string) error {
	if len(commands) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, c.hyprctl, "--batch", strings.Join(commands, " ; "))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed hyprctl batch apply: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Socket2Path() (string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return "", errors.New("XDG_RUNTIME_DIR is not set")
	}
	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if sig == "" {
		return "", errors.New("HYPRLAND_INSTANCE_SIGNATURE is not set")
	}
	return filepath.Join(runtimeDir, "hypr", sig, ".socket2.sock"), nil
}

func (c *Client) SubscribeMonitorEvents(ctx context.Context) (<-chan Event, <-chan error) {
	events := make(chan Event)
	errorsCh := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errorsCh)

		socketPath, err := Socket2Path()
		if err != nil {
			errorsCh <- err
			return
		}

		dialer := net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(ctx, "unix", socketPath)
		if err != nil {
			errorsCh <- fmt.Errorf("failed to connect to hyprland socket2: %w", err)
			return
		}
		defer conn.Close()

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			event, ok := parseEvent(line)
			if !ok {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case events <- event:
			}
		}

		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			errorsCh <- err
		}
	}()

	return events, errorsCh
}

func parseEvent(line string) (Event, bool) {
	parts := strings.SplitN(line, ">>", 2)
	if len(parts) != 2 {
		return Event{}, false
	}
	typeName := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	switch {
	case strings.HasPrefix(typeName, string(EventMonitorAdded)):
		return Event{Type: EventMonitorAdded, Value: value, Raw: line}, true
	case strings.HasPrefix(typeName, string(EventMonitorRemoved)):
		return Event{Type: EventMonitorRemoved, Value: value, Raw: line}, true
	}
	return Event{}, false
}

func versionAtLeast(value string, wantMajor, wantMinor, wantPatch int) bool {
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(value, "v")), ".")
	if len(parts) == 0 {
		return false
	}
	parsed := []int{0, 0, 0}
	for idx := 0; idx < len(parsed) && idx < len(parts); idx++ {
		part := parts[idx]
		end := 0
		for end < len(part) && part[end] >= '0' && part[end] <= '9' {
			end++
		}
		if end == 0 {
			continue
		}
		n, err := strconv.Atoi(part[:end])
		if err != nil {
			continue
		}
		parsed[idx] = n
	}

	if parsed[0] != wantMajor {
		return parsed[0] > wantMajor
	}
	if parsed[1] != wantMinor {
		return parsed[1] > wantMinor
	}
	return parsed[2] >= wantPatch
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
