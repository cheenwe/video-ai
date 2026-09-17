//go:build darwin

package license

import (
	"os/exec"
	"regexp"
	"strings"
)

var darwinUUIDRe = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)

func platformSystemIDFallback() (id string, source string, stable bool) {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").CombinedOutput()
	if err == nil {
		if m := darwinUUIDRe.FindSubmatch(out); len(m) > 1 {
			uuid := strings.TrimSpace(string(m[1]))
			if isValidSystemID(uuid) {
				return uuid, "darwin:platform-uuid", true
			}
		}
	}
	return ephemeralSystemID(), "darwin:fallback:ephemeral", false
}
