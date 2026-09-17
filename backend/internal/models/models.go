package models

import "time"

type User struct {
	LastTOTPStep   int64   `gorm:"not null;default:0" json:"-"`
	SessionVersion int     `gorm:"not null;default:0" json:"-"`
	ID             uint    `gorm:"primaryKey" json:"id"`
	Username       string  `gorm:"size:64;uniqueIndex;not null" json:"username"`
	Email          *string `gorm:"size:128;uniqueIndex" json:"-"`
	PasswordHash   string  `gorm:"size:255;not null" json:"-"`

	Role               string     `gorm:"size:32;not null;default:user" json:"role"`
	Disabled           int        `gorm:"not null;default:0" json:"disabled"`
	WebLoginEnabled    int        `gorm:"not null;default:1" json:"web_login_enabled"` // 0 = 禁止 Web 登录（如 guest）
	RealName           *string    `gorm:"size:64" json:"real_name"`
	DepartmentID       *uint      `gorm:"index" json:"department_id"`
	AuthSource         string     `gorm:"size:16;not null;default:local;index" json:"auth_source"` // local | ldap
	LDAPDN             string     `gorm:"size:512;index" json:"ldap_dn"`
	Timezone           string     `gorm:"size:64;not null;default:UTC" json:"timezone"`
	AvatarURL          *string    `gorm:"size:512" json:"avatar_url"`
	TOTPSecret         *string    `gorm:"size:64" json:"-"`
	TOTPEnabled        int        `gorm:"not null;default:0" json:"-"`
	MustChangePassword int        `gorm:"not null;default:0" json:"-"` // 1 = 首次登录需修改密码
	LastLoginAt        *time.Time `gorm:"index" json:"last_login_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type Department struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	ParentID  *uint     `gorm:"index" json:"parent_id"`
	SortOrder int       `gorm:"not null;default:0" json:"sort_order"`
	Remark    string    `gorm:"size:512" json:"remark"`
	Source    string    `gorm:"size:16;not null;default:local;index" json:"source"` // local | ldap
	LDAPDN    string    `gorm:"size:512;index" json:"ldap_dn"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SysConfig struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Category  string    `gorm:"size:64;not null;uniqueIndex:idx_cat_code" json:"category"`
	Code      string    `gorm:"size:128;not null;uniqueIndex:idx_cat_code" json:"code"`
	Value     string    `gorm:"type:text" json:"value"`
	Status    string    `gorm:"size:32;default:active" json:"status"`
	Version   string    `gorm:"size:32" json:"version"`
	UpdatedBy *uint     `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SysLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	IP         string    `gorm:"size:45" json:"ip"`
	UserID     *uint     `json:"user_id"`
	Username   string    `gorm:"size:64" json:"username"`
	Tag        string    `gorm:"size:32" json:"tag"`
	Result     string    `gorm:"size:16" json:"result"`
	Action     string    `gorm:"size:128" json:"action"`
	RequestID  string    `gorm:"size:64" json:"request_id"`
	Remark     string    `gorm:"type:text" json:"remark"`
	Source     uint8     `gorm:"not null;default:0" json:"source"` // FileSource*
	BeforeData string    `gorm:"type:text" json:"-"`               // 变更前数据（JSON，已脱敏）
	AfterData  string    `gorm:"type:text" json:"-"`               // 变更后数据（JSON，已脱敏）
}

type AuthRevokedToken struct {
	ID        uint      `gorm:"primaryKey"`
	JTI       string    `gorm:"size:64;uniqueIndex;not null"`
	RevokedAt time.Time `gorm:"not null"`
	ExpiresAt *time.Time
}

type OAuthAccount struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index;not null;uniqueIndex:idx_oauth_provider_subject" json:"user_id"`
	ProviderID string    `gorm:"size:64;not null;uniqueIndex:idx_oauth_provider_subject" json:"provider_id"`
	Subject    string    `gorm:"size:128;not null;uniqueIndex:idx_oauth_provider_subject" json:"subject"`
	CreatedAt  time.Time `json:"created_at"`
}

func (OAuthAccount) TableName() string { return "oauth_accounts" }

// PersonalAccessToken 个人访问令牌（仅存 hash，明文只在创建时返回一次）。
// 简化版：令牌继承用户角色权限，不含存储范围。
type PersonalAccessToken struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	UserID      uint       `gorm:"index;not null" json:"user_id"`
	Name        string     `gorm:"size:128;not null" json:"name"`
	TokenPrefix string     `gorm:"size:16;not null;index" json:"token_prefix"`
	TokenHash   string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	Enabled     int        `gorm:"not null;default:1" json:"enabled"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedBy   *uint      `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (PersonalAccessToken) TableName() string { return "personal_access_tokens" }

// ApiCallLog PAT 令牌调用日志，记录开放 API 的访问明细。
type ApiCallLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TokenID    uint      `gorm:"index;not null" json:"token_id"`
	UserID     uint      `gorm:"index;not null" json:"user_id"`
	Method     string    `gorm:"size:16;not null" json:"method"`
	Path       string    `gorm:"size:512;not null" json:"path"`
	StatusCode int       `gorm:"not null;default:0" json:"status_code"`
	DurationMs int64     `gorm:"not null;default:0" json:"duration_ms"`
	IP         string    `gorm:"size:64" json:"ip"`
	UserAgent  string    `gorm:"size:512" json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
}

func (ApiCallLog) TableName() string { return "api_call_logs" }

// ScheduleCommand 可执行命令模板。
type ScheduleCommand struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:128;not null" json:"name"`
	Key          string    `gorm:"size:64;uniqueIndex;not null" json:"key"`
	Kind         string    `gorm:"size:16;not null;default:custom" json:"kind"` // preset|custom
	Bin          string    `gorm:"size:255;not null" json:"bin"`
	ArgsTemplate string    `gorm:"type:text;not null" json:"args_template"`
	Workdir      string    `gorm:"size:512" json:"workdir"`
	TimeoutSec   int       `gorm:"not null;default:3600" json:"timeout_sec"`
	Enabled      int       `gorm:"not null;default:1" json:"enabled"`
	Description  string    `gorm:"size:512" json:"description"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (ScheduleCommand) TableName() string { return "schedule_commands" }

func (c ScheduleCommand) IsEnabled() bool { return c.Enabled != 0 }
func (c ScheduleCommand) IsPreset() bool  { return c.Kind == "preset" }

// ScheduleJob cron 调度任务。
type ScheduleJob struct {
	Timezone     string     `gorm:"not null;default:Asia/Shanghai" json:"timezone"`
	ID           uint       `gorm:"primaryKey" json:"id"`
	Name         string     `gorm:"size:128;not null" json:"name"`
	JobType      string     `gorm:"size:16;not null" json:"job_type"` // builtin|command
	BuiltinKey   string     `gorm:"size:64;index" json:"builtin_key"`
	CommandID    *uint      `gorm:"index" json:"command_id"`
	Args         string     `gorm:"type:text" json:"args"`
	Cron         string     `gorm:"size:64;not null" json:"cron"`
	Enabled      int        `gorm:"not null;default:0" json:"enabled"`
	Options      string     `gorm:"type:text" json:"options"`
	IsBuiltin    int        `gorm:"not null;default:0" json:"is_builtin"`
	LastRunAt    *time.Time `json:"last_run_at"`
	LastStatus   string     `gorm:"size:32" json:"last_status"`
	LastError    string     `gorm:"type:text" json:"last_error"`
	LastDuration int64      `gorm:"not null;default:0" json:"last_duration_ms"`
	RunCount     int64      `gorm:"not null;default:0" json:"run_count"`
	FailCount    int64      `gorm:"not null;default:0" json:"fail_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (ScheduleJob) TableName() string { return "schedule_jobs" }

func (j ScheduleJob) IsEnabled() bool { return j.Enabled != 0 }
func (j ScheduleJob) Builtin() bool   { return j.IsBuiltin != 0 }

const (
	RoleUser       = "user"
	RoleAdmin      = "admin"
	RoleSuperAdmin = "superadmin"

	AuthSourceLocal = "local"
	AuthSourceLDAP  = "ldap"
)

func (u User) IsLDAPAuth() bool {
	return u.AuthSource == AuthSourceLDAP
}

// SystemUsernames 系统内置账号：不可删除、不可改角色。
var SystemUsernames = map[string]struct{}{
	"admin":      {},
	"guest":      {},
	"superadmin": {},
}

func IsSystemUsername(username string) bool {
	_, ok := SystemUsernames[username]
	return ok
}

func (u User) IsSuperAdmin() bool { return u.Role == RoleSuperAdmin }

// IsAdmin 含超级管理员（可进入管理后台、绕过路径 ACL 等）。
func (u User) IsAdmin() bool { return u.Role == RoleAdmin || u.Role == RoleSuperAdmin }

func (u User) IsSystemUser() bool {
	return IsSystemUsername(u.Username) || u.Role == RoleSuperAdmin
}

func (u User) CanWebLogin() bool { return u.WebLoginEnabled != 0 }

// ---- RBAC 模型 ----

// DataScope 数据范围类型。
const (
	DataScopeAll        = "all"          // 全部数据
	DataScopeProject    = "project"      // 所在项目
	DataScopeDept       = "dept"         // 本部门
	DataScopeDeptAndSub = "dept_and_sub" // 本部门及下级
	DataScopeSelf       = "self"         // 仅本人
	DataScopeCustom     = "custom"       // 自定义部门
)

// Role 角色。
type Role struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Code        string    `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name        string    `gorm:"size:128;not null" json:"name"`
	Description string    `gorm:"size:512" json:"description"`
	DataScope   string    `gorm:"size:32;not null;default:self" json:"data_scope"`
	IsBuiltin   int       `gorm:"not null;default:0" json:"is_builtin"`
	SortOrder   int       `gorm:"not null;default:0" json:"sort_order"`
	Enabled     int       `gorm:"not null;default:1" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Role) TableName() string { return "roles" }

// Permission 权限点。
type Permission struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"size:128;uniqueIndex;not null" json:"code"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Module    string    `gorm:"size:64;not null;index" json:"module"`
	Type      string    `gorm:"size:16;not null;default:action" json:"type"` // menu | action
	ParentID  *uint     `gorm:"index" json:"parent_id"`
	SortOrder int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

func (Permission) TableName() string { return "permissions" }

// RolePermission 角色-权限关联。
type RolePermission struct {
	RoleID       uint `gorm:"primaryKey" json:"role_id"`
	PermissionID uint `gorm:"primaryKey" json:"permission_id"`
}

func (RolePermission) TableName() string { return "role_permissions" }

// UserRole 用户-角色关联。
type UserRole struct {
	UserID uint `gorm:"primaryKey" json:"user_id"`
	RoleID uint `gorm:"primaryKey" json:"role_id"`
}

func (UserRole) TableName() string { return "user_roles" }

// Project 项目（数据范围隔离维度）。
type Project struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Code        string    `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name        string    `gorm:"size:128;not null" json:"name"`
	Description string    `gorm:"size:512" json:"description"`
	OwnerID     *uint     `gorm:"index" json:"owner_id"`
	Enabled     int       `gorm:"not null;default:1" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Project) TableName() string { return "projects" }

// ProjectMember 项目成员。
type ProjectMember struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID uint      `gorm:"index;not null" json:"project_id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Role      string    `gorm:"size:32;not null;default:member" json:"role"` // owner | admin | member
	CreatedAt time.Time `json:"created_at"`
}

func (ProjectMember) TableName() string { return "project_members" }

// Storage 存储源（本地 / S3 / 云盘等）。
type Storage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Driver    string    `gorm:"size:32;not null;default:local" json:"driver"`
	RootPath  string    `gorm:"size:512;not null;default:/" json:"root_path"`
	Config    string    `gorm:"type:text" json:"-"`
	Enabled   int       `gorm:"not null;default:1" json:"enabled"`
	SortOrder int       `gorm:"not null;default:0" json:"sort_order"`
	Remark    string    `gorm:"size:512" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Storage) TableName() string { return "storages" }

func (s Storage) IsEnabled() bool { return s.Enabled != 0 }

// SysDict 字典类型（如告警级别、设备类型、状态枚举）。
type SysDict struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Code        string    `gorm:"size:64;not null;uniqueIndex" json:"code"`
	Name        string    `gorm:"size:128;not null" json:"name"`
	Description string    `gorm:"size:512" json:"description"`
	SortOrder   int       `gorm:"not null;default:0" json:"sort_order"`
	Enabled     int       `gorm:"not null;default:1" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (SysDict) TableName() string { return "sys_dicts" }

func (d SysDict) IsEnabled() bool { return d.Enabled != 0 }

// SysDictItem 字典项。
type SysDictItem struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	DictID    uint      `gorm:"not null;index:idx_dict_item" json:"dict_id"`
	Code      string    `gorm:"size:64;not null;index:idx_dict_item" json:"code"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Value     string    `gorm:"size:255;not null" json:"value"`
	SortOrder int       `gorm:"not null;default:0" json:"sort_order"`
	Enabled   int       `gorm:"not null;default:1" json:"enabled"`
	Remark    string    `gorm:"size:512" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SysDictItem) TableName() string { return "sys_dict_items" }

func (i SysDictItem) IsEnabled() bool { return i.Enabled != 0 }

// Notification 站内通知。
type Notification struct {
	DedupeKey *string    `gorm:"size:64;uniqueIndex" json:"-"`
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"not null;index" json:"user_id"`
	Type      string     `gorm:"size:32;not null" json:"type"` // info / success / warning / error
	Title     string     `gorm:"size:256;not null" json:"title"`
	Content   string     `gorm:"type:text" json:"content"`
	Level     string     `gorm:"size:16;not null;default:info" json:"level"`
	IsRead    int        `gorm:"not null;default:0;index" json:"is_read"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	RefType   string     `gorm:"size:64" json:"ref_type"`        // 关联对象类型，如 event / alert
	RefID     string     `gorm:"size:128" json:"ref_id"`         // 关联对象 ID
	EventID   string     `gorm:"size:128;index" json:"event_id"` // 来源事件 ID（用于去重）
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (Notification) TableName() string { return "notifications" }

// WebhookEndpoint Webhook 投递端点。
type WebhookEndpoint struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"size:128;not null" json:"name"`
	URL        string    `gorm:"size:512;not null" json:"url"`
	Secret     string    `gorm:"size:256" json:"-"`            // HMAC 签名密钥
	EventTypes string    `gorm:"type:text" json:"event_types"` // 逗号分隔的事件类型，空=全部
	Enabled    int       `gorm:"not null;default:1" json:"enabled"`
	TimeoutSec int       `gorm:"not null;default:10" json:"timeout_sec"`
	MaxRetries int       `gorm:"not null;default:0" json:"max_retries"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (WebhookEndpoint) TableName() string { return "webhook_endpoints" }

func (w WebhookEndpoint) IsEnabled() bool { return w.Enabled != 0 }

// WebhookDelivery Webhook 投递记录。
type WebhookDelivery struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	EndpointID   uint       `gorm:"not null;index" json:"endpoint_id"`
	DeliveryID   string     `gorm:"size:64;not null;uniqueIndex" json:"delivery_id"` // 稳定投递 ID
	EventType    string     `gorm:"size:64;not null;index" json:"event_type"`
	Payload      string     `gorm:"type:text;not null" json:"payload"`
	StatusCode   int        `gorm:"not null;default:0" json:"status_code"`
	ResponseBody string     `gorm:"type:text" json:"response_body"`
	Status       string     `gorm:"size:16;not null;default:pending" json:"status"` // pending / success / failed
	RetryCount   int        `gorm:"not null;default:0" json:"retry_count"`
	NextRetryAt  *time.Time `json:"next_retry_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (WebhookDelivery) TableName() string { return "webhook_deliveries" }

// CustomFieldDef 自定义字段定义。
// Module 标识所属业务模块（如 user / project），FieldCode 在同模块内唯一。
// FieldType 决定前端渲染方式与值序列化规则：
//   - text / textarea / number / date / datetime：value 存字面量字符串
//   - select：value 存选中字典项 code；DictCode 指定选项来源字典
//   - multi_select：value 存 JSON 数组字符串（如 ["a","b"]）
type CustomFieldDef struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Module      string    `gorm:"size:64;not null;index:idx_custom_field_def" json:"module"`
	FieldCode   string    `gorm:"size:64;not null;index:idx_custom_field_def" json:"field_code"`
	FieldName   string    `gorm:"size:128;not null" json:"field_name"`
	FieldType   string    `gorm:"size:32;not null;default:text" json:"field_type"`
	DictCode    string    `gorm:"size:64" json:"dict_code"`
	Description string    `gorm:"size:512" json:"description"`
	Required    int       `gorm:"not null;default:0" json:"required"`
	SortOrder   int       `gorm:"not null;default:0" json:"sort_order"`
	Enabled     int       `gorm:"not null;default:1" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (CustomFieldDef) TableName() string { return "custom_field_defs" }

func (f CustomFieldDef) IsEnabled() bool { return f.Enabled != 0 }

// CustomFieldValue 自定义字段值。
// Value 按 JSON 字符串存储，解析规则由对应 CustomFieldDef.FieldType 决定。
type CustomFieldValue struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Module    string    `gorm:"size:64;not null;index:idx_custom_field_value" json:"module"`
	EntityID  uint      `gorm:"not null;index:idx_custom_field_value" json:"entity_id"`
	FieldID   uint      `gorm:"not null;index:idx_custom_field_value" json:"field_id"`
	Value     string    `gorm:"type:text" json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (CustomFieldValue) TableName() string { return "custom_field_values" }

type ScheduleRun struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	JobID      uint       `gorm:"index" json:"job_id"`
	Trigger    string     `json:"trigger"`
	Status     string     `gorm:"index" json:"status"`
	Detail     string     `json:"detail"`
	DurationMs int64      `json:"duration_ms"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type PlatformEvent struct {
	ID        string    `gorm:"size:64;primaryKey" json:"id"`
	Payload   string    `gorm:"type:text" json:"-"`
	Processed int       `gorm:"index;not null;default:0" json:"processed"`
	CreatedAt time.Time `json:"created_at"`
}
