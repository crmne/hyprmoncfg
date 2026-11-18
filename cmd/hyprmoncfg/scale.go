package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

// scaleSteps is the ladder Omarchy's Super + / steps through, so the keys
// behave the same when hyprmoncfg takes them over.
var scaleSteps = []float64{1, 1.25, 1.6, 2, 3, 4}

// scaleClient is the subset of the daemon client the scale command needs.
type scaleClient interface {
	EditorState(ctx context.Context) (appstatus.EditorDocument, error)
	EditProfile(ctx context.Context, params ipc.EditParams) (appstatus.EditorDraft, error)
	Preview(ctx context.Context, params ipc.PreviewParams) (ipc.Transaction, error)
	Commit(ctx context.Context, transactionID string, save bool) error
	Confirm(ctx context.Context, transactionID string) error
}

func newScaleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scale [up|down|<scale>]",
		Short: "Change the focused display's scale in the active profile",
		Long: "Steps the focused display through 1x, 1.25x, 1.6x, 2x, 3x and 4x, as Omarchy's\n" +
			"Super + / does, or sets the scale given, snapped to one that is sharp on the display.\n" +
			"The change applies at once and is saved to the profile in use, so hyprmoncfg keeps it.\n" +
			"On Omarchy, hyprmoncfg binds Super + / and Super + Alt + / to this. Needs hyprmoncfgd.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()
			path, err := ipc.SocketPath()
			if err != nil {
				return err
			}
			client, err := ipc.Dial(ctx, path)
			if err != nil {
				return errors.New("changing the scale needs hyprmoncfgd: start it with systemctl --user start hyprmoncfgd.service")
			}
			defer client.Close()
			scale, saved, err := runScale(ctx, client, args[0])
			if err != nil {
				return err
			}
			if saved != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Scale %sx, saved to %q\n", scaling.Format(scale), saved)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Scale %sx\n", scaling.Format(scale))
			}
			return nil
		},
	}
}

// nextScale is the scale a step up or down from current lands on, or the
// requested scale, each snapped to a sharp scale for the mode. It reports
// false when there is no step that way.
func nextScale(width, height int, current float64, request string) (float64, bool) {
	const epsilon = 0.005
	switch request {
	case "up":
		for _, step := range scaleSteps {
			if clean, ok := scaling.CleanScale(width, height, step); ok && clean > current+epsilon {
				return clean, true
			}
		}
		return 0, false
	case "down":
		for i := len(scaleSteps) - 1; i >= 0; i-- {
			if clean, ok := scaling.CleanScale(width, height, scaleSteps[i]); ok && clean < current-epsilon {
				return clean, true
			}
		}
		return 0, false
	default:
		value, err := strconv.ParseFloat(request, 64)
		if err != nil || value <= 0 {
			return 0, false
		}
		return scaling.CleanScale(width, height, value)
	}
}

// runScale changes the focused display's scale through the daemon and saves
// it to the profile the live layout comes from. A layout no profile matches
// is only applied. It returns the new scale and the profile saved, if any.
func runScale(ctx context.Context, client scaleClient, request string) (float64, string, error) {
	document, err := client.EditorState(ctx)
	if err != nil {
		return 0, "", err
	}
	key := ""
	for _, display := range document.Displays {
		if display.Focused {
			key = display.Key
			break
		}
	}
	output, ok := document.Profile.OutputByKey(key)
	if key == "" || !ok || !output.Enabled {
		return 0, "", errors.New("no focused display to scale")
	}
	scale, ok := nextScale(output.Width, output.Height, output.Scale, request)
	if !ok {
		if request == "up" || request == "down" {
			return output.Scale, "", nil
		}
		return 0, "", fmt.Errorf("scale must be up, down or a positive number, got %q", request)
	}

	draft, err := client.EditProfile(ctx, ipc.EditParams{
		Profile: document.Profile,
		Edit:    profile.EditorEdit{OutputKey: key, Scale: &scale},
	})
	if err != nil {
		return 0, "", err
	}
	target := draft.Profile
	saved := document.SourceProfile
	target.Name = saved
	if saved == "" {
		// A preview needs a name. This layout is only applied, never saved.
		target.Name = "Unsaved layout"
	}
	transaction, err := client.Preview(ctx, ipc.PreviewParams{Profile: &target, SaveOnCommit: saved != ""})
	if err != nil {
		return 0, "", err
	}
	if saved != "" {
		return scale, saved, client.Commit(ctx, transaction.ID, true)
	}
	return scale, "", client.Confirm(ctx, transaction.ID)
}
