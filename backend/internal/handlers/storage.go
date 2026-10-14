package handlers

import (
	"errors"
	"strconv"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"
	"baseplatform/internal/settings"
	"baseplatform/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StorageHandler 存储源管理处理器。
// 云盘类驱动（onedrive/google_drive/baidu_pan）的配置通过 services 层处理，
// 其余驱动通过 storage.Manager 处理。
type StorageHandler struct {
	DB      *gorm.DB
	Manager *storage.Manager
}

type storageCreateReq struct {
	Name      string                 `json:"name" binding:"required"`
	Driver    string                 `json:"driver" binding:"required"`
	RootPath  string                 `json:"root_path"`
	Config    map[string]interface{} `json:"config"`
	Enabled   *bool                  `json:"enabled"`
	SortOrder *int                   `json:"sort_order"`
	Remark    string                 `json:"remark"`
}

type storageUpdateReq struct {
	Name      *string                `json:"name"`
	Driver    *string                `json:"driver"`
	RootPath  *string                `json:"root_path"`
	Config    map[string]interface{} `json:"config"`
	Enabled   *bool                  `json:"enabled"`
	SortOrder *int                   `json:"sort_order"`
	Remark    *string                `json:"remark"`
}

type storageTestReq struct {
	Driver string                 `json:"driver" binding:"required"`
	Config map[string]interface{} `json:"config"`
}

// storageOut 构造存储源对外结构。云盘配置返回摘要（不含 token 明文）。
func storageOut(s models.Storage, m *storage.Manager, includeConfig bool) gin.H {
	out := gin.H{
		"id":         strconv.FormatUint(uint64(s.ID), 10),
		"name":       s.Name,
		"driver":     s.Driver,
		"root_path":  s.RootPath,
		"enabled":    s.IsEnabled(),
		"sort_order": s.SortOrder,
		"remark":     s.Remark,
		"created_at": s.CreatedAt,
		"updated_at": s.UpdatedAt,
	}
	if includeConfig {
		if services.IsCloudStorageDriver(s.Driver) {
			out["config"] = services.CloudStorageConfigSummary(s.Config)
		} else {
			out["config"] = m.ParseDriverConfig(s.Driver, s.Config)
		}
	}
	return out
}

// List 列出存储源：管理员看全部，普通用户只看启用的。
func (h *StorageHandler) List(c *gin.Context) {
	user := middleware.CurrentUser(c)
	q := h.DB.Model(&models.Storage{})
	if !user.IsAdmin() {
		q = q.Where("enabled = ?", 1)
	}
	if c.Query("all") == "1" {
		var items []models.Storage
		q.Order("sort_order asc, id asc").Find(&items)
		out := make([]gin.H, 0, len(items))
		for _, item := range items {
			out = append(out, storageOut(item, h.Manager, user.IsAdmin()))
		}
		response.OK(c, services.PaginateResult(out, int64(len(items)), 1, len(items)), "")
		return
	}
	page, pageSize := services.ParsePagination(c, h.DB)
	var total int64
	q.Count(&total)
	var items []models.Storage
	q.Order("sort_order asc, id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, storageOut(item, h.Manager, user.IsAdmin()))
	}
	response.OK(c, services.PaginateResult(out, total, page, pageSize), "")
}

// Get 获取单个存储源详情。
func (h *StorageHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var item models.Storage
	if err := h.DB.First(&item, id).Error; err != nil {
		response.BizFail(c, "存储源不存在", 404)
		return
	}
	user := middleware.CurrentUser(c)
	if !user.IsAdmin() && !item.IsEnabled() {
		response.BizFail(c, "存储源不存在", 404)
		return
	}
	response.OK(c, storageOut(item, h.Manager, user.IsAdmin()), "")
}

// Create 创建存储源。
func (h *StorageHandler) Create(c *gin.Context) {
	var req storageCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 400)
		return
	}
	if !storage.IsSupportedDriver(req.Driver) && !services.IsCloudStorageDriver(req.Driver) {
		response.BizFail(c, "不支持的存储驱动", 400)
		return
	}
	rootPath := req.RootPath
	if rootPath == "" {
		rootPath = "/"
	}
	configJSON, err := encodeStorageConfig(h, req.Driver, req.Config, "")
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	item := models.Storage{
		Name:      req.Name,
		Driver:    req.Driver,
		RootPath:  storage.NormalizeVirtual(rootPath),
		Config:    configJSON,
		Enabled:   enabled,
		SortOrder: sortOrder,
		Remark:    req.Remark,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		response.BizFail(c, "创建失败", 500)
		return
	}
	user := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "ok", "storage.create", &user, item.Name)
	response.OK(c, storageOut(item, h.Manager, true), "已创建")
}

// Update 更新存储源。
func (h *StorageHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var item models.Storage
	if err := h.DB.First(&item, id).Error; err != nil {
		response.BizFail(c, "存储源不存在", 404)
		return
	}
	var req storageUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 400)
		return
	}
	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.Driver != nil {
		if !storage.IsSupportedDriver(*req.Driver) && !services.IsCloudStorageDriver(*req.Driver) {
			response.BizFail(c, "不支持的存储驱动", 400)
			return
		}
		item.Driver = *req.Driver
	}
	if req.RootPath != nil {
		item.RootPath = storage.NormalizeVirtual(*req.RootPath)
	}
	if req.Remark != nil {
		item.Remark = *req.Remark
	}
	if req.SortOrder != nil {
		item.SortOrder = *req.SortOrder
	}
	if req.Enabled != nil {
		if *req.Enabled {
			item.Enabled = 1
		} else {
			item.Enabled = 0
		}
	}
	if req.Config != nil {
		configJSON, err := encodeStorageConfig(h, item.Driver, req.Config, item.Config)
		if err != nil {
			response.BizFail(c, err.Error(), 400)
			return
		}
		item.Config = configJSON
	}
	if err := h.DB.Save(&item).Error; err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	user := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "ok", "storage.update", &user, item.Name)
	response.OK(c, storageOut(item, h.Manager, true), "已保存")
}

// Delete 删除存储源。
func (h *StorageHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var item models.Storage
	if err := h.DB.First(&item, id).Error; err != nil {
		response.BizFail(c, "存储源不存在", 404)
		return
	}
	if err := h.DB.Delete(&item).Error; err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	user := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "ok", "storage.delete", &user, item.Name)
	response.OK(c, nil, "已删除")
}

// encodeStorageConfig 按驱动类型编码配置：云盘走 services 层（保留既有 token），
// 其余走 storage.Manager。
func encodeStorageConfig(h *StorageHandler, driver string, raw map[string]interface{}, existing string) (string, error) {
	if services.IsCloudStorageDriver(driver) {
		return services.MergeCloudStorageConfig(driver, raw, existing)
	}
	if raw != nil && existing != "" {
		prev := h.Manager.ParseDriverConfigUnmasked(driver, existing)
		for _, key := range []string{"password", "secret_key"} {
			if v, ok := raw[key].(string); ok && settings.IsSecretPlaceholder(v) {
				if old, ok := prev[key].(string); ok {
					raw[key] = old
				}
			}
		}
	}
	return h.Manager.EncodeDriverConfig(driver, raw)
}

// Test 测试存储源连接（按 ID）。
func (h *StorageHandler) Test(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	item, err := h.Manager.GetStorage(uint(id))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			response.BizFail(c, "存储源不存在", 404)
			return
		}
		response.BizFail(c, "读取失败", 500)
		return
	}
	if err := h.testStoredConfig(item); err != nil {
		response.BizFail(c, "连接失败: "+err.Error(), 400)
		return
	}
	response.OK(c, gin.H{"ok": true}, "连接正常")
}

// TestConfig 按前端表单配置测试连接（无需先创建）。
func (h *StorageHandler) TestConfig(c *gin.Context) {
	var req storageTestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 400)
		return
	}
	if err := h.testDriverConfig(req.Driver, req.Config, ""); err != nil {
		response.BizFail(c, "连接失败: "+err.Error(), 400)
		return
	}
	response.OK(c, gin.H{"ok": true}, "连接正常")
}

func (h *StorageHandler) testStoredConfig(item *models.Storage) error {
	if services.IsCloudStorageDriver(item.Driver) {
		summary := storage.ParseCloudStorageConfig(item.Config)
		connected, _ := summary["connected"].(bool)
		if !connected {
			return errors.New("云盘尚未授权，请先完成 OAuth 连接")
		}
		// 云盘远端驱动尚未接入，仅校验授权状态。
		return nil
	}
	drv, err := h.Manager.DriverFor(item)
	if err != nil {
		return err
	}
	return drv.Test()
}

func (h *StorageHandler) testDriverConfig(driver string, config map[string]interface{}, existing string) error {
	if !storage.IsSupportedDriver(driver) && !services.IsCloudStorageDriver(driver) {
		return errors.New("不支持的存储驱动")
	}
	configJSON, err := encodeStorageConfig(h, driver, config, existing)
	if err != nil {
		return err
	}
	if services.IsCloudStorageDriver(driver) {
		summary := storage.ParseCloudStorageConfig(configJSON)
		connected, _ := summary["connected"].(bool)
		if !connected {
			return errors.New("云盘尚未授权，请先完成 OAuth 连接")
		}
		return nil
	}
	item := models.Storage{Driver: driver, Config: configJSON}
	drv, err := h.Manager.DriverFor(&item)
	if err != nil {
		return err
	}
	return drv.Test()
}

// Browse 浏览存储源目录。
func (h *StorageHandler) Browse(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	item, err := h.Manager.GetStorage(uint(id))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			response.BizFail(c, "存储源不存在", 404)
			return
		}
		response.BizFail(c, "读取失败", 500)
		return
	}
	user := middleware.CurrentUser(c)
	if !user.IsAdmin() && !item.IsEnabled() {
		response.BizFail(c, "存储源不存在", 404)
		return
	}
	drv, err := h.Manager.DriverFor(item)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	dir := c.DefaultQuery("path", "/")
	items, err := drv.List(dir)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			response.BizFail(c, "目录不存在", 404)
			return
		}
		response.BizFail(c, "读取目录失败: "+err.Error(), 400)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		out = append(out, storage.FileItemOut(it))
	}
	response.OK(c, gin.H{"path": storage.NormalizeVirtual(dir), "items": out}, "")
}

// Drivers 返回支持的驱动列表。
func (h *StorageHandler) Drivers(c *gin.Context) {
	drivers := []gin.H{
		{"driver": "local", "name": "本地存储", "cloud": false},
		{"driver": "s3", "name": "S3 / 对象存储", "cloud": false},
		{"driver": "oss", "name": "阿里云 OSS", "cloud": false},
		{"driver": "ftp", "name": "FTP", "cloud": false},
		{"driver": "sftp", "name": "SFTP", "cloud": false},
		{"driver": "webdav", "name": "WebDAV", "cloud": false},
		{"driver": "nfs", "name": "NFS", "cloud": false},
		{"driver": "smb", "name": "SMB", "cloud": false},
		{"driver": "onedrive", "name": "OneDrive", "cloud": true},
		{"driver": "google_drive", "name": "Google Drive", "cloud": true},
		{"driver": "baidu_pan", "name": "百度网盘", "cloud": true},
	}
	response.OK(c, drivers, "")
}

// ConfigSchema 返回各驱动的配置字段说明。
func (h *StorageHandler) ConfigSchema(c *gin.Context) {
	schema := map[string][]gin.H{
		"local": {
			{"key": "base_path", "label": "本地根目录", "type": "string", "required": false},
		},
		"s3": {
			{"key": "endpoint", "label": "Endpoint", "type": "string", "required": true},
			{"key": "bucket", "label": "Bucket", "type": "string", "required": true},
			{"key": "access_key", "label": "AccessKey", "type": "string", "required": true},
			{"key": "secret_key", "label": "SecretKey", "type": "password", "required": true},
			{"key": "region", "label": "Region", "type": "string", "required": false},
			{"key": "use_ssl", "label": "使用 SSL", "type": "bool", "required": false},
			{"key": "prefix", "label": "路径前缀", "type": "string", "required": false},
		},
		"ftp": {
			{"key": "host", "label": "主机", "type": "string", "required": true},
			{"key": "port", "label": "端口", "type": "int", "required": false},
			{"key": "username", "label": "用户名", "type": "string", "required": false},
			{"key": "password", "label": "密码", "type": "password", "required": false},
			{"key": "base_path", "label": "根路径", "type": "string", "required": false},
		},
		"sftp": {
			{"key": "host", "label": "主机", "type": "string", "required": true},
			{"key": "port", "label": "端口", "type": "int", "required": false},
			{"key": "username", "label": "用户名", "type": "string", "required": false},
			{"key": "password", "label": "密码/密钥", "type": "password", "required": false},
			{"key": "base_path", "label": "根路径", "type": "string", "required": false},
		},
		"webdav": {
			{"key": "url", "label": "服务地址", "type": "string", "required": true},
			{"key": "username", "label": "用户名", "type": "string", "required": false},
			{"key": "password", "label": "密码", "type": "password", "required": false},
		},
		"nfs": {
			{"key": "server", "label": "服务器", "type": "string", "required": true},
			{"key": "export", "label": "导出路径", "type": "string", "required": true},
			{"key": "mount_point", "label": "挂载点", "type": "string", "required": false},
			{"key": "sub_path", "label": "子路径", "type": "string", "required": false},
			{"key": "version", "label": "NFS 版本", "type": "int", "required": false},
			{"key": "options", "label": "挂载选项", "type": "string", "required": false},
		},
		"smb": {
			{"key": "host", "label": "主机", "type": "string", "required": true},
			{"key": "port", "label": "端口", "type": "int", "required": false},
			{"key": "username", "label": "用户名", "type": "string", "required": false},
			{"key": "password", "label": "密码", "type": "password", "required": false},
			{"key": "domain", "label": "域", "type": "string", "required": false},
			{"key": "share", "label": "共享名", "type": "string", "required": true},
			{"key": "base_path", "label": "根路径", "type": "string", "required": false},
		},
		"onedrive": {
			{"key": "remote_path", "label": "远端根路径", "type": "string", "required": false},
		},
		"google_drive": {
			{"key": "remote_path", "label": "远端根路径", "type": "string", "required": false},
		},
		"baidu_pan": {
			{"key": "remote_path", "label": "远端根路径", "type": "string", "required": false},
		},
	}
	response.OK(c, schema, "")
}
