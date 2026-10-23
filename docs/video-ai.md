# 视频 AI（M0–M6）

本文描述平台内视频 AI 管理能力的**当前落地方式**。推理适配器可插拔（进程内注册表），默认注册 `mock` 适配器用于流程验收；接口与数据模型已按规格预留 ONNX 替换点；**不得把模拟结果标为真实检测精度**。

## 架构

```text
浏览器 → 主服务 /api/*（会话 + RBAC + 审计）
         ├─ 摄像头 / 分组 / 码流配置（probe 拨测、CSV 批量导入）
         ├─ 坑边版本 / 布控启停（期望修订 vs 已应用修订）
         ├─ 算法资产（模型 / 规则 / 业务模板；发布 / 自检）
         ├─ 事件处置 / 报表 / 自检（含分析质量与离线时长运营指标）
         ├─ 扩展接入（GB28181 / 推流；真机探测 + 同步为平台摄像头）
         └─ 可选：代理 NVR 接入节点做 HLS 预览与录像
                    ↓
              videoai.Manager（进程内）
                    ↓
              每 deployment 一个 goroutine，tick 周期 = analysisInterval(AnalysisFPS)
                    ↓
         ingest.Manager：RTSP 拉流（gortsplib，H264/H265）
              → ffmpeg 子进程解码（无 CGO）→ 最新帧队列
                    ↓
              resolveModel()：布控 → 业务模板 → 已发布模型版本 → adapter
                    ↓
              Inferencer（mock 演示 / onnx 占位明确报错）
                    ↓
              EvaluateGrabRules() + hysteresis（TriggerHoldSec / RecoverHoldSec 时序滞回）
                    ↓
              VideoEvent + Evidence(screenshot.jpg + clip.mp4)
              + AnalysisStat 日聚合 + MediaOutage 断流时长
              + Hub → WebSocket /api/video/ws 实时推送
```

接线：`backend/internal/server/server.go` → `videoai.NewManager(...)` → `Application.VideoAI.Start()`。适配器通过 `videoai.RegisterAdapter` 注册，`resolveModel` 按模型版本的 `adapter` 字段选择；未绑定模板/模型时上报 `unknown`，**不会**静默回落 demo 推理。

帧接入开关：环境变量 `BASE_VIDEO_AI_REQUIRE_FRAMES=1` 时，取不到新鲜帧的布控报 `offline`（不伪造心跳、不回落 mock）。

## 菜单与权限

| 页面 | 权限 |
| --- | --- |
| 实时监控 | `camera:live` |
| 视频管理 | `camera:list/create/update/delete` |
| 坑边管理 | `boundary:list/edit/publish` |
| 算法管理 | `algorithm:list/publish` |
| 布控管理 | `deployment:list/edit/control` |
| 告警中心 | `event:list/handle` |
| 统计报表 | `report:list` |
| 录像中心 | `recording:list/download` |
| 扩展接入 | `access:list/edit` |

超级管理员拥有全部权限。新建库时内置 `admin` 角色会带上上述权限；**已有库**需在「角色权限」中为管理员勾选新增权限点（重启不会覆盖手工授权）。

## 推荐联调流程

1. **视频管理**登记摄像头：可绑定已登记的 NVR 接入通道，或直连 RTSP；支持「探测」拨测（RTSP TCP 连接 + NVR 通道查询）与 CSV **批量导入**（逐行反馈）。
2. **坑边管理**获取演示候选 → 校准折线 → 发布。（候选为演示几何，带 demo 标记。）
3. **算法管理**确认内置模板 `grab_edge_guard`；页面支持新建模型版本、发布、自检（自检报告含文件校验、classes 解析、样例推理结果）。
4. **布控管理**新建布控 → 自检 → 启动；界面显示期望修订号与已应用修订号；表单暴露静音通知 / OSD / 保持时长等字段。
5. **实时监控**：布局支持 1/4/9/16 路切换（选择与布局记入 localStorage）；NVR 通道走 HLS，直连 RTSP 走 MJPEG（`/api/cameras/:id/preview.mjpg`）；Canvas 叠加坑边折线 / 抓斗轮廓 / 质心；右侧实时事件侧栏经 WebSocket 推送，断线 3s 重连。
6. **告警中心**查看合并事件，确认/关闭（关闭不改现场 `scene_status`）；证据含现场截图 `screenshot.jpg` 与事件片段 `clip.mp4`（事件前 15s + 后 10s，录像中心可查看）。
7. **统计报表**查看趋势、分析质量（ticks/normal/near/cross/unknown/offline、不可判定率）与离线时长；`GET /api/video/self-check` 输出真实注册的适配器、ffmpeg 可用性、ingest 路数与 WebSocket 订阅数。
8. **扩展接入**登记 GB28181/推流配置；「探测」执行真机探测（SIP OPTIONS over UDP/TCP、HTTP 探活），探测成功后才可「同步为平台摄像头」。

## 自检与长跑（M5）

- API：`GET /api/video/self-check`（摄像头数、运行布控、已发布边界/模板、待处理告警、证据目录、注册适配器、ffmpeg、ingest 路数、WS 订阅数）。
- 脚本：`scripts/video-ai-selfcheck.sh`（调 API + 打印人工长跑清单）。
- 脚本：`scripts/video-ai-soak.sh` — **四路自动化压测**：登录 → 建 4 路（可复用）→ 启动 → 观察心跳 → 停用第 1 路模拟断流 → 断言其余 3 路仍运行 → 清理。需 `jq` 与可用的 ffmpeg。
- 恢复后不使用失效边界（码流变更会使已发布边界变为 `invalid`）。
- 计划任务内置动作 `evidence_cleanup`：将超过保留期的证据标记为 `expired`。

## 通知与时段

- 新事件创建时通过 EventBus 发出 `video.event.created`，站内通知管理员，并按 Webhook 端点的事件类型过滤投递。
- 布控 `schedule_cron` 为空表示始终分析；填写标准 5 段 Cron（如 `* 8-18 * * 1-5`）时，计划外进入 `out_of_schedule`，界面不得显示为正常监测。
- 静音（`mute_notify`）时仍记录事件，不发通知。

## 明确限制

- **mock 适配器**仅用于流程验收；证据、overlay、报表、自检均带或暗示 `demo` / `demo_adapter`。不得宣传为真实检测精度。
- **ONNX Runtime / GPU 会话池未接入**：`Inferencer` 接口与注册表已就绪，`adapter=onnx` 的模型版本在自检中会诚实校验文件（存在性 / 大小 / SHA-256 / classes），但默认二进制与 `go.mod` **不含** ORT；接入时应实现可替换 Inferencer / 独立 Worker，而不是声称「已支持 ONNX」。
- **自动坑边仍为演示候选**：`mock.SuggestBoundary` 返回 `ai_candidate_demo` 演示折线，真模型候选依赖 M3 之后的模型接入。
- **MJPEG 预览路数上限**：并发预览默认 32 路（每路一个 ffmpeg 解码进程），可用 `BASE_VIDEO_AI_MAX_PREVIEW` 调整；超限时面板提示「超过服务端预览上限」。
- **GB28181 / 推流**：信令探测（SIP OPTIONS）与媒体拨测已对接真实协议；完整注册 / 媒体会话未与真机联验，按实际设备推进。
- NVR 预览/录像仍走 [nvr.md](nvr.md)；平台摄像头是统一业务实体。
- 已有库的 `admin` 角色不会自动获得新增权限点，需在角色页勾选。

## 相关文档

- [功能说明](features.md) — 全平台已实现范围与「未包含 / 限制」
- [海康 NVR 接入](nvr.md)
- 归档规格 [管理平台功能规格.md](archive/管理平台功能规格.md) §11
