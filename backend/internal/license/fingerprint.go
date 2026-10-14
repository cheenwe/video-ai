package license

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"strings"
)

// FingerprintVersion identifies this product line to nextlicense (VA1 = baseplatform).
const FingerprintVersion = "VA1"

// MachineInfo describes how the deployment fingerprint was derived (no secrets).
type MachineInfo struct {
	MachineID      string   `json:"machine_id"`
	ShortMachineID string   `json:"short_machine_id"`
	Version        string   `json:"version"`
	Hostname       string   `json:"hostname"`
	Platform       string   `json:"platform"`
	Sources        []string `json:"sources"`
	Stable         bool     `json:"stable"`
	Overridden     bool     `json:"overridden"`
}

const (
	BindingFloating = "floating"
	BindingBound    = "bound"
	BindingOnline   = "online"
)

func MachineID() string {
	return CollectMachineInfo().MachineID
}

func CollectMachineInfo() MachineInfo {
	if override := strings.TrimSpace(os.Getenv("BASE_MACHINE_ID")); override != "" {
		id := normalizeMachineID(override)
		return MachineInfo{
			MachineID:      id,
			ShortMachineID: ToShortMachineID(id),
			Version:        FingerprintVersion,
			Hostname:       hostname(),
			Platform:       platform(),
			Sources:        []string{"env:BASE_MACHINE_ID"},
			Stable:         true,
			Overridden:     true,
		}
	}

	host := hostname()
	sysID, sysSource, stable := platformSystemID()
	sources := []string{sysSource, "host:" + host, "platform:" + platform()}

	material := strings.Join([]string{
		FingerprintVersion,
		sysID,
		host,
		runtime.GOOS,
		runtime.GOARCH,
	}, "|")
	sum := sha256.Sum256([]byte(material))
	id := FingerprintVersion + "-" + hex.EncodeToString(sum[:16])

	return MachineInfo{
		MachineID:      id,
		ShortMachineID: ToShortMachineID(id),
		Version:        FingerprintVersion,
		Hostname:       host,
		Platform:       platform(),
		Sources:        sources,
		Stable:         stable,
		Overridden:     false,
	}
}

func normalizeMachineID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return id
	}
	upper := strings.ToUpper(id)
	if strings.HasPrefix(upper, FingerprintVersion+"-") {
		return id
	}
	return FingerprintVersion + "-M-" + id
}

func platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	return strings.TrimSpace(h)
}

func ephemeralSystemID() string {
	return "ephemeral"
}

func ResolveBindingMode(machineID, bindingMode string) string {
	bindingMode = strings.TrimSpace(bindingMode)
	if bindingMode != "" {
		return bindingMode
	}
	machineID = strings.TrimSpace(machineID)
	if machineID == "" || machineID == "offline" {
		return BindingFloating
	}
	return BindingBound
}

func ValidateMachineBinding(payload *LicensePayload) error {
	if payload == nil {
		return nil
	}
	mode := ResolveBindingMode(payload.MachineID, payload.BindingMode)
	switch mode {
	case BindingFloating:
		return nil
	case BindingBound, BindingOnline:
		want := strings.TrimSpace(payload.MachineID)
		if want == "" || want == "offline" {
			return nil
		}
		got := MachineID()
		if MachineIDsMatch(want, got) {
			return nil
		}
		return errMachineMismatch(want, got)
	}
	return nil
}

func errMachineMismatch(want, got string) error {
	return &MachineMismatchError{Expected: want, Actual: got}
}

func LegacyMachineID() string {
	host, _ := os.Hostname()
	data := host + "|" + runtime.GOOS + "|" + runtime.GOARCH
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:16])
}

func machineIDCandidates() []string {
	full := MachineID()
	short := ToShortMachineID(full)
	leg := LegacyMachineID()
	out := []string{full, short, leg}
	seen := map[string]struct{}{}
	var uniq []string
	for _, id := range out {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	return uniq
}

type MachineMismatchError struct {
	Expected string
	Actual   string
}

func (e *MachineMismatchError) Error() string {
	return "部署指纹不匹配（授权绑定 " + e.Expected + "，本机 " + e.Actual + "）"
}
