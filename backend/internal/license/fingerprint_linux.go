//go:build linux

package license

import (
	"os"
	"strings"
)

func platformSystemIDFallback() (id string, source string, stable bool) {
	paths := []struct {
		path   string
		source string
	}{
		{"/etc/machine-id", "linux:machine-id"},
		{"/var/lib/dbus/machine-id", "linux:dbus-machine-id"},
		{"/sys/class/dmi/id/product_uuid", "linux:dmi-product-uuid"},
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p.path)
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(raw))
		if isValidSystemID(v) {
			return v, p.source, true
		}
	}
	return ephemeralSystemID(), "linux:fallback:ephemeral", false
}
