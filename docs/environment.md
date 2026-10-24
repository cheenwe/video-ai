# 环境变量

所有配置项使用 `BASE_` 前缀。完整列表如下。

## 服务与数据

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BASE_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `BASE_PORT` | `8181` | HTTP 监听端口 |
| `BASE_DATA_DIR` | `./data` | 数据根目录；生产建议使用绝对路径 |
| `BASE_FRONTEND_DIR` | `../frontend/dist` | 前端静态资源目录（相对 backend 工作目录） |
| `BASE_SECRET_KEY` | 自动生成 | 会话/JWT 签名密钥，至少 32 字节；未设置时在数据目录写入 `session.key` |
| `BASE_PUBLIC_URL` | — | 对外访问根 URL，OAuth 回调等场景使用 |

## 首次初始化

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BASE_ADMIN_PASSWORD` | `ChangeMe123` | **仅空数据库**首次创建 `admin` 时使用；已有账号不会被覆盖 |

初始化完成后可移除该变量。生产环境请设置强密码，或首次登录后立即改密。

## 开发代理

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BASE_API_TARGET` | `http://127.0.0.1:8181` | Vite 开发服务器 `/api` 代理目标 |

## Docker Compose

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BASE_BIND_ADDRESS` | `127.0.0.1` | 宿主机绑定地址 |
| `BASE_PUBLIC_PORT` | `8181` | 宿主机映射端口 |
| `TZ` | `Asia/Shanghai` | 容器时区 |

`.env` 示例见项目根目录 [`.env.example`](../.env.example)。密码含 `$` 时用单引号包裹，避免 Compose 变量插值。

## 数据库（可选）

默认使用 SQLite（`{BASE_DATA_DIR}/baseplatform.db`）。对接 MySQL / PostgreSQL 时配置：

| 变量 | 说明 |
| --- | --- |
| `BASE_DB_DRIVER` | `sqlite`（默认）/ `mysql` / `postgres` |
| `BASE_DB_DSN` | 完整 DSN，优先于下方拆分字段 |
| `BASE_DB_HOST` | 主机 |
| `BASE_DB_PORT` | 端口（MySQL 默认 3306，PostgreSQL 默认 5432） |
| `BASE_DB_USER` | 用户名 |
| `BASE_DB_PASSWORD` | 密码 |
| `BASE_DB_NAME` | 库名（非 SQLite 默认 `baseplatform`） |
| `BASE_DB_SSLMODE` | SSL 模式 |
| `BASE_DB_DUMP_BIN` | `mysqldump` / `pg_dump` 可执行路径 |

外部数据库备份依赖对应原生客户端；恢复须使用独立库与原生工具，详见 [运维与备份](ops-backup.md)。

## 视频 AI（可选）

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BASE_VIDEO_AI_MAX_PREVIEW` | `32` | MJPEG 并发预览上限；超限返回 503 并在面板提示，按机器负载调整 |
| `BASE_VIDEO_AI_ONNX_WORKER` | 内建 worker | ONNX 推理 worker 命令（默认 `python3 backend/workers/onnx/worker.py`）；指向自定义命令可实现同一协议的其他 worker |
| `BASE_VIDEO_AI_ONNX_WORKERS` | `min(GOMAXPROCS, 8)` | 每个模型的 worker 进程数。一个 worker 一次只处理一次推理，多路摄像头共用同一模型时会排队；调大可并行（每个进程自带一份 ORT 会话与内存） |
| `BASE_VIDEO_AI_RESIZE` | `approx` | 帧缩放插值：`approx`（默认，快约 3 倍，与双线性平均差 0.68/255）或 `bilinear`（精确 tent 核，慢且每帧多约 16 MB 分配） |

说明：

- 视频 AI 分析**只走真实链路**：无新鲜帧时布控报 `offline`（`waiting_video`），不存在开关可以回落到演示推理（旧版 `BASE_VIDEO_AI_REQUIRE_FRAMES` 已随 mock 移除而取消，设置它无效果）。
- MJPEG 预览**复用共享的取流解码器**（同一摄像头多路面板只解码一次），上限保护的是并发连接数而不是解码进程数。
- 推理 worker 依赖 Python + onnxruntime（`pip install -r backend/workers/onnx/requirements.txt`）；`GET /api/video/self-check` 的 `onnx_worker` 项会如实报告其状态。
- 联调行为见 [视频 AI](video-ai.md)。

## 授权

| 变量 | 说明 |
| --- | --- |
| `BASE_MACHINE_ID` | 可选，覆盖商业授权指纹中的机器标识（`VA1-` 前缀；nextlicense 据此识别为 baseplatform） |
| `LICENSE_PUBLIC_KEY_PATH` | 可选，覆盖验签公钥文件路径（默认同 `nextlicense` 签发公钥） |
| `LICENSE_PUBLIC_KEY_PEM` | 可选，直接提供验签公钥 PEM 文本 |
| `LICENSE_SALT` | 可选，AES 解密盐；**勿改**，须与 nextlicense 的 `nextlicense-v1` 一致 |
| `BASE_WEBHOOK_ALLOW_PRIVATE` | 设为 `1` 时允许 Webhook 指向内网（仅用于开发/测试，生产勿开） |

本仓库**只验签、不签发**：内嵌/跟踪的仅为 `backend/internal/license/public.pem`。签发私钥仅存在于独立的 nextlicense 服务（`data/keys/private.pem`），不得提交到本开源仓。`.lic` 授权文件属于客户机密，也不应入库。

## 注意事项

1. 更换工作目录会改变相对路径下的 `BASE_DATA_DIR`，正式部署请固定绝对路径。
2. 迁移或恢复时须同时保留数据库、`session.key` 及数据目录中的其他文件。
3. 本表未列出的变量可能被内部测试使用，生产环境仅依赖上述公开项。
4. 安装包与 CI 产物不得包含 `private.pem`、nextlicense CLI 或客户 `.lic` 文件。