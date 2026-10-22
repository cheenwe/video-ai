# 海康 NVR 接入

平台通过边缘 **接入节点**（运行时可访问 NVR 的现场程序）对接海康 NVR 4.0。平台负责登录、RBAC、审计与界面；接入节点负责 ISAPI / 拉流 / HLS / 本地录像。浏览器只访问平台，不会拿到节点 Token。

```
浏览器 ──(平台会话 + RBAC)──> 平台 ──(节点 Token)──> 接入节点 ──(ISAPI/RTSP)──> NVR
```

节点地址与 Token 由管理员在平台登记；Token 经 AES-GCM 加密存库（密钥由 `BASE_SECRET_KEY` 派生）。实时预览、回放与下载均由平台代理。

> 接入节点程序后续将作为独立项目维护与发布；本仓库文档只描述**平台侧**如何登记与使用。

## 平台操作

1. 菜单 **视频监控 → NVR 接入**：「登记」节点（名称、HTTP 地址、Token）→「测试连接」→ 保存。
2. 选中节点后配置设备：添加 NVR（IP / 账号 / 密码）→「连接并发现通道」→「添加为摄像头」；可测主/子码流；「保存配置」立即生效。
3. 在节点侧改 API Token 时，平台登记的 Token 需同步更新。
4. **视频监控 → 监控预览**：选摄像头预览；可启停录制、回放/下载本地录像，或按时间检索 NVR 历史并下载到节点本地。

同机部署时平台登记地址一般为 `http://127.0.0.1:8090`，Token 可按节点要求留空；分机部署时登记局域网可达地址，并填写与节点一致的 Token。可同时登记多台节点。

## 权限

| 权限点 | 作用 |
| --- | --- |
| `nvr:list` | 查看节点、测试连接 |
| `nvr:create` / `nvr:update` / `nvr:delete` | 登记、编辑、删除节点 |
| `nvr:config` | 读写节点设备配置、通道发现、码流测试 |
| `camera:list` | 摄像头列表与运行状态 |
| `camera:live` | 实时预览 |
| `camera:record` | 启停录制、发起 NVR 历史下载 |
| `recording:list` | 录像列表、在线回放、检索 NVR 录像 |
| `recording:download` | 下载录像文件 |

新安装时内置「管理员」角色已包含除 `nvr:delete` 外的上述权限；升级后的系统可在「角色权限」中补勾。节点登记/配置、录制与下载写入系统审计日志。

## 平台 API

路径前缀 `/api/nvr/`，需平台会话或个人访问令牌，信封 `{success, msg, data}`。

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET / POST | `/api/nvr/agents` | `nvr:list` / `nvr:create` |
| PUT / DELETE | `/api/nvr/agents/:id` | `nvr:update` / `nvr:delete` |
| POST | `/api/nvr/agents/test`、`/api/nvr/agents/:id/test` | `nvr:create` / `nvr:list` |
| GET / PUT | `/api/nvr/agents/:id/config` | `nvr:config` |
| POST | `/api/nvr/agents/:id/debug/nvr`、`/debug/stream` | `nvr:config` |
| GET | `/api/nvr/cameras`、`/api/nvr/agents/:id/status` | `camera:list` |
| POST | `/api/nvr/agents/:id/cameras/:cam/record/start\|stop` | `camera:record` |
| GET | `/api/nvr/agents/:id/cameras/:cam/recordings`、`/archive`、`/archive/search` | `recording:list` |
| POST | `/api/nvr/agents/:id/cameras/:cam/archive` | `camera:record` |
| GET | `/api/nvr/agents/:id/live/:cam/index.m3u8`（及分片） | `camera:live` |
| GET | `/api/nvr/agents/:id/recordings/:cam/:file`（`?download=1` 另需 `recording:download`） | `recording:list` |

节点返回 401 时，平台以 502 提示 Token 不正确，不会踢掉用户会话。

## 常见问题

- **连接被拒绝**：节点未运行，或仅监听本机而平台从其他机器访问。
- **Token 不正确**：平台登记与节点配置不一致，在节点「编辑」中重填。
- **预览黑屏**：先在设备配置中测试子码流；H.265 可勾选「预览时转 H.264」。
- **更换 `BASE_SECRET_KEY` 后**：已存 Token 无法解密，需为各节点重新填写 Token。
