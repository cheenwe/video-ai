package handlers

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestDecodeAccessConfigDefaults(t *testing.T) {
	cfg := decodeAccessConfig(`{"host":"10.0.0.5","device_id":"34020000001320000001"}`)
	if cfg.Host != "10.0.0.5" || cfg.DeviceID != "34020000001320000001" {
		t.Fatalf("config not decoded: %+v", cfg)
	}
	if cfg.Port != 5060 || cfg.Transport != "udp" {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	// Garbage input must not panic.
	if got := decodeAccessConfig("not json"); got.Port != 5060 {
		t.Fatalf("bad json should still default: %+v", got)
	}
}

func TestSIPOptionsRequestShape(t *testing.T) {
	req := sipOptions("10.1.1.1:5060", "34020000001320000001", "10.0.0.5")
	for _, want := range []string{
		"OPTIONS sip:34020000001320000001@10.0.0.5 SIP/2.0",
		"Via: SIP/2.0/UDP 10.1.1.1:5060;branch=z9hG4bK",
		"CSeq: 1 OPTIONS",
		"Content-Length: 0",
	} {
		if !strings.Contains(req, want) {
			t.Fatalf("request missing %q:\n%s", want, req)
		}
	}
	if !strings.HasSuffix(req, "\r\n\r\n") {
		t.Fatal("SIP request must end with a blank line")
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("SIP/2.0 200 OK\r\nVia: x\r\n\r\n"); got != "SIP/2.0 200 OK" {
		t.Fatalf("got %q", got)
	}
	if got := firstLine("no newline at all"); got != "no newline at all" {
		t.Fatalf("got %q", got)
	}
}

// TestProbeSIPAgainstResponder proves the probe reports a real answer when a
// device actually replies, instead of always claiming pending_adapter.
func TestProbeSIPAgainstResponder(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	_, port, _ := net.SplitHostPort(pc.LocalAddr().String())
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}

	go func() {
		buf := make([]byte, 2048)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		if !strings.Contains(string(buf[:n]), "OPTIONS") {
			return
		}
		_, _ = pc.WriteTo([]byte("SIP/2.0 200 OK\r\nCSeq: 1 OPTIONS\r\nContent-Length: 0\r\n\r\n"), addr)
	}()

	line, err := probeSIP(accessConfig{Host: "127.0.0.1", Port: p, DeviceID: "dev-1", Transport: "udp"}, 2*time.Second)
	if err != nil {
		t.Fatalf("expected a response: %v", err)
	}
	if !strings.HasPrefix(line, "SIP/2.0 200") {
		t.Fatalf("unexpected status line: %q", line)
	}
}

// TestProbeSIPUnreachable proves silence or refusal is reported as failure.
func TestProbeSIPUnreachable(t *testing.T) {
	// Port 1 is closed on any normal host, so the probe must not succeed.
	if _, err := probeSIP(accessConfig{Host: "127.0.0.1", Port: 1, DeviceID: "dev-1", Transport: "udp"}, 2*time.Second); err == nil {
		t.Fatal("expected failure against a closed port")
	}
}
