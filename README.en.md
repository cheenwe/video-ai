# Video AI Admin Platform

> A reusable platform foundation built with **Go + Vue 3**, providing user & permission management, RBAC, data scoping, dictionaries, scheduled tasks, logs, notifications, storage and commercial licensing. It serves as an extensible administration backend for any business system.

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/cheenwe/video-ai)](https://goreportcard.com/report/github.com/cheenwe/video-ai)

- GitHub: <https://github.com/cheenwe/video-ai>
- [中文 README](README.md) | [Documentation Index](docs/README.md)

## Overview

The base capabilities of Video AI Admin Platform are extracted from the [NextFile](https://github.com/cheenwe/nextfile) Go / Vue project, decoupled from any specific business such as video. It runs against real databases and real APIs — no mocked admin data.

> Note: the "Video AI" branding follows the migration origin convention. This open repository targets the **generic platform foundation**; the video-specific modules (cameras, grabber model, pit-edge algorithm) are not included and can be built on top of this base.

## Features

- **Auth & Sessions**: login, HttpOnly Cookie sessions, logout, profile & password change, captcha, TOTP 2FA, LDAP / OAuth login
- **Users & Departments**: create/edit/disable/delete users, role assignment, department tree with cycle protection
- **Roles & Permissions**: built-in roles, custom RBAC, action-level permission points, user-role mapping, menu/entry gating
- **Data Scoping**: projects & project members across `all` / `project` / `dept` / `dept_and_sub` / `self` dimensions
- **Lists & Config**: dictionaries, custom fields, system params, branding (name/theme/login page copy), security policy
- **Operations**: audit/login logs, API call logs, scheduled tasks, backup & restore, health checks, run overview
- **Notifications & Integration**: in-app notifications, webhook delivery (HMAC signature, exponential backoff retry)
- **Storage & Licensing**: local / S3(OSS) / FTP / SFTP / WebDAV storages; commercial licensing (community/flagship, RSA-signed license file)

## Tech Stack

| Layer | Stack |
| --- | --- |
| Backend | Go + Gin, GORM + pure-Go SQLite (MySQL/PostgreSQL opt-in), JWT, bcrypt |
| Frontend | Vue 3 + Pinia + Vite + Tabler UI + TypeScript |
| Tests | `go test` / `go vet`, `npm run typecheck` / `npm test` / `npm run build` |

## Quick Start

Requires Node.js 22+ (verified on 24) and Go 1.26.

```sh
# Frontend dev server
cd frontend
npm ci
npm run dev
```

In another terminal start the backend:

```sh
cd backend
# Optional: override the default first-run admin password ChangeMe123
export BASE_ADMIN_PASSWORD='your-own-admin-password'
go run ./cmd/baseplatform
```

Open <http://127.0.0.1:5173> as `admin` (default password `ChangeMe123`). See [docs/quickstart.md](docs/quickstart.md) for the full walkthrough.

## Documentation

| Doc | Description |
| --- | --- |
| [docs/README.md](docs/README.md) | Documentation index |
| [docs/quickstart.md](docs/quickstart.md) | Local development & first login |
| [docs/environment.md](docs/environment.md) | `BASE_*` reference |
| [docs/deployment.md](docs/deployment.md) | Docker, packages & CI |
| [docs/features.md](docs/features.md) | Implemented modules & menus |
| [docs/ops-backup.md](docs/ops-backup.md) | Backup, restore & ops constraints |

## Testing

```sh
make test
```

## Contributing & Security

- See [CONTRIBUTING.md](CONTRIBUTING.md)
- Report vulnerabilities per [SECURITY.md](SECURITY.md)
## License

Open-sourced under the [MIT License](LICENSE).