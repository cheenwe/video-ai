package models

import "time"

// CameraGroup 摄像头区域/分组。
type CameraGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID *uint     `gorm:"index" json:"project_id"`
	Code      string    `gorm:"size:64;not null;uniqueIndex" json:"code"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Remark    string    `gorm:"size:512" json:"remark"`
	Enabled   int       `gorm:"not null;default:1" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (CameraGroup) TableName() string { return "camera_groups" }

// Camera 平台级摄像头实体；可绑定 nvr-agent 通道或直连 RTSP。
type Camera struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Code           string    `gorm:"size:64;not null;uniqueIndex" json:"code"`
	Name           string    `gorm:"size:128;not null" json:"name"`
	ProjectID      *uint     `gorm:"index" json:"project_id"`
	GroupID        *uint     `gorm:"index" json:"group_id"`
	AccessType     string    `gorm:"size:32;not null;default:nvr_agent" json:"access_type"` // nvr_agent | rtsp | gb28181 | push
	Vendor         string    `gorm:"size:64" json:"vendor"`
	Model          string    `gorm:"size:64" json:"model"`
	Remark         string    `gorm:"size:512" json:"remark"`
	Enabled        int       `gorm:"not null;default:1" json:"enabled"`
	Archived       int       `gorm:"not null;default:0;index" json:"archived"`
	NVRAgentID     *uint     `gorm:"index" json:"nvr_agent_id"`
	AgentCameraID  string    `gorm:"size:64;index" json:"agent_camera_id"`
	DeviceStatus   string    `gorm:"size:32;not null;default:registered" json:"device_status"` // registered | offline | online
	MediaStatus    string    `gorm:"size:32;not null;default:unknown" json:"media_status"`     // unknown | online | offline | auth_failed | timeout | no_frame | decode_error
	AnalysisStatus string    `gorm:"size:32;not null;default:idle" json:"analysis_status"`     // idle | waiting_video | running | fault | paused
	LastError      string    `gorm:"size:512" json:"last_error"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Camera) TableName() string { return "cameras" }

// StreamProfile 主/子码流配置版本。
type StreamProfile struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	CameraID   uint      `gorm:"index;not null" json:"camera_id"`
	Role       string    `gorm:"size:16;not null" json:"role"` // main | sub
	URL        string    `gorm:"size:512" json:"url"`
	Protocol   string    `gorm:"size:32;not null;default:rtsp" json:"protocol"`
	Revision   int       `gorm:"not null;default:1" json:"revision"`
	FOVChanged int       `gorm:"not null;default:0" json:"fov_changed"`
	Enabled    int       `gorm:"not null;default:1" json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (StreamProfile) TableName() string { return "stream_profiles" }

// BoundaryVersion 坑边边界版本。
type BoundaryVersion struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	CameraID      uint       `gorm:"index;not null" json:"camera_id"`
	Version       int        `gorm:"not null" json:"version"`
	Status        string     `gorm:"size:32;not null;default:draft;index" json:"status"` // draft | published | invalid | archived
	Name          string     `gorm:"size:128" json:"name"`
	GeometryJSON  string     `gorm:"type:text;not null" json:"geometry_json"` // polyline/polygon + pit_side + buffer
	RefWidth      int        `gorm:"not null;default:0" json:"ref_width"`
	RefHeight     int        `gorm:"not null;default:0" json:"ref_height"`
	StreamRev     int        `gorm:"not null;default:0" json:"stream_rev"`
	Source        string     `gorm:"size:32;not null;default:manual" json:"source"` // manual | ai_candidate
	PublishedAt   *time.Time `json:"published_at"`
	PublishedBy   *uint      `json:"published_by"`
	InvalidReason string     `gorm:"size:512" json:"invalid_reason"`
	CreatedBy     *uint      `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (BoundaryVersion) TableName() string { return "boundary_versions" }

// AlgoModel 基础模型资产。
type AlgoModel struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"size:64;not null;uniqueIndex" json:"code"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Framework string    `gorm:"size:64;not null;default:onnx" json:"framework"`
	Remark    string    `gorm:"size:512" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AlgoModel) TableName() string { return "algo_models" }

// AlgoModelVersion 模型版本。
type AlgoModelVersion struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	ModelID      uint       `gorm:"index;not null" json:"model_id"`
	Version      string     `gorm:"size:64;not null" json:"version"`
	Status       string     `gorm:"size:32;not null;default:draft;index" json:"status"` // draft | validating | published | archived
	FilePath     string     `gorm:"size:512" json:"file_path"`
	Checksum     string     `gorm:"size:128" json:"checksum"`
	Adapter      string     `gorm:"size:64;not null;default:onnx" json:"adapter"` // onnx | (registered adapters)
	InputSchema  string     `gorm:"type:text" json:"input_schema"`
	OutputSchema string     `gorm:"type:text" json:"output_schema"`
	ClassesJSON  string     `gorm:"type:text" json:"classes_json"`
	Backend      string     `gorm:"size:64;not null;default:cpu" json:"backend"`
	PublishedAt  *time.Time `json:"published_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (AlgoModelVersion) TableName() string { return "algo_model_versions" }

// RuleVersion 行为规则版本。
type RuleVersion struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Code        string    `gorm:"size:64;not null;index" json:"code"` // grab_near_edge | grab_cross_boundary
	Version     string    `gorm:"size:64;not null" json:"version"`
	Name        string    `gorm:"size:128;not null" json:"name"`
	Status      string    `gorm:"size:32;not null;default:published" json:"status"`
	ConfigJSON  string    `gorm:"type:text" json:"config_json"`
	Description string    `gorm:"size:512" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (RuleVersion) TableName() string { return "rule_versions" }

// BusinessTemplate 业务算法模板（基础模型 + 规则组合）。
type BusinessTemplate struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Code           string    `gorm:"size:64;not null;uniqueIndex" json:"code"`
	Name           string    `gorm:"size:128;not null" json:"name"`
	Version        string    `gorm:"size:64;not null" json:"version"`
	Status         string    `gorm:"size:32;not null;default:published" json:"status"`
	ModelVersionID *uint     `gorm:"index" json:"model_version_id"`
	RuleCodesJSON  string    `gorm:"type:text" json:"rule_codes_json"` // ["grab_near_edge","grab_cross_boundary"]
	Description    string    `gorm:"size:512" json:"description"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (BusinessTemplate) TableName() string { return "business_templates" }

// Deployment 布控期望配置。
type Deployment struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	Name              string     `gorm:"size:128;not null" json:"name"`
	CameraID          uint       `gorm:"index;not null" json:"camera_id"`
	ProjectID         *uint      `gorm:"index" json:"project_id"`
	TemplateID        *uint      `gorm:"index" json:"template_id"`
	BoundaryVersionID *uint      `gorm:"index" json:"boundary_version_id"`
	DesiredRevision   int        `gorm:"not null;default:1" json:"desired_revision"`
	AppliedRevision   int        `gorm:"not null;default:0" json:"applied_revision"`
	Status            string     `gorm:"size:32;not null;default:draft;index" json:"status"` // draft|pending|starting|running|out_of_schedule|paused|fault|stopped
	AnalysisFPS       float64    `gorm:"not null;default:2" json:"analysis_fps"`
	ThresholdNear     float64    `gorm:"not null;default:0.15" json:"threshold_near"`
	ThresholdCross    float64    `gorm:"not null;default:0" json:"threshold_cross"`
	TriggerHoldSec    float64    `gorm:"not null;default:1" json:"trigger_hold_sec"`
	RecoverHoldSec    float64    `gorm:"not null;default:2" json:"recover_hold_sec"`
	ScheduleCron      string     `gorm:"size:128" json:"schedule_cron"` // 空=始终；cron 表达时段
	MuteNotify        int        `gorm:"not null;default:0" json:"mute_notify"`
	OSDEnabled        int        `gorm:"not null;default:1" json:"osd_enabled"`
	ConfigJSON        string     `gorm:"type:text" json:"config_json"`
	LastError         string     `gorm:"size:512" json:"last_error"`
	LastSelfCheck     string     `gorm:"type:text" json:"last_self_check"`
	StartedAt         *time.Time `json:"started_at"`
	StoppedAt         *time.Time `json:"stopped_at"`
	CreatedBy         *uint      `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (Deployment) TableName() string { return "deployments" }

// DeploymentRevision 布控历史修订。
type DeploymentRevision struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	DeploymentID uint      `gorm:"index;not null" json:"deployment_id"`
	Revision     int       `gorm:"not null" json:"revision"`
	SnapshotJSON string    `gorm:"type:text;not null" json:"snapshot_json"`
	CreatedBy    *uint     `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

func (DeploymentRevision) TableName() string { return "deployment_revisions" }

// RuntimeStatus 每路分析运行时状态。
type RuntimeStatus struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	DeploymentID uint       `gorm:"uniqueIndex;not null" json:"deployment_id"`
	CameraID     uint       `gorm:"index;not null" json:"camera_id"`
	State        string     `gorm:"size:32;not null;default:stopped" json:"state"`
	AppliedRev   int        `gorm:"not null;default:0" json:"applied_rev"`
	ActualFPS    float64    `json:"actual_fps"`
	FrameAgeMS   int64      `json:"frame_age_ms"`
	QueueDropped int64      `json:"queue_dropped"`
	Backend      string     `gorm:"size:64" json:"backend"`
	ModelLoaded  int        `gorm:"not null;default:0" json:"model_loaded"`
	LastResult   string     `gorm:"size:64" json:"last_result"` // normal|near|cross|unknown|offline
	LastError    string     `gorm:"size:512" json:"last_error"`
	// LastOverlayJSON keeps the geometry of the most recent analysis so the live
	// view can draw the boundary and detections on top of the video.
	LastOverlayJSON string `gorm:"type:text" json:"last_overlay_json"`
	UpdatedAt    time.Time  `json:"updated_at"`
	HeartbeatAt  *time.Time `json:"heartbeat_at"`
}

func (RuntimeStatus) TableName() string { return "runtime_statuses" }

// AnalysisStat is a daily per-deployment counter rollup. It backs operational
// metrics such as the undecidable rate, which cannot be derived from events
// alone (most frames never raise an event).
type AnalysisStat struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Day          string    `gorm:"size:10;not null;index" json:"day"` // YYYY-MM-DD
	CameraID     uint      `gorm:"index;not null" json:"camera_id"`
	DeploymentID uint      `gorm:"index;not null" json:"deployment_id"`
	Ticks        int64     `gorm:"not null;default:0" json:"ticks"`
	Normal       int64     `gorm:"not null;default:0" json:"normal"`
	Near         int64     `gorm:"not null;default:0" json:"near"`
	Cross        int64     `gorm:"not null;default:0" json:"cross"`
	Unknown      int64     `gorm:"not null;default:0" json:"unknown"`
	Offline      int64     `gorm:"not null;default:0" json:"offline"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (AnalysisStat) TableName() string { return "analysis_stats" }

// MediaOutage records one continuous period during which a camera produced no
// usable frames. It is opened when analysis degrades to offline/unknown and
// closed when frames resume, so offline duration is measured, not guessed.
type MediaOutage struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	CameraID    uint       `gorm:"index;not null" json:"camera_id"`
	ProjectID   *uint      `gorm:"index" json:"project_id"`
	StartedAt   time.Time  `gorm:"index;not null" json:"started_at"`
	EndedAt     *time.Time `gorm:"index" json:"ended_at"`
	DurationSec int64      `json:"duration_sec"` // 0 while still open
	Reason      string     `gorm:"size:512" json:"reason"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (MediaOutage) TableName() string { return "media_outages" }

// VideoEvent 现场分析事件（与 PlatformEvent 通知总线分离）。
type VideoEvent struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	EventKey           string     `gorm:"size:128;not null;uniqueIndex" json:"event_key"` // 稳定合并键
	CameraID           uint       `gorm:"index;not null" json:"camera_id"`
	ProjectID          *uint      `gorm:"index" json:"project_id"`
	DeploymentID       *uint      `gorm:"index" json:"deployment_id"`
	Category           string     `gorm:"size:64;not null;index" json:"category"` // grab_near_edge | grab_cross_boundary
	Level              string     `gorm:"size:32;not null;default:warning" json:"level"`
	SceneStatus        string     `gorm:"size:32;not null;default:active;index" json:"scene_status"`   // active | recovered | unknown
	HandleStatus       string     `gorm:"size:32;not null;default:pending;index" json:"handle_status"` // pending|confirmed|processing|closed
	Conclusion         string     `gorm:"size:32" json:"conclusion"`                                   // real|false_positive|unknown
	ConclusionRemark   string     `gorm:"size:1024" json:"conclusion_remark"`
	StartedAt          time.Time  `gorm:"index;not null" json:"started_at"`
	LastEvidenceAt     time.Time  `json:"last_evidence_at"`
	EndedAt            *time.Time `json:"ended_at"`
	TriggerMetricJSON  string     `gorm:"type:text" json:"trigger_metric_json"`
	ConfigSnapshotJSON string     `gorm:"type:text" json:"config_snapshot_json"`
	OverlayJSON        string     `gorm:"type:text" json:"overlay_json"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (VideoEvent) TableName() string { return "video_events" }

// EventAction 人工处置记录。
type EventAction struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	EventID    uint      `gorm:"index;not null" json:"event_id"`
	Action     string    `gorm:"size:32;not null" json:"action"` // confirm|process|close|false_positive
	FromStatus string    `gorm:"size:32" json:"from_status"`
	ToStatus   string    `gorm:"size:32" json:"to_status"`
	Remark     string    `gorm:"size:1024" json:"remark"`
	ActorID    *uint     `json:"actor_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func (EventAction) TableName() string { return "event_actions" }

// Evidence 事件证据。
type Evidence struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	EventID   uint      `gorm:"index;not null" json:"event_id"`
	Kind      string    `gorm:"size:32;not null" json:"kind"`                   // screenshot | clip | overlay
	Status    string    `gorm:"size:32;not null;default:pending" json:"status"` // pending|ready|failed|expired
	Path      string    `gorm:"size:512" json:"path"`
	MetaJSON  string    `gorm:"type:text" json:"meta_json"`
	Error     string    `gorm:"size:512" json:"error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Evidence) TableName() string { return "evidences" }

// RecordingPlan 录像计划。
type RecordingPlan struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	CameraID     uint      `gorm:"uniqueIndex;not null" json:"camera_id"`
	Mode         string    `gorm:"size:32;not null;default:off" json:"mode"` // off|schedule|continuous|event_only
	RetainDays   int       `gorm:"not null;default:7" json:"retain_days"`
	MaxGB        int       `gorm:"not null;default:50" json:"max_gb"`
	ScheduleJSON string    `gorm:"type:text" json:"schedule_json"`
	Enabled      int       `gorm:"not null;default:1" json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (RecordingPlan) TableName() string { return "recording_plans" }

// AccessDevice GB28181/推流等扩展接入设备（骨架）。
type AccessDevice struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Kind       string    `gorm:"size:32;not null;index" json:"kind"` // gb28181 | rtmp_push | http_flv | hls_in
	Code       string    `gorm:"size:64;not null;uniqueIndex" json:"code"`
	Name       string    `gorm:"size:128;not null" json:"name"`
	CameraID   *uint     `gorm:"index" json:"camera_id"`
	ConfigJSON string    `gorm:"type:text" json:"config_json"`
	Status     string    `gorm:"size:32;not null;default:registered" json:"status"`
	LastError  string    `gorm:"size:512" json:"last_error"`
	Enabled    int       `gorm:"not null;default:1" json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (AccessDevice) TableName() string { return "access_devices" }
