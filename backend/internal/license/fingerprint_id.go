package license

import (
	"errors"
	"regexp"
	"strings"
)

const ShortFingerprintHexLen = 8

var (
	fullVA1HexRe  = regexp.MustCompile(`^va1-[0-9a-f]{32}$`)
	shortVA1HexRe = regexp.MustCompile(`^va1-[0-9a-f]{8}$`)
	manualVA1Re   = regexp.MustCompile(`^VA1-M-.+$`)
	bareHexRe     = regexp.MustCompile(`^[0-9a-f]+$`)
)

func ToShortMachineID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, FingerprintVersion+"-M-") {
		suffix := strings.TrimPrefix(id, FingerprintVersion+"-M-")
		if len(suffix) <= ShortFingerprintHexLen {
			return id
		}
		return FingerprintVersion + "-M-" + suffix[:ShortFingerprintHexLen]
	}
	if strings.HasPrefix(id, FingerprintVersion+"-") {
		hexPart := strings.TrimPrefix(id, FingerprintVersion+"-")
		if len(hexPart) >= ShortFingerprintHexLen && bareHexRe.MatchString(hexPart[:ShortFingerprintHexLen]) {
			return FingerprintVersion + "-" + strings.ToLower(hexPart[:ShortFingerprintHexLen])
		}
	}
	return id
}

func NormalizeMachineIDInput(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("部署指纹不能为空")
	}
	lower := strings.ToLower(input)
	prefix := strings.ToLower(FingerprintVersion) + "-"
	if fullVA1HexRe.MatchString(lower) || shortVA1HexRe.MatchString(lower) || manualVA1Re.MatchString(input) {
		if strings.HasPrefix(lower, prefix) && !strings.HasPrefix(lower, prefix+"m-") {
			return FingerprintVersion + "-" + strings.TrimPrefix(lower, prefix), nil
		}
		return input, nil
	}
	if bareHexRe.MatchString(lower) {
		switch len(lower) {
		case 32:
			return FingerprintVersion + "-" + lower, nil
		case ShortFingerprintHexLen:
			return FingerprintVersion + "-" + lower, nil
		default:
			if len(lower) > ShortFingerprintHexLen && len(lower) < 32 {
				return FingerprintVersion + "-" + lower, nil
			}
		}
	}
	if !strings.Contains(input, "-") && len(input) <= 64 {
		return FingerprintVersion + "-M-" + input, nil
	}
	return "", errors.New("部署指纹格式无效，请填写短码（8 位）或完整 VA1- 指纹")
}

func MachineIDsMatch(want, got string) bool {
	want = strings.TrimSpace(want)
	got = strings.TrimSpace(got)
	if want == "" || want == "offline" {
		return true
	}
	if want == got || want == LegacyMachineID() && got == LegacyMachineID() {
		return true
	}
	if ToShortMachineID(got) == want || ToShortMachineID(want) == ToShortMachineID(got) {
		return true
	}
	if norm, err := NormalizeMachineIDInput(want); err == nil {
		if norm == got || strings.HasPrefix(got, norm) {
			return true
		}
		if ToShortMachineID(got) == norm {
			return true
		}
	}
	return false
}
