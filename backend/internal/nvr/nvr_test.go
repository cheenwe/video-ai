package nvr

import "testing"

func TestTokenRoundTrip(t *testing.T) {
	c, err := EncryptToken("k1", "tok")
	if err != nil || c == "" || c == "tok" {
		t.Fatalf("encrypt: %q %v", c, err)
	}
	if p, err := DecryptToken("k1", c); err != nil || p != "tok" {
		t.Fatalf("decrypt: %q %v", p, err)
	}
	if _, err := DecryptToken("k2", c); err == nil {
		t.Fatal("decrypt with another key should fail")
	}
	if c, _ := EncryptToken("k1", ""); c != "" {
		t.Fatal("empty token should stay empty")
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	good := map[string]string{
		"http://127.0.0.1:8090":       "http://127.0.0.1:8090",
		" http://192.168.1.20:8090/ ": "http://192.168.1.20:8090",
		"https://nvr.local":           "https://nvr.local",
	}
	for in, want := range good {
		if got, err := NormalizeBaseURL(in); err != nil || got != want {
			t.Errorf("%q => %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "127.0.0.1:8090", "ftp://x", "http://u:p@h:1", "http://h:1/api", "http://h:1?x=1", "http://:8090"} {
		if _, err := NormalizeBaseURL(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestValidNames(t *testing.T) {
	for _, ok := range []string{"cam1", "nvr1_ch3", "a-b"} {
		if !ValidCameraID(ok) {
			t.Errorf("camera %q rejected", ok)
		}
	}
	for _, bad := range []string{"", "../x", "a/b", "a.b"} {
		if ValidCameraID(bad) {
			t.Errorf("camera %q accepted", bad)
		}
	}
	for _, bad := range []string{"..", "../a.mp4", "a/b.mp4", ".hidden.mp4", "a..b.mp4"} {
		if ValidFileName(bad) {
			t.Errorf("file %q accepted", bad)
		}
	}
	if !ValidFileName("cam1_20261014_130000.mp4") || !ValidFileName("seg_00012.ts") {
		t.Error("valid media names rejected")
	}
}
