package license

import "testing"

func TestCanonicalEdition(t *testing.T) {
	cases := map[string]string{
		"":           EditionCommunity,
		"community":  EditionCommunity,
		"team":       EditionTeam,
		"enterprise": EditionEnterprise,
		"flagship":   EditionEnterprise,
		"FLAGSHIP":   EditionEnterprise,
	}
	for in, want := range cases {
		if got := CanonicalEdition(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestNormalizePayloadEnterpriseFromNextlicense(t *testing.T) {
	p := &LicensePayload{Edition: "enterprise", MaxUsers: 0}
	NormalizePayload(p)
	if p.Edition != EditionEnterprise {
		t.Fatalf("edition=%q", p.Edition)
	}
	if len(p.Features) == 0 {
		t.Fatal("expected default enterprise features")
	}
	if !IsCommercialEdition(p.Edition) {
		t.Fatal("expected commercial")
	}
}

func TestNormalizeMachineIDAcceptsNF1AndVA1(t *testing.T) {
	va1, err := NormalizeMachineIDInput("VA1-411040032dcc8e567b643f7a80d64176")
	if err != nil || !stringsHasPrefix(va1, "VA1-") {
		t.Fatalf("va1: %q %v", va1, err)
	}
	bare, err := NormalizeMachineIDInput("41104003")
	if err != nil || bare != "VA1-41104003" {
		t.Fatalf("bare: %q %v", bare, err)
	}
}

func stringsHasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}

func TestEditionLabel(t *testing.T) {
	if EditionLabel("enterprise") != "旗舰版" {
		t.Fatal(EditionLabel("enterprise"))
	}
	if EditionLabel("team") != "团队版" {
		t.Fatal(EditionLabel("team"))
	}
	if EditionLabel("flagship") != "旗舰版" {
		t.Fatal(EditionLabel("flagship"))
	}
}
