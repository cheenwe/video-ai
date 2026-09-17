package license

import (
	"strings"

	"github.com/shirou/gopsutil/v3/host"
)

// platformSystemID returns a stable OS-level identifier when available.
func platformSystemID() (id string, source string, stable bool) {
	if info, err := host.Info(); err == nil {
		id = strings.TrimSpace(info.HostID)
		if isValidSystemID(id) {
			return id, "os:host-id", true
		}
	}
	return platformSystemIDFallback()
}

func isValidSystemID(id string) bool {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		return false
	}
	switch id {
	case "unknown", "none", "n/a", "na", "00000000-0000-0000-0000-000000000000":
		return false
	}
	return true
}
