package daemon

import (
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) batteryState() (bool, bool) {
	if s.cfg.ReadBatteryState != nil {
		return s.cfg.ReadBatteryState()
	}
	return readBatteryState("/sys/class/power_supply")
}

// readBatteryState requires a present system battery. Peripheral batteries and
// missing or inconsistent telemetry must not turn a desktop into a laptop.
func readBatteryState(root string) (battery bool, known bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, false
	}
	hasBattery, discharging, charging, mainsKnown := false, false, false, false
	read := func(dir, name string) string {
		b, _ := os.ReadFile(filepath.Join(root, dir, name))
		return strings.TrimSpace(string(b))
	}
	for _, entry := range entries {
		name := entry.Name()
		if read(name, "scope") == "Device" {
			continue
		}
		switch read(name, "type") {
		case "Battery":
			if read(name, "present") == "0" {
				continue
			}
			hasBattery = true
			discharging = discharging || read(name, "status") == "Discharging"
			charging = charging || read(name, "status") == "Charging"
		case "Mains", "USB", "USB_C", "USB_PD":
			if read(name, "online") == "1" {
				mainsKnown, charging = true, true
			}
			if read(name, "online") == "0" {
				mainsKnown = true
			}
		}
	}
	if !hasBattery {
		return false, false
	}
	if charging {
		return false, true
	}
	if discharging || mainsKnown {
		return true, true
	}
	return false, false
}
