package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
)

// accessConfig is the subset of device config the probe understands.
type accessConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	DeviceID  string `json:"device_id"`
	Transport string `json:"transport"` // udp | tcp
	URL       string `json:"url"`       // push / flv / hls ingress address
}

func decodeAccessConfig(raw string) accessConfig {
	var cfg accessConfig
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if cfg.Port == 0 {
		cfg.Port = 5060
	}
	if cfg.Transport == "" {
		cfg.Transport = "udp"
	}
	return cfg
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sipOptions builds a minimal SIP OPTIONS request. A conforming GB28181
// device answers with a status line; silence means unreachable.
func sipOptions(localAddr, deviceID, host string) string {
	return fmt.Sprintf("OPTIONS sip:%s@%s SIP/2.0\r\n"+
		"Via: SIP/2.0/UDP %s;branch=z9hG4bK%s;rport\r\n"+
		"From: <sip:platform@%s>;tag=%s\r\n"+
		"To: <sip:%s@%s>\r\n"+
		"Call-ID: %s@baseplatform\r\n"+
		"CSeq: 1 OPTIONS\r\n"+
		"Max-Forwards: 70\r\n"+
		"User-Agent: baseplatform\r\n"+
		"Content-Length: 0\r\n\r\n",
		deviceID, host, localAddr, randHex(8), localAddr, randHex(8),
		deviceID, host, randHex(12))
}

// probeSIP sends SIP OPTIONS and returns the status line it got back.
func probeSIP(cfg accessConfig, timeout time.Duration) (string, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	switch strings.ToLower(cfg.Transport) {
	case "tcp":
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return "", err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(timeout))
		if _, err := conn.Write([]byte(sipOptions(conn.LocalAddr().String(), cfg.DeviceID, cfg.Host))); err != nil {
			return "", err
		}
		buf := make([]byte, 2048)
		n, err := conn.Read(buf)
		if err != nil {
			return "", err
		}
		return firstLine(string(buf[:n])), nil
	default:
		conn, err := net.DialTimeout("udp", addr, timeout)
		if err != nil {
			return "", err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(timeout))
		if _, err := conn.Write([]byte(sipOptions(conn.LocalAddr().String(), cfg.DeviceID, cfg.Host))); err != nil {
			return "", err
		}
		buf := make([]byte, 2048)
		n, err := conn.Read(buf)
		if err != nil {
			return "", err
		}
		return firstLine(string(buf[:n])), nil
	}
}

func firstLine(s string) string {
	if i := strings.Index(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	if len(s) > 120 {
		return s[:120]
	}
	return s
}

// probeHTTP checks ingress endpoints used by push / FLV / HLS devices.
func probeHTTP(raw string, timeout time.Duration) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("地址无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("仅支持 http/https 地址")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	return fmt.Sprintf("HTTP %d", res.StatusCode), nil
}

// Probe attempts a real reachability check for the device. GB28181 devices get
// a SIP OPTIONS exchange; push-style devices get an HTTP request. Only when
// there is nothing to talk to does it fall back to pending_adapter, and it
// never claims success it did not observe.
func (h *AccessHandler) Probe(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var d models.AccessDevice
	if h.DB.First(&d, id).Error != nil {
		response.BizFail(c, "设备不存在", 404)
		return
	}
	cfg := decodeAccessConfig(d.ConfigJSON)

	const timeout = 4 * time.Second
	status := "unreachable"
	ok := false
	detail := ""
	var next []string

	switch d.Kind {
	case "gb28181":
		if cfg.Host == "" || cfg.DeviceID == "" {
			status, detail = "pending_adapter", "缺少 host 或 device_id，无法发起 SIP 信令"
			next = []string{"在配置中填写 host/port/device_id", "确认设备侧已指向本平台 SIP 端口"}
			break
		}
		line, err := probeSIP(cfg, timeout)
		if err != nil {
			status, detail = "unreachable", "SIP OPTIONS 无响应："+err.Error()
			next = []string{"确认设备 IP 与端口可达", "确认传输方式（UDP/TCP）与设备一致", "确认设备已注册到本平台"}
		} else {
			status, ok, detail = "reachable", true, "SIP 响应："+line
			next = []string{"配置目录订阅与媒体邀请", "绑定平台摄像头通道"}
		}
	case "rtmp_push", "http_flv", "hls_in":
		if cfg.URL == "" {
			status, detail = "pending_adapter", "缺少 url，无法检查接入地址"
			next = []string{"在配置中填写 url"}
			break
		}
		line, err := probeHTTP(cfg.URL, timeout)
		if err != nil {
			status, detail = "unreachable", "接入地址不可达："+err.Error()
			next = []string{"确认推流/拉流服务已启动", "确认地址与鉴权参数"}
		} else {
			status, ok, detail = "reachable", true, line
			next = []string{"绑定平台摄像头通道"}
		}
	default:
		status, detail = "pending_adapter", "未知接入类型"
	}

	if ok {
		_ = h.DB.Model(&d).Updates(map[string]any{"status": status, "last_error": ""}).Error
	} else {
		_ = h.DB.Model(&d).Updates(map[string]any{"status": status, "last_error": detail}).Error
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", map[bool]string{true: "success", false: "failure"}[ok],
		"access.probe", &u, fmt.Sprintf("device=%d %s", d.ID, detail))

	response.OK(c, gin.H{
		"id": d.ID, "kind": d.Kind, "status": status, "ok": ok,
		"message": detail, "next_steps": next,
	}, "")
}

// SyncCamera creates or updates the platform Camera represented by a device,
// so a successfully probed device becomes usable everywhere else.
func (h *AccessHandler) SyncCamera(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var d models.AccessDevice
	if h.DB.First(&d, id).Error != nil {
		response.BizFail(c, "设备不存在", 404)
		return
	}
	if d.Status != "reachable" {
		response.BizFail(c, "请先探测成功再同步（当前状态："+d.Status+"）", 409)
		return
	}
	cfg := decodeAccessConfig(d.ConfigJSON)

	accessType := d.Kind
	if accessType == "rtmp_push" || accessType == "http_flv" || accessType == "hls_in" {
		accessType = "push"
	}

	var cam models.Camera
	created := false
	if d.CameraID != nil {
		if err := h.DB.First(&cam, *d.CameraID).Error; err != nil {
			response.BizFail(c, "已绑定的摄像头不存在", 409)
			return
		}
	} else {
		var existing models.Camera
		if err := h.DB.Where("code = ?", d.Code).First(&existing).Error; err == nil {
			cam = existing
		} else {
			cam = models.Camera{
				Code: d.Code, Name: d.Name, AccessType: accessType,
				DeviceStatus: "registered", MediaStatus: "unknown", AnalysisStatus: "idle",
				Enabled: 1,
			}
			if err := h.DB.Create(&cam).Error; err != nil {
				response.BizFail(c, "创建摄像头失败："+err.Error(), 400)
				return
			}
			created = true
		}
	}

	// Refresh the mutable bits so a re-sync picks up config changes.
	_ = h.DB.Model(&cam).Updates(map[string]any{
		"name":        d.Name,
		"access_type": accessType,
		"archived":    0,
	}).Error

	// Keep one main stream profile pointing at the device.
	if cfg.URL != "" {
		var sp models.StreamProfile
		if err := h.DB.Where("camera_id = ? AND role = ?", cam.ID, "main").First(&sp).Error; err == nil {
			_ = h.DB.Model(&sp).Update("url", cfg.URL).Error
		} else {
			_ = h.DB.Create(&models.StreamProfile{
				CameraID: cam.ID, Role: "main", URL: cfg.URL,
				Protocol: "http", Revision: 1, Enabled: 1,
			}).Error
		}
	}

	_ = h.DB.Model(&d).Update("camera_id", cam.ID).Error
	_ = h.DB.First(&cam, cam.ID)

	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "access.sync_camera", &u,
		fmt.Sprintf("device=%d camera=%d created=%v", d.ID, cam.ID, created))

	response.OK(c, gin.H{"camera": cam, "created": created}, "已同步为平台摄像头")
}
