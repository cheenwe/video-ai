package handlers

import (
	"strconv"
	"time"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"
	"baseplatform/internal/settings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TokenHandler struct {
	DB  *gorm.DB
	PAT *services.PATService
}

type tokenCreateReq struct {
	Name      string  `json:"name" binding:"required,max=128"`
	ExpiresIn *int64 `json:"expires_in"` // 秒；可选
}

type tokenUpdateReq struct {
	Name      *string `json:"name"`
	Enabled   *bool   `json:"enabled"`
	ExpiresAt *int64  `json:"expires_at"` // unix 秒；传 0 清空
}

func tokenOut(row models.PersonalAccessToken, raw string) gin.H {
	out := gin.H{
		"id":           row.ID,
		"name":         row.Name,
		"token_prefix": row.TokenPrefix,
		"enabled":      row.Enabled != 0,
		"expires_at":   row.ExpiresAt,
		"last_used_at": row.LastUsedAt,
		"created_by":   row.CreatedBy,
		"created_at":   row.CreatedAt,
	}
	if raw != "" {
		out["token"] = raw
	}
	return out
}

func (h *TokenHandler) List(c *gin.Context) {
	u := middleware.CurrentUser(c)
	rows, err := h.PAT.ListForUser(u.ID)
	if err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, tokenOut(r, ""))
	}
	response.OK(c, gin.H{"items": out}, "")
}

func (h *TokenHandler) Create(c *gin.Context) {
	var req tokenCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	u := middleware.CurrentUser(c)
	var exp *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Second)
		exp = &t
	}
	row, raw, err := h.PAT.Create(u, services.CreateTokenInput{Name: req.Name, ExpiresAt: exp, CreatedBy: &u.ID})
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "token.create", &u, row.Name)
	response.OK(c, tokenOut(*row, raw), "令牌已创建，明文仅展示一次")
}

func (h *TokenHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var req tokenUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	u := middleware.CurrentUser(c)
	in := services.UpdateTokenInput{Name: req.Name, Enabled: req.Enabled}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == 0 {
			var nilTime *time.Time
			in.ExpiresAt = &nilTime
		} else {
			t := time.Unix(*req.ExpiresAt, 0)
			ptr := &t
			in.ExpiresAt = &ptr
		}
	}
	row, err := h.PAT.Update(u, uint(id), in)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "token.update", &u, row.Name)
	response.OK(c, tokenOut(*row, ""), "已更新")
}

func (h *TokenHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	u := middleware.CurrentUser(c)
	if err := h.PAT.Delete(u.ID, uint(id)); err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "token.delete", &u, "")
	response.OK(c, nil, "已删除")
}

// AdminListForUser 管理员查看指定用户的令牌（仅元数据）。
func (h *TokenHandler) AdminListForUser(c *gin.Context) {
	uid, err := strconv.Atoi(c.Param("uid"))
	if err != nil || uid < 1 {
		response.BizFail(c, "无效用户 ID", 400)
		return
	}
	rows, err := h.PAT.AdminListForUser(uint(uid))
	if err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, tokenOut(r, ""))
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "token.admin.list", &me, "")
	response.OK(c, gin.H{"items": out}, "")
}

func (h *TokenHandler) AdminDeleteForUser(c *gin.Context) {
	uid, err := strconv.Atoi(c.Param("uid"))
	if err != nil || uid < 1 {
		response.BizFail(c, "无效用户 ID", 400)
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效令牌 ID", 400)
		return
	}
	if err := h.PAT.AdminDeleteForUser(uint(uid), uint(id)); err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "token.admin.delete", &me, "")
	response.OK(c, nil, "已删除")
}

// GatewayStatus 返回个人令牌网关开关（用户可见）。
func (h *TokenHandler) GatewayStatus(c *gin.Context) {
	response.OK(c, gin.H{"personal_tokens_enabled": settings.LoadGateway(h.DB).PersonalTokensEnabled}, "")
}
