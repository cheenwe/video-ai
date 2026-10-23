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
- 自检（`POST /api/algorithms/model-versions/:id/self-check`）会**真实加载模型并跑一帧样例推理**；worker 不可用时如实失败，不会伪装通过。
- 自定义 worker：设置 `BASE_VIDEO_AI_ONNX_WORKER` 指向自己的命令（需实现同一协议）。

```bash
pip install -r backend/workers/onnx/requirements.txt   # onnxruntime + numpy
```

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

## 明确限制

- **无演示适配器**：mock 已从生产注册、种子与文档移除；无帧时布控报 `offline`（`analysis_status=waiting_video`），不伪造心跳、不合成检测。
- **ONNX Runtime 不在主二进制内**：推理经独立 worker（`backend/workers/onnx`）。worker 未安装时 `/api/video/self-check` 的 `onnx_worker` 项失败，模型版本自检失败——这是诚实失败，不是缺陷。当前仅验证过 CPU provider；GPU（CUDA）需在 worker 环境启用。
- **模型输出布局假设 YOLO detect**（`[1, 4+nc, N]` 或 `[1, N, 4+nc]`）；其他布局需在 `internal/videoai/postprocess.go` 扩展解码。
- **自动坑边候选不可用**：演示折线生成器已移除；AI 候选需真实模型推理（当前接口明确返回不可用）。
- **MJPEG 预览路数上限**：并发预览默认 32 路（每路一个 ffmpeg 解码进程），可用 `BASE_VIDEO_AI_MAX_PREVIEW` 调整；超限时面板提示「超过服务端预览上限」。
- **GB28181 / 推流**：信令探测（SIP OPTIONS）与媒体拨测已对接真实协议；完整注册 / 媒体会话未与真机联验，按实际设备推进。
- **直连 RTSP 连续录像未接入**：录像计划可保存，执行依赖 nvr-agent；事件片段（clip.mp4）来自取流缓冲环。
- NVR 预览/录像仍走 [nvr.md](nvr.md)；平台摄像头是统一业务实体。
- 已有库的 `admin` 角色不会自动获得新增权限点，需在角色页勾选。
- **历史库迁移**：启动时种子会把绑定到旧 mock 版本的模板改绑到 onnx 版本，并将旧 mock 版本归档；运行适配器始终以模型版本的 `adapter` 字段为准。

## 相关文档

- [功能说明](features.md) — 全平台已实现范围与「未包含 / 限制」
- [海康 NVR 接入](nvr.md)
- [ONNX worker](workers)（仓库内 `backend/workers/onnx/README.md`）
- 归档规格 [管理平台功能规格.md](archive/管理平台功能规格.md) §11
