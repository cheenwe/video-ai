package services

import (
	"encoding/json"
	"log"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"baseplatform/internal/models"
)

// sensitiveKeys 需要脱敏的字段名（不区分大小写匹配）。
var sensitiveKeys = map[string]struct{}{
	"password":      {},
	"password_hash": {},
	"passwd":        {},
	"secret":        {},
	"secret_key":    {},
	"client_secret": {},
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"api_key":       {},
	"smtp_pass":     {},
	"bind_pass":     {},
	"totp_secret":   {},
	"license":       {},
	"private_key":   {},
	"credential":    {},
	"credentials":   {},
}

// maskSensitive 递归脱敏 map 中的敏感字段，返回新 map，不修改原数据。
func maskSensitive(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	out := make(map[string]interface{}, len(data))
	for k, v := range data {
		lk := strings.ToLower(k)
		if _, ok := sensitiveKeys[lk]; ok {
			out[k] = "***"
			continue
		}
		switch val := v.(type) {
		case map[string]interface{}:
			out[k] = maskSensitive(val)
		case []interface{}:
			out[k] = maskSensitiveSlice(val)
		default:
			out[k] = v
		}
	}
	return out
}

func maskSensitiveSlice(items []interface{}) []interface{} {
	out := make([]interface{}, len(items))
	for i, v := range items {
		switch val := v.(type) {
		case map[string]interface{}:
			out[i] = maskSensitive(val)
		case []interface{}:
			out[i] = maskSensitiveSlice(val)
		default:
			out[i] = v
		}
	}
	return out
}

// toJSONMap 将任意结构体转为 map[string]interface{} 并脱敏。
func toMaskedJSON(v interface{}) string {
	if v == nil || (reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil()) {
		return ""
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	masked := maskSensitive(m)
	out, _ := json.Marshal(masked)
	return string(out)
}

// LogAction 写入 sys_logs。
// tag 约定：
//   - login：登录、登出、登录失败、OAuth 登录
//   - audit：安全/合规敏感操作（用户/权限/存储/安全配置、2FA、系统配置项等）
//   - sync：存储同步任务（创建/执行/定时调度/结果）
//   - 其他（file/storage/share/operation/agent/...）：日常业务操作日志
func LogAction(db *gorm.DB, c *gin.Context, tag, result, action string, user *models.User, remark string) {
	LogActionSource(db, c, tag, result, action, user, remark, 0)
}

// LogActionWithChange 写入带变更前后数据的审计日志（自动脱敏敏感字段）。
func LogActionWithChange(db *gorm.DB, c *gin.Context, tag, result, action string, user *models.User, remark string, before, after interface{}) {
	entry := buildLogEntry(c, tag, result, action, user, remark, 0)
	entry.BeforeData = toMaskedJSON(before)
	entry.AfterData = toMaskedJSON(after)
	_ = db.Create(&entry).Error
}

func LogActionSource(db *gorm.DB, c *gin.Context, tag, result, action string, user *models.User, remark string, source uint8) {
	entry := buildLogEntry(c, tag, result, action, user, remark, source)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		if tag == "audit" && result == "success" && !strings.HasSuffix(action, ".view") {
			return NewEventBus(tx).Emit(action, user, map[string]interface{}{"title": "平台操作完成", "content": action, "level": "info"})
		}
		return nil
	}); err != nil {
		log.Printf("audit persistence failed for %s: %v", action, err)
	}
}

func buildLogEntry(c *gin.Context, tag, result, action string, user *models.User, remark string, source uint8) models.SysLog {
	var uid *uint
	username := ""
	if user != nil {
		uid = &user.ID
		username = user.Username
	}
	return models.SysLog{
		IP:        auditClientIP(c),
		UserID:    uid,
		Username:  username,
		Tag:       tag,
		Result:    result,
		Action:    action,
		RequestID: auditRequestID(c),
		Remark:    remark,
		Source:    source,
	}
}

func auditClientIP(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.ClientIP()
}

func auditRequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get("request_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// LogSystem 写入系统/后台任务日志（无 HTTP 请求上下文）。
func LogSystem(db *gorm.DB, tag, result, action, remark string) {
	entry := models.SysLog{
		Username: "system",
		Tag:      tag,
		Result:   result,
		Action:   action,
		Remark:   remark,
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		if tag == "schedule" {
			return NewEventBus(tx).Emit(action, nil, map[string]interface{}{"title": "计划任务执行结果", "content": action + ": " + result, "level": result})
		}
		return nil
	}); err != nil {
		log.Printf("system audit persistence: %v", err)
	}
}

// LogFileAction 网关/无 gin 上下文的文件操作日志。
func LogFileAction(db *gorm.DB, user *models.User, source uint8, action, remark, ip string) {
	var uid *uint
	username := ""
	if user != nil {
		uid = &user.ID
		username = user.Username
	} else if source == models.FileSourceNFS {
		username = "nfs"
	}
	entry := models.SysLog{
		IP:       ip,
		UserID:   uid,
		Username: username,
		Tag:      "file",
		Result:   "ok",
		Action:   action,
		Remark:   remark,
		Source:   source,
	}
	_ = db.Create(&entry).Error
}
