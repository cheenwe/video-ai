package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/nvr"
)

const fakeAgentToken = "agent-secret-token"

// fakeAgent mimics the nvr-agent HTTP API closely enough for proxy tests.
func fakeAgent(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var saved []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"version":"9.9.9","cameras":[{"id":"cam1","name":"门口","record":{"running":false}}]}`)
	})
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			saved = append(saved, string(body))
			if strings.Contains(string(body), "invalid") {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"error":"camera cam1: unknown nvr"}`)
				return
			}
			io.WriteString(w, `{"ok":true,"applied":true,"restart_required":false}`)
			return
		}
		io.WriteString(w, `{"listen":"127.0.0.1:8090","api_token":"********","nvrs":[],"cameras":[]}`)
	})
	mux.HandleFunc("/live/cam1/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		io.WriteString(w, "#EXTM3U\nseg_00001.ts\n")
	})
	mux.HandleFunc("/recordings/cam1/cam1_20261014_130000.mp4", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("download") == "1" {
			w.Header().Set("Content-Disposition", `attachment; filename="cam1_20261014_130000.mp4"`)
		}
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, "x.mp4", time.Now(), bytes.NewReader([]byte("0123456789")))
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeAgentToken || r.URL.Query().Get("token") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":"unauthorized"}`)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &saved
}

func (f *fixture) raw(method, path string, body []byte, cookie *http.Cookie, header map[string]string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Origin", "http://example.com")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.r.ServeHTTP(w, req)
	return w
}

func TestNVRAgentProxy(t *testing.T) {
	agent, saved := fakeAgent(t)
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	e, _ := f.call("POST", "/api/nvr/agents/test", map[string]any{"base_url": agent.URL, "token": "wrong"}, f.cookie, "http://example.com")
	if e.Success == 1 || e.Code != 502 || !strings.Contains(e.Msg, "Token") {
		t.Fatalf("wrong token should fail as 502, got %+v", e)
	}
	e = f.ok("POST", "/api/nvr/agents/test", map[string]any{"base_url": agent.URL + "/", "token": fakeAgentToken})
	if !strings.Contains(string(e.Data), `"camera_count":1`) {
		t.Fatalf("draft test: %s", e.Data)
	}

	e = f.ok("POST", "/api/nvr/agents", map[string]any{"name": "机房 agent", "base_url": agent.URL, "token": fakeAgentToken})
	var created struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(e.Data, &created)
	base := fmt.Sprintf("/api/nvr/agents/%d", created.ID)

	if e, _ := f.call("POST", "/api/nvr/agents", map[string]any{"name": "dup", "base_url": agent.URL}, f.cookie, "http://example.com"); e.Success == 1 {
		t.Fatal("duplicate base_url accepted")
	}
	if e, _ := f.call("POST", "/api/nvr/agents", map[string]any{"name": "bad", "base_url": "http://u:p@10.0.0.1:8090/x"}, f.cookie, "http://example.com"); e.Success == 1 {
		t.Fatal("base_url with credentials/path accepted")
	}

	list := f.ok("GET", "/api/nvr/agents", nil)
	if strings.Contains(string(list.Data), fakeAgentToken) || !strings.Contains(string(list.Data), `"token_set":true`) {
		t.Fatalf("agent list leaks token or misses token_set: %s", list.Data)
	}
	var stored models.NVRAgent
	f.db.First(&stored, created.ID)
	if stored.TokenCipher == "" || strings.Contains(stored.TokenCipher, fakeAgentToken) {
		t.Fatalf("token not encrypted at rest: %q", stored.TokenCipher)
	}

	e = f.ok("POST", base+"/test", nil)
	if !strings.Contains(string(e.Data), "9.9.9") {
		t.Fatalf("test: %s", e.Data)
	}
	f.db.First(&stored, created.ID)
	if stored.LastStatus != "online" || stored.Version != "9.9.9" {
		t.Fatalf("status not recorded: %+v", stored)
	}

	e = f.ok("GET", base+"/config", nil)
	if !strings.Contains(string(e.Data), `"listen":"127.0.0.1:8090"`) {
		t.Fatalf("config: %s", e.Data)
	}
	e = f.ok("PUT", base+"/config", map[string]any{"listen": "127.0.0.1:8090", "cameras": []any{}})
	if !strings.Contains(string(e.Data), `"applied":true`) || len(*saved) != 1 {
		t.Fatalf("config save: %s %v", e.Data, *saved)
	}
	f.ok("PUT", base+"/config", map[string]any{"listen": "127.0.0.1:8090", "api_token": "rotated-token"})
	f.db.First(&stored, created.ID)
	if tok, _ := nvr.DecryptToken("test-only-secret-with-at-least-32-bytes", stored.TokenCipher); tok != "rotated-token" {
		t.Fatalf("platform token not synced after agent token change: %q", tok)
	}
	f.ok("PUT", base, map[string]any{"token": fakeAgentToken})
	e, w := f.call("PUT", base+"/config", map[string]any{"listen": "invalid"}, f.cookie, "http://example.com")
	if w.Code != 422 || !strings.Contains(e.Msg, "unknown nvr") {
		t.Fatalf("agent validation error not surfaced: %d %+v", w.Code, e)
	}

	e = f.ok("GET", "/api/nvr/cameras", nil)
	if !strings.Contains(string(e.Data), `"online":true`) || !strings.Contains(string(e.Data), `"cam1"`) {
		t.Fatalf("cameras: %s", e.Data)
	}

	r := f.raw("GET", base+"/live/cam1/index.m3u8", nil, f.cookie, nil)
	if r.Code != 200 || !strings.Contains(r.Body.String(), "seg_00001.ts") || r.Header().Get("Content-Type") != "application/vnd.apple.mpegurl" {
		t.Fatalf("live playlist: %d %q %v", r.Code, r.Body.String(), r.Header())
	}
	r = f.raw("GET", base+"/recordings/cam1/cam1_20261014_130000.mp4", nil, f.cookie, map[string]string{"Range": "bytes=2-4"})
	if r.Code != http.StatusPartialContent || r.Body.String() != "234" || r.Header().Get("Content-Range") == "" {
		t.Fatalf("range proxy: %d %q %v", r.Code, r.Body.String(), r.Header())
	}
	r = f.raw("GET", base+"/recordings/cam1/cam1_20261014_130000.mp4?download=1", nil, f.cookie, nil)
	if r.Code != 200 || !strings.Contains(r.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %d %v", r.Code, r.Header())
	}
	for _, p := range []string{
		base + "/recordings/cam1/..%2Fsecret.mp4",
		base + "/recordings/cam1/nvr-agent.json",
		base + "/recordings/..%2F..%2Fapi/x.mp4",
		base + "/live/cam1/..%2F..%2Fapi%2Fconfig",
		base + "/live/cam1/other.m3u8",
	} {
		if r := f.raw("GET", p, nil, f.cookie, nil); r.Code == 200 {
			t.Errorf("%s should be rejected", p)
		}
	}

	var audits int64
	f.db.Model(&models.SysLog{}).Where("action LIKE ?", "nvr.%").Count(&audits)
	if audits < 4 {
		t.Fatalf("expected nvr audit logs, got %d", audits)
	}

	f.ok("POST", "/api/users", map[string]any{"username": "viewer", "email": "viewer@example.test", "password": "Viewer12345"})
	viewer := f.login("viewer", "Viewer12345")
	if r := f.raw("GET", base+"/live/cam1/index.m3u8", nil, viewer, nil); r.Code != http.StatusForbidden {
		t.Fatalf("user without camera:live got %d", r.Code)
	}
	if e, w := f.call("GET", "/api/nvr/cameras", nil, viewer, ""); w.Code != http.StatusForbidden || e.Success == 1 {
		t.Fatalf("user without camera:list got %d", w.Code)
	}

	f.ok("PUT", base, map[string]any{"enabled": 0})
	if e, w := f.call("GET", base+"/config", nil, f.cookie, ""); w.Code != http.StatusConflict || e.Success == 1 {
		t.Fatalf("disabled agent still proxied: %d", w.Code)
	}
	f.ok("DELETE", base, nil)
}

func TestNVRAgentUnreachable(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")
	e, w := f.call("POST", "/api/nvr/agents/test", map[string]any{"base_url": "http://127.0.0.1:1"}, f.cookie, "http://example.com")
	if w.Code != http.StatusBadGateway || !strings.Contains(e.Msg, "nvr-agent") {
		t.Fatalf("unreachable: %d %+v", w.Code, e)
	}
}
