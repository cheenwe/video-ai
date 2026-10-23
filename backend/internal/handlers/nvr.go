package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"baseplatform/internal/config"
	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/nvr"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NVRHandler 管理 nvr-agent 节点，并以平台会话 + RBAC 代理其全部功能。
// 浏览器从不直接访问 agent，agent Token 只保存在服务端。
type NVRHandler struct {
	DB   *gorm.DB
	Cfg  *config.Config
	RBAC *services.RBACService
}

const maxAgentConfigBody = 2 << 20

func nvrAgentView(a models.NVRAgent) gin.H {
	return gin.H{
		"id": a.ID, "name": a.Name, "base_url": a.BaseURL, "token_set": a.TokenCipher != "",
		"enabled": a.Enabled, "remark": a.Remark, "last_status": a.LastStatus, "last_error": a.LastError,
		"last_seen_at": a.LastSeenAt, "version": a.Version, "created_at": a.CreatedAt, "updated_at": a.UpdatedAt,
	}
}

func (h *NVRHandler) List(c *gin.Context) {
	var items []models.NVRAgent
	if err := h.DB.Order("id asc").Find(&items).Error; err != nil {
		response.BizFail(c, "查询失败", 500)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, a := range items {
		out = append(out, nvrAgentView(a))
	}
	response.OK(c, gin.H{"items": out}, "")
}

type nvrAgentRequest struct {
	Name    *string `json:"name"`
	BaseURL *string `json:"base_url"`
	Token   *string `json:"token"`
	Enabled *int    `json:"enabled"`
	Remark  *string `json:"remark"`
}

func (h *NVRHandler) Create(c *gin.Context) {
	var req nvrAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == nil || req.BaseURL == nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	a := models.NVRAgent{Enabled: 1, LastStatus: "unknown"}
	if msg := h.applyAgentRequest(&a, req); msg != "" {
		response.BizFail(c, msg, 422)
		return
	}
	if err := h.DB.Create(&a).Error; err != nil {
		response.BizFail(c, "创建失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "nvr.agent.create", &me, a.Name+" "+a.BaseURL)
	response.OK(c, nvrAgentView(a), "创建成功")
}

func (h *NVRHandler) Update(c *gin.Context) {
	a, ok := h.loadAgent(c)
	if !ok {
		return
	}
	var req nvrAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if msg := h.applyAgentRequest(&a, req); msg != "" {
		response.BizFail(c, msg, 422)
		return
	}
	if err := h.DB.Save(&a).Error; err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "nvr.agent.update", &me, a.Name+" "+a.BaseURL)
	response.OK(c, nvrAgentView(a), "保存成功")
}

func (h *NVRHandler) Delete(c *gin.Context) {
	a, ok := h.loadAgent(c)
	if !ok {
		return
	}
	if err := h.DB.Delete(&models.NVRAgent{}, a.ID).Error; err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "nvr.agent.delete", &me, a.Name+" "+a.BaseURL)
	response.OK(c, nil, "已删除")
}

// applyAgentRequest validates and copies non-nil fields; it returns a user-facing error.
func (h *NVRHandler) applyAgentRequest(a *models.NVRAgent, req nvrAgentRequest) string {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len([]rune(name)) > 64 {
			return "名称不能为空且不超过 64 个字符"
		}
		a.Name = name
	}
	if req.BaseURL != nil {
		base, err := nvr.NormalizeBaseURL(*req.BaseURL)
		if err != nil {
			return err.Error()
		}
		var count int64
		h.DB.Model(&models.NVRAgent{}).Where("base_url = ? AND id <> ?", base, a.ID).Count(&count)
		if count > 0 {
			return "该地址的 nvr-agent 已登记"
		}
		a.BaseURL = base
	}
	if req.Token != nil {
		token := strings.TrimSpace(*req.Token)
		if len(token) > 256 {
			return "Token 过长"
		}
		cipher, err := nvr.EncryptToken(h.Cfg.SecretKey, token)
		if err != nil {
			return "Token 加密失败"
		}
		a.TokenCipher = cipher
	}
	if req.Enabled != nil {
		if *req.Enabled != 0 && *req.Enabled != 1 {
			return "启用状态无效"
		}
		a.Enabled = *req.Enabled
	}
	if req.Remark != nil {
		remark := strings.TrimSpace(*req.Remark)
		if len([]rune(remark)) > 200 {
			return "备注不超过 200 个字符"
		}
		a.Remark = remark
	}
	return ""
}

// TestDraft 测试尚未保存（或正在编辑）的 agent 地址与 Token。
// token 为 null 时沿用已保存的 Token。
func (h *NVRHandler) TestDraft(c *gin.Context) {
	var req struct {
		ID      uint    `json:"id"`
		BaseURL string  `json:"base_url"`
		Token   *string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	base, err := nvr.NormalizeBaseURL(req.BaseURL)
	if err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	token := ""
	if req.Token != nil {
		token = strings.TrimSpace(*req.Token)
	} else if req.ID > 0 {
		var a models.NVRAgent
		if err := h.DB.First(&a, req.ID).Error; err == nil {
			if token, err = nvr.DecryptToken(h.Cfg.SecretKey, a.TokenCipher); err != nil {
				response.BizFail(c, err.Error(), 422)
				return
			}
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	data, err := nvr.New(base, token).JSON(ctx, http.MethodGet, "/api/status", nil, nil)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.OK(c, summarizeStatus(data), "连接成功")
}

// Test 测试已登记的 agent 并刷新其在线状态。
func (h *NVRHandler) Test(c *gin.Context) {
	a, ok := h.loadAgent(c)
	if !ok {
		return
	}
	data, err := h.fetchStatus(c.Request.Context(), &a, 10*time.Second)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.OK(c, summarizeStatus(data), "连接成功")
}

func summarizeStatus(data json.RawMessage) gin.H {
	var st struct {
		Version string            `json:"version"`
		Cameras []json.RawMessage `json:"cameras"`
	}
	_ = json.Unmarshal(data, &st)
	return gin.H{"version": st.Version, "camera_count": len(st.Cameras)}
}

// fetchStatus calls /api/status and records the outcome on the agent row.
func (h *NVRHandler) fetchStatus(parent context.Context, a *models.NVRAgent, timeout time.Duration) (json.RawMessage, error) {
	client, err := h.client(a)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	data, err := client.JSON(ctx, http.MethodGet, "/api/status", nil, nil)
	h.markAgent(a, data, err)
	return data, err
}

func (h *NVRHandler) markAgent(a *models.NVRAgent, data json.RawMessage, callErr error) {
	if errors.Is(callErr, context.Canceled) {
		return
	}
	status, lastErr, version := "online", "", a.Version
	if callErr != nil {
		status, lastErr = "offline", callErr.Error()
		var ae *nvr.Error
		if errors.As(callErr, &ae) && ae.Status == http.StatusUnauthorized {
			status = "unauthorized"
		}
	} else {
		var st struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &st) == nil && st.Version != "" {
			version = st.Version
		}
	}
	if len(lastErr) > 500 {
		lastErr = lastErr[:500]
	}
	now := time.Now()
	fresh := a.LastSeenAt != nil && now.Sub(*a.LastSeenAt) < time.Minute
	if status == a.LastStatus && lastErr == a.LastError && version == a.Version && (status != "online" || fresh) {
		return
	}
	updates := map[string]interface{}{"last_status": status, "last_error": lastErr, "version": version}
	if status == "online" {
		updates["last_seen_at"] = now
		a.LastSeenAt = &now
	}
	a.LastStatus, a.LastError, a.Version = status, lastErr, version
	h.DB.Model(&models.NVRAgent{}).Where("id = ?", a.ID).Updates(updates)
}

// Cameras 汇总所有已启用 agent 的摄像头与运行状态，供监控页使用。
func (h *NVRHandler) Cameras(c *gin.Context) {
	var agents []models.NVRAgent
	if err := h.DB.Where("enabled = ?", 1).Order("id asc").Find(&agents).Error; err != nil {
		response.BizFail(c, "查询失败", 500)
		return
	}
	out := make([]gin.H, len(agents))
	var wg sync.WaitGroup
	for i := range agents {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := &agents[i]
			item := gin.H{"agent_id": a.ID, "agent_name": a.Name, "online": false, "cameras": []interface{}{}}
			data, err := h.fetchStatus(c.Request.Context(), a, 6*time.Second)
			if err != nil {
				item["error"] = err.Error()
			} else {
				var st struct {
					Version string          `json:"version"`
					Cameras json.RawMessage `json:"cameras"`
				}
				if json.Unmarshal(data, &st) == nil {
					item["online"] = true
					item["version"] = st.Version
					item["cameras"] = st.Cameras
				}
			}
			out[i] = item
		}(i)
	}
	wg.Wait()
	response.OK(c, gin.H{"items": out}, "")
}

func (h *NVRHandler) Status(c *gin.Context) {
	a, ok := h.loadAgent(c)
	if !ok {
		return
	}
	data, err := h.fetchStatus(c.Request.Context(), &a, 10*time.Second)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.OK(c, data, "")
}

func (h *NVRHandler) GetConfig(c *gin.Context) {
	h.proxyJSON(c, http.MethodGet, "/api/config", nil, false, "")
}

// agentSecretPlaceholder is what nvr-agent returns in place of stored secrets.
const agentSecretPlaceholder = "********"

// SaveConfig 保存 agent 配置；若同时修改了 agent 的 api_token，
// 平台保存的 Token 同步更新，避免保存后平台失去访问权限。
func (h *NVRHandler) SaveConfig(c *gin.Context) {
	a, ok := h.loadAgent(c)
	if !ok {
		return
	}
	client, err := h.client(&a)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, maxAgentConfigBody+1))
	var probe struct {
		APIToken *string `json:"api_token"`
	}
	if err != nil || len(payload) > maxAgentConfigBody || json.Unmarshal(payload, &probe) != nil {
		response.BizFail(c, "请求内容无效", 422)
		return
	}
	data, err := client.JSON(c.Request.Context(), http.MethodPut, "/api/config", nil, payload)
	me := middleware.CurrentUser(c)
	if err != nil {
		services.LogAction(h.DB, c, "audit", "failure", "nvr.config.update", &me, a.Name+"："+err.Error())
		writeAgentError(c, err)
		return
	}
	remark := a.Name
	if probe.APIToken != nil && *probe.APIToken != agentSecretPlaceholder && *probe.APIToken != client.Token {
		cipher, encErr := nvr.EncryptToken(h.Cfg.SecretKey, *probe.APIToken)
		if encErr == nil {
			h.DB.Model(&models.NVRAgent{}).Where("id = ?", a.ID).Update("token_cipher", cipher)
			remark += "（同步更新 Token）"
		}
	}
	services.LogAction(h.DB, c, "audit", "success", "nvr.config.update", &me, remark)
	response.OK(c, data, "")
}

func (h *NVRHandler) DebugNVR(c *gin.Context) {
	h.proxyJSON(c, http.MethodPost, "/api/debug/nvr", nil, true, "")
}

func (h *NVRHandler) DebugStream(c *gin.Context) {
	h.proxyJSON(c, http.MethodPost, "/api/debug/stream", nil, true, "")
}

func (h *NVRHandler) RecordControl(c *gin.Context) {
	action := c.Param("action")
	if action != "start" && action != "stop" {
		response.BizFail(c, "不支持的操作", 404)
		return
	}
	cam, ok := cameraParam(c)
	if !ok {
		return
	}
	h.proxyJSON(c, http.MethodPost, "/api/cameras/"+cam+"/record/"+action, nil, false, "nvr.record."+action)
}

func (h *NVRHandler) Recordings(c *gin.Context) {
	if cam, ok := cameraParam(c); ok {
		h.proxyJSON(c, http.MethodGet, "/api/cameras/"+cam+"/recordings", nil, false, "")
	}
}

func (h *NVRHandler) ArchiveStatus(c *gin.Context) {
	if cam, ok := cameraParam(c); ok {
		h.proxyJSON(c, http.MethodGet, "/api/cameras/"+cam+"/archive", nil, false, "")
	}
}

func (h *NVRHandler) ArchiveSearch(c *gin.Context) {
	cam, ok := cameraParam(c)
	if !ok {
		return
	}
	q := url.Values{"start": {c.Query("start")}, "end": {c.Query("end")}}
	h.proxyJSON(c, http.MethodGet, "/api/cameras/"+cam+"/archive/search", q, false, "")
}

func (h *NVRHandler) ArchiveStart(c *gin.Context) {
	if cam, ok := cameraParam(c); ok {
		h.proxyJSON(c, http.MethodPost, "/api/cameras/"+cam+"/archive", nil, true, "nvr.archive.start")
	}
}

// proxyJSON forwards one API call to the agent and wraps the result in the
// platform envelope. When audit is set, the action is recorded in sys logs.
func (h *NVRHandler) proxyJSON(c *gin.Context, method, path string, query url.Values, withBody bool, audit string) {
	a, ok := h.loadAgent(c)
	if !ok {
		return
	}
	client, err := h.client(&a)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	var payload []byte
	if withBody {
		payload, err = io.ReadAll(io.LimitReader(c.Request.Body, maxAgentConfigBody+1))
		if err != nil || len(payload) > maxAgentConfigBody || !json.Valid(payload) {
			response.BizFail(c, "请求内容无效", 422)
			return
		}
	}
	data, err := client.JSON(c.Request.Context(), method, path, query, payload)
	if audit != "" {
		me := middleware.CurrentUser(c)
		result := "success"
		remark := a.Name
		if cam := c.Param("cam"); cam != "" {
			remark += " / " + cam
		}
		if err != nil {
			result = "failure"
			remark += "：" + err.Error()
		}
		services.LogAction(h.DB, c, "audit", result, audit, &me, remark)
	}
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.OK(c, data, "")
}

// Live 代理 HLS 播放列表与分片：/nvr/agents/:id/live/:cam/:file
func (h *NVRHandler) Live(c *gin.Context) {
	cam, ok := cameraParam(c)
	if !ok {
		return
	}
	file := c.Param("file")
	if file != "index.m3u8" && !(strings.HasPrefix(file, "seg_") && strings.HasSuffix(file, ".ts") && nvr.ValidFileName(file)) {
		response.BizFail(c, "文件不存在", 404)
		return
	}
	h.proxyMedia(c, "/live/"+cam+"/"+file, nil)
}

// RecordingFile 代理本地录像 MP4（支持 Range 拖动）；download=1 需额外的下载权限。
func (h *NVRHandler) RecordingFile(c *gin.Context) {
	cam, ok := cameraParam(c)
	if !ok {
		return
	}
	file := c.Param("file")
	if !strings.HasSuffix(file, ".mp4") || !nvr.ValidFileName(file) {
		response.BizFail(c, "文件不存在", 404)
		return
	}
	var query url.Values
	if c.Query("download") == "1" {
		me := middleware.CurrentUser(c)
		if !h.RBAC.HasPermission(me, "recording:download") {
			response.Fail(c, http.StatusForbidden, "无下载权限", 403)
			return
		}
		query = url.Values{"download": {"1"}}
		if c.GetHeader("Range") == "" {
			services.LogAction(h.DB, c, "audit", "success", "nvr.recording.download", &me, cam+"/"+file)
		}
	}
	h.proxyMedia(c, "/recordings/"+cam+"/"+file, query)
}

var forwardedMediaHeaders = []string{
	"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges",
	"Last-Modified", "ETag", "Content-Disposition", "Cache-Control",
}

func (h *NVRHandler) proxyMedia(c *gin.Context, path string, query url.Values) {
	a, ok := h.loadAgent(c)
	if !ok {
		return
	}
	client, err := h.client(&a)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	resp, err := client.Stream(c.Request.Context(), path, query, c.Request.Header)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		writeAgentError(c, &nvr.Error{Status: resp.StatusCode, Msg: nvr.AgentErrorMessage(resp.StatusCode, body)})
		return
	}
	for _, k := range forwardedMediaHeaders {
		if v := resp.Header.Get(k); v != "" {
			c.Header(k, v)
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}

func (h *NVRHandler) loadAgent(c *gin.Context) (models.NVRAgent, bool) {
	var a models.NVRAgent
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BizFail(c, "无效 ID", 400)
		return a, false
	}
	if err := h.DB.First(&a, id).Error; err != nil {
		response.BizFail(c, "nvr-agent 不存在", 404)
		return a, false
	}
	return a, true
}

// ProbeAgentCamera checks whether an agent is reachable and reports the
// channel's presence and live-stream state. Used by the platform camera probe.
func (h *NVRHandler) ProbeAgentCamera(agentID uint, agentCamID string) (found bool, live bool, err error) {
	var a models.NVRAgent
	if err := h.DB.First(&a, agentID).Error; err != nil {
		return false, false, fmt.Errorf("节点不存在")
	}
	data, err := h.fetchStatus(context.Background(), &a, 6*time.Second)
	if err != nil {
		return false, false, err
	}
	var st struct {
		Cameras []struct {
			ID   string          `json:"id"`
			Live *struct {
				Running   bool   `json:"running"`
				LastError string `json:"last_error"`
			} `json:"live"`
		} `json:"cameras"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return false, false, fmt.Errorf("节点状态解析失败: %w", err)
	}
	for _, cam := range st.Cameras {
		if cam.ID == agentCamID {
			if cam.Live != nil && cam.Live.Running {
				return true, true, nil
			}
			return true, false, nil
		}
	}
	return false, false, nil
}

func (h *NVRHandler) client(a *models.NVRAgent) (*nvr.Client, error) {
	if a.Enabled != 1 {
		return nil, &nvr.Error{Status: http.StatusConflict, Msg: "该 nvr-agent 已停用"}
	}
	token, err := nvr.DecryptToken(h.Cfg.SecretKey, a.TokenCipher)
	if err != nil {
		return nil, &nvr.Error{Status: http.StatusConflict, Msg: err.Error()}
	}
	return nvr.New(a.BaseURL, token), nil
}

func cameraParam(c *gin.Context) (string, bool) {
	cam := c.Param("cam")
	if !nvr.ValidCameraID(cam) {
		response.BizFail(c, "摄像头 ID 无效", 400)
		return "", false
	}
	return cam, true
}

// writeAgentError maps agent failures to platform errors. Agent 401s become
// 502 so the browser session is not mistaken for expired.
func writeAgentError(c *gin.Context, err error) {
	var ae *nvr.Error
	if !errors.As(err, &ae) {
		response.BizFail(c, err.Error(), 502)
		return
	}
	switch {
	case ae.Status == http.StatusBadRequest:
		response.BizFail(c, ae.Msg, 422)
	case ae.Status == http.StatusNotFound:
		response.BizFail(c, ae.Msg, 404)
	case ae.Status == http.StatusConflict:
		response.BizFail(c, ae.Msg, 409)
	default:
		response.BizFail(c, ae.Msg, 502)
	}
}
