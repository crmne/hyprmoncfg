package hypr

import "testing"

func TestParseEventAcceptsMonitorV2Events(t *testing.T) {
	tests := []struct {
		line string
		want EventType
	}{
		{
			line: "monitoraddedv2>>3,DP-1,Dell U2720Q",
			want: EventMonitorAdded,
		},
		{
			line: "monitorremovedv2>>3,DP-1,Dell U2720Q",
			want: EventMonitorRemoved,
		},
	}

	for _, tt := range tests {
		event, ok := parseEvent(tt.line)
		if !ok {
			t.Fatalf("expected %q to be parsed", tt.line)
		}
		if event.Type != tt.want {
			t.Fatalf("expected %q to map to %q, got %q", tt.line, tt.want, event.Type)
		}
		if event.Raw != tt.line {
			t.Fatalf("expected raw line to be preserved, got %q", event.Raw)
		}
	}
}
