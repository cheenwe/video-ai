# 贡献指南 / Contributing

欢迎参与 Video AI 管理平台开发。We welcome contributions.

## 行为准则 / Code of Conduct

请保持友好、专业，尊重每一位参与者。请勿提交日志、凭据、密钥或任何涉及真实环境的敏感数据。

## 提交前检查 / Before Submitting

- 后端：`cd backend && go test ./... && go vet ./...`
- 前端：`npm run typecheck && npm test && npm run build`
- 保持变更聚焦：一个 Pull Request 聚焦一个问题或特性
- 新功能请补充相应测试，并在 docs 中同步说明

## 开发流程 / Workflow

1. Fork 本仓库
2. 创建特性分支：`git checkout -b feat/my-feature`
3. 本地通过上述检查并提交
4. 推送分支并提交 Pull Request 到 `main`
5. 通过 Review 与 CI 后合并

## 提交信息规范 / Commit Convention

建议使用 [Conventional Commits](https://www.conventionalcommits.org/)：`feat:`、`fix:`、`docs:`、`refactor:`、`test:`、`chore:` 等前缀。

## 分支 / Branching

`main` 分支保持可发布。新特性走特性分支；修复缺陷尽量先提供可复现用例。

## 提问 / Questions

使用 GitHub Issues 提问，附上环境（OS、Go/Vue 版本）、复现步骤与报错日志。