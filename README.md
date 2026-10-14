# Video AI 管理平台

> 一个基于 Go + Vue3 的通用平台基础能力管理层，已内置用户与权限、RBAC、数据范围、字典、计划任务、日志、通知、存储与商业授权等模块，可作为独立业务系统的后台底座复用。

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/cheenwe/video-ai)](https://goreportcard.com/report/github.com/cheenwe/video-ai)

- GitHub：<https://github.com/cheenwe/video-ai>
- [English README](README.en.md) | [文档索引](docs/README.md)

## 简介

Video AI 管理平台源自 [NextFile](https://github.com/cheenwe/nextfile) 的 Go / Vue 项目基础能力模块迁移，提取出与具体业务（视频）解耦的通用平台底座。项目使用真实数据库与真实 API，不依赖模拟管理数据。

> 说明：仓库中 "Video AI" 品牌与命名沿用迁移来源约定；本开放仓库面向通用基础平台能力，可自行在底座上扩展。

## 功能特性

- **认证与会话**：登录、HttpOnly Cookie 会话、退出、个人资料与密码修改、验证码、TOTP 双因素、LDAP / OAuth 登录
- **用户与部门**：用户创建/编辑/禁用/删除、管理员分配角色、部门树层级与用户归属（阻止循环层级）
- **角色与权限**：内置角色、自定义 RBAC、动作级权限点（Permission）、用户-角色关联、菜单/操作入口联动
- **数据范围**：项目（Project）与项目成员，支持 all / project / dept / dept_and_sub / self 维度
- **通用列表与配置**：字典、自定义字段、系统参数、品牌（平台名称/主题色/登录页文案）、安全策略
- **运维能力**：操作/登录/审计日志、API 调用日志、计划任务、备份恢复、健康检查、运行概览
- **通知与集成**：站内通知、Webhook 事件投递（HMAC 签名、指数退避重试）
- **存储与授权**：本地 / S3(OSS) / FTP / SFTP / WebDAV 存储源；商业授权（社区版/旗舰版，RSA 签名授权文件）

## 技术栈

| 端 | 技术 |
| --- | --- |
| 后端 | Go + Gin、GORM + 纯 Go SQLite（可对接 MySQL/PostgreSQL）、JWT、bcrypt |
| 前端 | Vue 3 + Pinia + Vite + Tabler UI + TypeScript |
| 测试 | `go test` / `go vet`、`npm run typecheck` / `npm test` / `npm run build` |

## 快速开始

依赖 Node.js 22+（验证 24）、Go 1.26（沿用原后端依赖要求；允许 Go 自动下载工具链）。

```sh
# 前端开发服务器
cd frontend
npm ci
npm run dev
```

另开终端启动后端：

```sh
cd backend
go run ./cmd/baseplatform
```

访问 <http://127.0.0.1:5173>，账号 `admin`，默认密码 `ChangeMe123`。首次登录会强制修改密码，生产环境务必更换。如需在初始化时自定义密码，可通过 `BASE_ADMIN_PASSWORD` 环境变量指定。详细步骤见 [docs/quickstart.md](docs/quickstart.md)。

## 文档

| 文档 | 说明 |
| --- | --- |
| [docs/quickstart.md](docs/quickstart.md) | 本地开发与首次登录 |
| [docs/environment.md](docs/environment.md) | `BASE_*` 配置参考 |
| [docs/deployment.md](docs/deployment.md) | Docker、二进制包与 CI |
| [docs/features.md](docs/features.md) | 已实现模块与菜单 |
| [docs/ops-backup.md](docs/ops-backup.md) | 备份恢复与运行约束 |
| [docs/README.md](docs/README.md) | 完整文档索引 |

## 测试

```sh
make test
```

- 后端：`go test ./...`、`go vet ./...`
- 前端：`npm run typecheck`、`npm run build`、`npm test`

## 贡献与安全

- 参与开发请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)
- 报告安全漏洞请遵循 [SECURITY.md](SECURITY.md) 的流程
## 许可证

本项目基于 [MIT 许可证](LICENSE) 开源。

## Docker 与 GitHub CI

支持 Docker Compose 一键构建部署、数据卷持久化及容器健康检查。GitHub Actions 自动测试并生成六平台程序包。详见 [docs/deployment.md](docs/deployment.md)。
