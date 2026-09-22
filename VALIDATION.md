# 验收记录

## 2026-09-22 放行约束补齐

- 静态检查：Go 1.22.12 下 `gofmt`、`go vet`、`go test ./...`、`go build ./...` 通过；前端 `npm run typecheck` 与 `npm run build` 通过。
- 链路模型：`LabSample` 增加 `batchCode`/`methodCode` 与处置快照（`disposedReason/disposedAt/disposedMethodCode/disposedBatchCode`）；`ResultReview` 增加 `sampleCode`/`methodCode`；种子数据含已处置样本快照。
- 运行时冒烟（SQLite，端口 17508）：
  - 样本接收：批次为 `planned` 或方法非 `active` 时创建与接收均返回 422 `release_blocked`；批次已收到且方法 `active` 时放行。
  - 批次关闭：SB-003 尚有 3 个未处置样本时关闭返回 422；SB-002 无未处置样本时 `received -> closed` 放行。
  - 样本处置：空原因被拒绝；处置后快照字段持久化，`GET /api/samples/:id` 刷新回读一致。
  - 结果签发：方法为 `draft` 或样本非 `testing` 时签发返回 422，且原记录状态、版本、签发人均不变；提交人自签仍返回 422；链路有效时独立 reviewer 签发成功并记录签发人。
- 前端：批次/样本/复核页新增"链路关系"列（`ChainRelations`）与逐条阻断原因，迁移弹窗强制填写原因（处置时默认留空），链路数据每次加载后从后端重新读取。

## 2026-08-22 初始交付

- 静态检查：Go 1.22.12 下 `go test ./...`、`go build ./...` 通过；Angular 类型检查和 Vite 生产构建通过；`docker compose config --quiet` 通过。
- 容器启动：MySQL、Redis、backend、frontend 均达到 healthy，`GET /healthz` 返回 200。
- API 流程：管理员登录、概览、4 个实体列表、创建采样批次、状态迁移、会话、脱敏运行配置、审计列表及审计汇总均通过。
- RBAC 与双人复核：验证 `draft` 不可跳过 `peer_review`；提交复核者不可自签；operator 不可签发；独立 reviewer 可签发并记录提交人、复核人和签发人。
- 内置 Browser：验证采样批次、实验室样本、检测方法、结果复核、审计 5 个页面；搜索、重置、新增、状态确认弹窗、`ChainBadge`、`ChainTimeline`、`MethodSelector` 和审计回显均正常；控制台 0 error / 0 warning。
- 规模：3045 行 Go 功能代码，38 个非测试 `.go` 文件。
- 清理：已执行 `docker compose down -v --remove-orphans`，无项目容器和数据卷残留。
