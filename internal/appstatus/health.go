package appstatus

import "github.com/crmne/hyprmoncfg/internal/hypr"

func monitorHealth(m hypr.Monitor) string {
	switch {
	case m.Name == "FALLBACK":
		return "synthetic"
	case m.Disabled:
		return "off"
	case !m.DPMSStatus:
		return "sleeping"
	case m.Width <= 0 || m.Height <= 0:
		return "no_signal"
	default:
		return "usable"
	}
}
