//go:build !linux && !darwin

package license

func platformSystemIDFallback() (id string, source string, stable bool) {
	return ephemeralSystemID(), "fallback:ephemeral", false
}
