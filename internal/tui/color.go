package tui

// Labels are presentation only; pickers and saved profiles keep Hyprland's
// wire values. Keep these terms aligned with the Omarchy panel's inspector.
func fieldOptionLabel(field int, value string) string {
	switch field {
	case 3:
		return value + "-bit"
	case 4:
		switch value {
		case "srgb":
			return "sRGB (SDR)"
		case "auto":
			return "Automatic"
		case "wide":
			return "BT.2020 (SDR)"
		case "hdr":
			return "BT.2020 + PQ (HDR)"
		case "hdredid":
			return "EDID primaries + PQ"
		case "dcip3":
			return "DCI-P3"
		case "dp3":
			return "Display P3"
		case "adobe":
			return "Adobe RGB"
		case "edid":
			return "EDID primaries (SDR)"
		}
	case 14:
		switch value {
		case "default":
			return "Default"
		case "gamma22":
			return "Gamma 2.2"
		case "srgb":
			return "sRGB"
		}
	case 18, 19:
		switch value {
		case "off":
			return "Force off"
		case "auto":
			return "Auto-detect"
		case "on":
			return "Force on"
		}
	}
	return value
}

func sdrMultiplier(value float64) float64 {
	if value == 0 {
		return 1
	}
	return value
}
