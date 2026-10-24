# 视频 AI（M0–M6）

本文描述平台内视频 AI 管理能力的**当前落地方式**。推理**只走真实链路**：RTSP 拉流 → ffmpeg 解码 → 真实帧送入 ONNX 推理 worker → 几何规则判定。**生产路径没有任何 mock/演示适配器**；取不到新鲜帧时布控报 `offline`，不会伪造心跳或合成检测。

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
              无新鲜帧 → offline（waiting_video），不分析、不伪造
                    ↓
              resolveModel()：布控 → 业务模板 → 已发布模型版本 → adapter
                    ↓
              onnx 适配器：JPEG 解码 → letterbox → 归一化 → NCHW float32
                    ↓
              ONNX Runtime worker（独立进程，backend/workers/onnx/worker.py）
              ← 原始输出张量（float32）
                    ↓
              Go 侧后处理：框解码 → 置信度过滤 → NMS → 源坐标还原
                    ↓
              EvaluateGrabRules() + hysteresis（TriggerHoldSec / RecoverHoldSec 时序滞回）
                    ↓
              VideoEvent + Evidence(screenshot.jpg + clip.mp4)
              + AnalysisStat 日聚合 + MediaOutage 断流时长
              + Hub → WebSocket /api/video/ws 实时推送
```

接线：`backend/internal/server/server.go` → `videoai.NewManager(...)` → `Application.VideoAI.Start()`。生产仅注册 `onnx` 适配器（`Inferencer` 接口 + 注册表，`RegisteredAdapters()` 可见）；`resolveModel` 按模型版本的 `adapter` 字段选择，未绑定模板/模型或模型未发布时上报 `unknown`，**不会**静默回落任何演示推理。

帧与推理的关系是硬约束：`Inferencer.Infer` 接收解码后的 JPEG 帧；没有帧就没有推理调用。

## ONNX 推理 worker

主二进制**不链接** ONNX Runtime（无 CGO/ORT 依赖）。推理由独立 worker 进程完成：

- 协议与实现：`backend/workers/onnx/`（`worker.py` + `requirements.txt` + README）。
- 平台负责取流、JPEG 解码、letterbox、归一化、NCHW 张量构建、输出解码与 NMS；worker 只持有 ORT 会话。
- 会话按模型配置惰性启动、长驻；崩溃或 15s 超时后标记死亡，下一 tick 重启。
- **每个模型维护一个 worker 池**（默认 `min(GOMAXPROCS,8)`，`BASE_VIDEO_AI_ONNX_WORKERS` 可调）。一个 worker 一次只做一次推理，若只有单个会话，共用同一模型的多路摄像头会互相排队；池按争用惰性扩容，单路布控仍只占一个进程。
- 自检（`POST /api/algorithms/model-versions/:id/self-check`）会**真实加载模型并跑一帧样例推理**；worker 不可用时如实失败，不会伪装通过。
- 自定义 worker：设置 `BASE_VIDEO_AI_ONNX_WORKER` 指向自己的命令（需实现同一协议）。

```bash
pip install -r backend/workers/onnx/requirements.txt   # onnxruntime + numpy
```

## 性能

热路径（每帧、每 tick）已做针对性优化，`scripts/video-ai-bench.sh` 可复现这些数字：

| 环节 | 优化前 | 优化后 | 做法 |
| --- | --- | --- | --- |
| 帧预处理（1280x720 → 640x640） | 23.6 ms / 22.8 MB | **11.1 ms / 1.75 MB** | 张量与 letterbox 画布池化；像素直读 `RGBA.Pix`（原来每像素一次 `At().RGBA()` 接口调用）；缩放器按尺寸缓存，避免每帧重建 14.7 MB 临时缓冲与贡献表 |
| 帧缩放插值 | 14.4 ms | **4.5 ms** | 默认 `ApproxBiLinear`（与双线性平均差 0.68/255）；`BASE_VIDEO_AI_RESIZE=bilinear` 可切回精确核 |
| 后处理（8400 anchor） | 8657 次分配 | **55 次分配** | 每个 anchor 的分数向量改为池化复用 |
| 每 tick 数据库写入 | 5 次 UPDATE | **2 次 UPDATE** | 相机状态、`applied_revision`、overlay 仅在变化时写（overlay 剥离随帧变化的 `frame_age_ms`） |
| 每 tick 数据库读取 | 每次重查码流/边界/模型 | **带 TTL 缓存 + 变更显式失效** | 发布边界、发布/改绑模型、编辑码流时由 handler 主动失效 |
| WebSocket 广播 | 每 tick 2 条、每订阅者各序列化一次 | **每 tick 1 条、整条只序列化一次** | 无订阅者时完全不编码（0 分配） |
| 摄像头列表 / 布控列表 | N+1（51 / 201 条语句） | **2 条语句** | 码流与 runtime 行改为 `IN` 批量预取 |

其他：报表汇总把对事件表的 5 次 `COUNT` 扫描合并为 1 次；补了 `(camera_id,started_at)`、`(handle_status,started_at)`、`(day,camera_id,deployment_id)`、`(camera_id,ended_at)`、`(camera_id,enabled)` 复合索引；MJPEG 帧扫描器改为读取偏移记账（不再每帧重分配），一次 64 KB 读取不再额外拷贝；实时监控前端只重绘变化的那一格并缓存面板尺寸，去掉了每帧对全部面板的强制回流。

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

1. **安装推理 worker**：`pip install -r backend/workers/onnx/requirements.txt`（onnx + numpy）；确认 `/api/video/self-check` 的 `onnx_worker` 项通过。
2. **视频管理**登记摄像头：可绑定已登记的 NVR 接入通道，或直连 RTSP；支持「探测」拨测（RTSP TCP 连接 + NVR 通道查询）与 CSV **批量导入**（逐行反馈）。
3. **坑边管理**手动绘制折线 → 校准 → 发布。（AI 候选需要真实模型推理，不可用时接口会明确提示，不会给出演示几何。）
4. **算法管理**：内置模板 `grab_edge_guard` 绑定一个**草稿** onnx 版本；填写真实 `.onnx` 文件路径 → 自检（真实加载 + 样例推理）→ 发布。
5. **布控管理**新建布控 → 自检 → 启动；界面显示期望修订号与已应用修订号；表单暴露静音通知 / OSD / 保持时长等字段。
6. **实时监控**：布局支持 1/4/9/16 路切换（选择与布局记入 localStorage）；NVR 通道走 HLS，直连 RTSP 走 MJPEG（`/api/cameras/:id/preview.mjpg`）；Canvas 叠加坑边折线 / 抓斗轮廓 / 质心；右侧实时事件侧栏经 WebSocket 推送，断线 3s 重连。
7. **告警中心**查看合并事件，确认/关闭（关闭不改现场 `scene_status`）；证据含现场截图 `screenshot.jpg` 与事件片段 `clip.mp4`（事件前 15s + 后 10s，录像中心可查看）。
8. **统计报表**查看趋势、分析质量（ticks/normal/near/cross/unknown/offline、不可判定率）与离线时长；`GET /api/video/self-check` 输出注册适配器、推理就绪状态、ffmpeg、ingest 路数与 WS 订阅数。
9. **扩展接入**登记 GB28181/推流配置；「探测」执行真机探测（SIP OPTIONS over UDP/TCP、HTTP 探活），探测成功后才可「同步为平台摄像头」。

## 自检与长跑（M5）

- API：`GET /api/video/self-check`（摄像头数、运行布控、已发布边界/模板、待处理告警、证据目录、注册适配器、`real_inference`、`onnx_worker`、ffmpeg、ingest 路数、WS 订阅数）。
- 脚本：`scripts/video-ai-selfcheck.sh`（调 API + 打印人工长跑清单）。
- 脚本：`scripts/video-ai-soak.sh` — **四路自动化压测**，两种模式：
  - **真实模式**（`SOAK_RTSP_URL` + `SOAK_MODEL` + 可用 worker）：断言 4 路持续心跳**且分析真实帧**，停用第 1 路后其余 3 路不受影响；
  - **fail-closed 模式**（默认）：断言 4 路持续心跳、状态 `offline`、**零事件**（无帧不得伪造）；
  - 真实模式缺 worker 时退出码 3（BLOCKED），不计入通过。
- 恢复后不使用失效边界（码流变更会使已发布边界变为 `invalid`）。
- 计划任务内置动作 `evidence_cleanup`：将超过保留期的证据标记为 `expired`。

## 通知与时段

- 新事件创建时通过 EventBus 发出 `video.event.created`，站内通知管理员，并按 Webhook 端点的事件类型过滤投递。
- 布控 `schedule_cron` 为空表示始终分析；非法表达式在保存时被拒绝（422），启动前自检失败，运行中上报 `unknown`——**不会**静默按始终分析处理。
- 静音（`mute_notify`）时仍记录事件，不发通知。

## 实现对照（M0–M6）

对照表反映**当前仓库的真实状态**，不等于归档规格已全部交付；`partial` 项的具体边界见下一节「明确限制」。

| 里程碑 | 能力 | 状态 | 说明 |
| --- | --- | --- | --- |
| M0 | 菜单/权限/种子 | OK | 种子只建 onnx **草稿**版本（无模型文件不得发布）；旧库 mock 版本归档、模板改绑、framework 列纠正 |
| M1 | 摄像头 CRUD / 拨测 / CSV 导入 | OK | 分组、码流版本、拨测探测、逐行反馈的批量导入；列表码流改 `IN` 批量预取 |
| M2 | 预览 / 边界 / 布控 / 时段 | OK | HLS + MJPEG 预览、边界版本与校验、布控启停与滞回；AI 坑边候选见 M3 |
| M3 | 真实帧 + ONNX worker | OK | 主二进制无 ORT 依赖；需 `pip install` worker 与真实模型文件；**本机未安装 onnxruntime 时自检如实失败** |
| M3a | AI 坑边候选 | OK | 实时帧 + 已发布模型 + 模型含坑边类才出候选；否则明确 422/503，绝不返回写死折线 |
| M4 | 告警 / 报表 / 证据 / 录像 | OK | 事件合并与处置、截图与事件片段、报表含分析质量与离线时长；直连码流平台录制（分段 + 保留策略 + 重启恢复） |
| M5 | 自检 / 长跑 | OK | `/api/video/self-check` 14 项；soak 双模式（真机 / fail-closed），缺 worker 时 exit 3 BLOCKED |
| M6 | GB28181 / 推流接入 | partial | 信令探测（SIP OPTIONS）与媒体拨测已对接真实协议；**注册/目录/Invite 媒体会话未与真机联验** |

## 明确限制

- **无演示适配器**：mock 已从生产注册、种子与文档移除；无帧时布控报 `offline`（`analysis_status=waiting_video`），不伪造心跳、不合成检测。
- **ONNX Runtime 不在主二进制内**：推理经独立 worker（`backend/workers/onnx`）。worker 未安装时 `/api/video/self-check` 的 `onnx_worker` 项失败，模型版本自检失败——这是诚实失败，不是缺陷。当前仅验证过 CPU provider；GPU（CUDA）需在 worker 环境启用。
- **模型输出布局假设 YOLO detect**（`[1, 4+nc, N]` 或 `[1, N, 4+nc]`）；其他布局需在 `internal/videoai/postprocess.go` 扩展解码。
- **AI 坑边候选的条件**：需要实时帧 + 已发布模型 + 模型输出含「坑边/边界」类（标签含 `edge`/`border`/`boundary`/`坑边` 等）。仅有抓斗类的模型会被明确拒绝（422），不会用抓斗框猜边界；候选只是建议，仍需人工确认后保存。
- **平台录制的边界**：仅覆盖平台能自己拉流的直连码流（ffmpeg 流复制，5 分钟分段）。NVR 通道由 nvr-agent 录制，平台不重复录；`event_only` 不产生连续录像。`schedule` 模式需要 `schedule_json`（`{"windows":[{"from":"08:00","to":"18:00","days":[1,2,3,4,5]}]}`，`from > to` 表示跨零点），缺失或非法会拒绝启动而不是悄悄 24 小时连录。
- **MJPEG 预览路数上限**：并发预览连接默认 32（`BASE_VIDEO_AI_MAX_PREVIEW`）；同一摄像头的多个面板复用同一个取流解码器，超限时面板提示「超过服务端预览上限」。
- **GB28181 / 推流**：信令探测（SIP OPTIONS）与媒体拨测已对接真实协议；完整注册 / 媒体会话未与真机联验，按实际设备推进。
- **NVR 通道录像仍由 nvr-agent 执行**：平台侧录制只处理直连码流；事件片段（clip.mp4）来自取流缓冲环。
- NVR 预览/录像仍走 [nvr.md](nvr.md)；平台摄像头是统一业务实体。
- 已有库的 `admin` 角色不会自动获得新增权限点，需在角色页勾选。
- **历史库迁移**：启动时种子会把绑定到旧 mock 版本的模板改绑到 onnx 版本，并将旧 mock 版本归档；运行适配器始终以模型版本的 `adapter` 字段为准。

## 相关文档

- [功能说明](features.md) — 全平台已实现范围与「未包含 / 限制」
- [海康 NVR 接入](nvr.md)
- [ONNX worker](workers)（仓库内 `backend/workers/onnx/README.md`）
- 归档规格 [管理平台功能规格.md](archive/管理平台功能规格.md) §11
