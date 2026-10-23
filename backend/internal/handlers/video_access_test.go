package handlers

import (
	"encoding/json"
	"testing"
)

// TestAccessReqAcceptsBothConfigAliases locks the read/write symmetry of the
// access-device config: the model serializes it as config_json, so the write
// path must accept that name too (it used to accept only "config", which made
// read-modify-write clients silently drop the whole config).
func TestAccessReqAcceptsBothConfigAliases(t *testing.T) {
	var a accessReq
	if err := json.Unmarshal([]byte(`{"kind":"gb28181","code":"c","name":"n","config":{"host":"1.2.3.4"}}`), &a); err != nil {
		t.Fatal(err)
	}
	if got := string(a.effectiveConfig()); got != `{"host":"1.2.3.4"}` {
		t.Fatalf("config alias not accepted: %s", got)
	}

	var b accessReq
	if err := json.Unmarshal([]byte(`{"kind":"gb28181","code":"c","name":"n","config_json":{"host":"1.2.3.4"}}`), &b); err != nil {
		t.Fatal(err)
	}
	if got := string(b.effectiveConfig()); got != `{"host":"1.2.3.4"}` {
		t.Fatalf("config_json alias not accepted: %s", got)
	}

	// When both are present the canonical field wins.
	var both accessReq
	if err := json.Unmarshal([]byte(`{"config":{"a":1},"config_json":{"b":2}}`), &both); err != nil {
		t.Fatal(err)
	}
	if got := string(both.effectiveConfig()); got != `{"a":1}` {
		t.Fatalf("precedence wrong: %s", got)
	}

	// The stored form is text, so clients may round-trip config_json as a
	// quoted string; that must not double-encode and break probing.
	var strForm accessReq
	if err := json.Unmarshal([]byte(`{"kind":"gb28181","code":"c","name":"n","config_json":"{\"host\":\"1.2.3.4\"}"}`), &strForm); err != nil {
		t.Fatal(err)
	}
	if got := string(strForm.effectiveConfig()); got != `{"host":"1.2.3.4"}` {
		t.Fatalf("string form not normalized: %s", got)
	}
	if cfg := decodeAccessConfig(string(strForm.effectiveConfig())); cfg.Host != "1.2.3.4" {
		t.Fatalf("normalized string did not decode: %+v", cfg)
	}

	// An empty string must not become a stored value.
	var empty accessReq
	if err := json.Unmarshal([]byte(`{"config_json":""}`), &empty); err != nil {
		t.Fatal(err)
	}
	if got := empty.effectiveConfig(); len(got) != 0 {
		t.Fatalf("empty config should normalize away: %s", got)
	}

	// The stored string must decode into probe settings.
	cfg := decodeAccessConfig(`{"host":"10.0.0.9","port":5060,"device_id":"dev-1","transport":"tcp"}`)
	if cfg.Host != "10.0.0.9" || cfg.Port != 5060 || cfg.DeviceID != "dev-1" || cfg.Transport != "tcp" {
		t.Fatalf("decodeAccessConfig: %+v", cfg)
	}
}
