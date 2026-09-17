package handlers

import (
	"fmt"
	"strconv"

	"baseplatform/internal/avatars"
	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AvatarHandler struct {
	DB     *gorm.DB
	Avatar *avatars.Service
}

type avatarPresetReq struct {
	Preset string `json:"preset" binding:"required"`
}

func (h *AvatarHandler) UploadUserAvatar(c *gin.Context) {
	u := middleware.CurrentUser(c)
	url, err := h.saveUpload(c, "users", u.ID)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	h.DB.Model(&u).Update("avatar_url", url)
	u.AvatarURL = &url
	services.LogAction(h.DB, c, "operation", "success", "auth.avatar.upload", &u, "")
	response.OK(c, userPayload(h.DB, u), "头像已更新")
}

func (h *AvatarHandler) SetUserAvatarPreset(c *gin.Context) {
	u := middleware.CurrentUser(c)
	url, err := h.setPreset(c)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	h.Avatar.RemoveCustom("users", u.ID)
	h.DB.Model(&u).Update("avatar_url", url)
	u.AvatarURL = &url
	services.LogAction(h.DB, c, "operation", "success", "auth.avatar.preset", &u, url)
	response.OK(c, userPayload(h.DB, u), "头像已更新")
}

func (h *AvatarHandler) DeleteUserAvatar(c *gin.Context) {
	u := middleware.CurrentUser(c)
	h.Avatar.RemoveCustom("users", u.ID)
	h.DB.Model(&u).Update("avatar_url", nil)
	u.AvatarURL = nil
	services.LogAction(h.DB, c, "operation", "success", "auth.avatar.delete", &u, "")
	response.OK(c, userPayload(h.DB, u), "已恢复默认头像")
}

func (h *AvatarHandler) UploadUserAvatarAdmin(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("uid"))
	var u models.User
	if err := h.DB.First(&u, id).Error; err != nil {
		response.BizFail(c, "用户不存在", 404)
		return
	}
	url, err := h.saveUpload(c, "users", u.ID)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	h.DB.Model(&u).Update("avatar_url", url)
	u.AvatarURL = &url
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "operation", "success", "user.avatar.upload", &me, u.Username)
	response.OK(c, userOut(h.DB, u), "头像已更新")
}

func (h *AvatarHandler) SetUserAvatarPresetAdmin(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("uid"))
	var u models.User
	if err := h.DB.First(&u, id).Error; err != nil {
		response.BizFail(c, "用户不存在", 404)
		return
	}
	url, err := h.setPreset(c)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	h.Avatar.RemoveCustom("users", u.ID)
	h.DB.Model(&u).Update("avatar_url", url)
	u.AvatarURL = &url
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "operation", "success", "user.avatar.preset", &me, u.Username)
	response.OK(c, userOut(h.DB, u), "头像已更新")
}

func (h *AvatarHandler) DeleteUserAvatarAdmin(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("uid"))
	var u models.User
	if err := h.DB.First(&u, id).Error; err != nil {
		response.BizFail(c, "用户不存在", 404)
		return
	}
	h.Avatar.RemoveCustom("users", u.ID)
	h.DB.Model(&u).Update("avatar_url", nil)
	u.AvatarURL = nil
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "operation", "success", "user.avatar.delete", &me, u.Username)
	response.OK(c, userOut(h.DB, u), "已恢复默认头像")
}

func (h *AvatarHandler) ListPresets(c *gin.Context) {
	items := []string{"user-01.svg", "user-02.svg", "user-03.svg", "user-04.svg", "user-05.svg", "user-06.svg"}
	out := make([]gin.H, 0, len(items))
	for _, name := range items {
		out = append(out, gin.H{
			"name": name,
			"url":  h.Avatar.PresetURL(name),
		})
	}
	response.OK(c, gin.H{"items": out}, "")
}

func (h *AvatarHandler) saveUpload(c *gin.Context, kind string, id uint) (string, error) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return "", fmt.Errorf("请上传图片文件")
	}
	defer file.Close()
	return h.Avatar.SaveUpload(kind, id, file, header)
}

func (h *AvatarHandler) setPreset(c *gin.Context) (string, error) {
	var req avatarPresetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		return "", fmt.Errorf("请选择头像")
	}
	return h.Avatar.SetPreset(req.Preset)
}
