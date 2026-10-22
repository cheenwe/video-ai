# 视频 AI（M1–M6）

本文描述平台内视频 AI 管理能力的落地方式。推理默认使用**进程内 mock 适配器**（演示），接口与数据模型已按规格预留 ONNX 替换点；**不得把模拟结果标为真实检测精度**。

## 架构

```text
浏览器 → 主服务 /api/*（会话 + RBAC + 审计）
         ├─ 摄像头 / 分组 / 码流配置
         ├─ 坑边版本 / 布控启停
         ├─ 算法资产（模型 / 规则 / 业务模板）
         ├─ 事件处置 / 报表 / 自检
         ├─ 扩展接入骨架（GB28181 / 推流）
         └─ 可选：代理 NVR 接入节点做 HLS 预览与录像
                    ↓
              videoai.Manager（mock 推理循环）
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
| 扩展接入 | `access:list/edit` |

超级管理员拥有全部权限。新建库时内置 `admin` 角色会带上上述权限；**已有库**需在「角色权限」中为管理员勾选新增权限点（重启不会覆盖手工授权）。

## 推荐联调流程

1. **视频管理**登记摄像头（可绑定已登记的 NVR 接入通道，或填 RTSP）。
2. **坑边管理**获取演示候选 → 校准折线 → 发布。
3. **算法管理**确认内置模板 `grab_edge_guard`（抓斗分割 + 临边/越界规则）。
4. **布控管理**新建布控 → 自检 → 启动；界面显示期望修订号与已应用修订号。
5. **告警中心**查看合并事件，确认/关闭（关闭不改现场 `scene_status`）。
6. **统计报表**查看趋势与 `/api/video/self-check`。
7. **扩展接入**登记 GB28181/推流配置（骨架，探测返回 `pending_adapter`）。

## 自检与长跑（M5）

- API：`GET /api/video/self-check`
- 四路持续运行 / 单路断网恢复：在部署环境启动 4 路布控，断开其中一路码流后确认：
  - 其他三路 `runtime` 仍心跳；
  - 恢复后不使用失效边界（码流变更会使已发布边界变为 `invalid`）。
- 计划任务内置动作 `evidence_cleanup`：将超过 30 天的证据标记为 `expired`。

## 通知与时段

- 新事件创建时通过 EventBus 发出 `video.event.created`，站内通知管理员，并按 Webhook 端点的事件类型过滤投递。
- 布控 `schedule_cron` 为空表示始终分析；填写标准 5 段 Cron（如 `* 8-18 * * 1-5`）时，计划外进入 `out_of_schedule`，界面不得显示为正常监测。
- 静音（`mute_notify`）时仍记录事件，不发通知。

## 明确限制

- mock 适配器仅用于流程验收，证据与报表会带 `demo: true`。
- ONNX Runtime / GPU 会话池未打包进默认二进制；接入时实现 `adapter=onnx` 的工作进程即可替换。
- GB28181 信令与推流媒体会话未对接真机，仅配置与探测骨架。
- NVR 预览/录像仍走 [nvr.md](nvr.md)；平台摄像头是统一业务实体。
- 已有库的 `admin` 角色不会自动获得新增权限点，需在角色页勾选。
