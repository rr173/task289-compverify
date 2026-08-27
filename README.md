# task289-compverify — 分布式事务补偿链完整性验证服务

面向分布式系统工程师的补偿链验证后端服务：工程师导入事务步骤、效果记录与补偿事件，服务重建因果图、验证补偿覆盖与逆依赖顺序、检查重试代次幂等，定位未被补偿的遗留状态路径；工程师可确认或豁免补偿边、隔离重试并发布不可变验证快照。

## 业务闭环

1. 创建事务运行（记录外部追踪号 trace_id）→ 2. 导入步骤 / 效果 / 补偿 / 重试事件（效果指纹幂等、代次严格递增）→ 3. 重建效果-补偿因果图 → 4. 执行完整性验证（覆盖检查 + 逆依赖顺序检查）→ 5. 未覆盖效果展开为遗留路径，运行进入 has_residue → 6. 工程师确认/豁免补偿边后复验 → 7. 运行闭合（closed）→ 8. 发布冻结验证快照 → 9. 封存运行（sealed，拒绝写入）。

## 状态机

- 事务运行：`receiving → compensating → has_residue → closed → sealed`（验证通过直达 closed；封存为终态）
- 步骤效果：`pending → effective → compensated`；`duplicate`（幂等去重）；`residual`（验证后定位遗留）
- 补偿边：`candidate → valid / order_conflict`（验证动作自动定级）；`waived`（工程师豁免）；`confirmed`（工程师确认）
- 验证快照：`draft → published → superseded`（发布即不可变，新版本自动替代旧版）

## 标准命令

```bash
# 构建 / 静态检查 / 测试 / 端到端自检（均须真实通过）
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/compverify --smoke-test

# 启动服务
CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/compverify --addr :8080 --db compverify.db
```

## HTTP API（前缀 /api）

| 能力 | 入口 | 说明 |
|---|---|---|
| 创建运行 | POST /api/runs | trace_id、description |
| 运行列表/详情 | GET /api/runs、/api/runs/{id} | 支持 ?limit= |
| 运行状态迁移 | PATCH /api/runs/{id}/status | receiving→compensating→…→sealed |
| 导入步骤 | POST /api/runs/{id}/steps | name、ordinal |
| 导入效果 | POST /api/runs/{id}/effects | step_name、effect_key（幂等） |
| 导入补偿 | POST /api/runs/{id}/compensations | effect_key、action、deps_on |
| 导入重试 | POST /api/runs/{id}/retries | step_name、gen（严格递增） |
| 因果图 | GET /api/runs/{id}/graph | 节点 + 补偿边 + 拓扑 |
| 步骤/效果详情 | GET /api/steps/{id}、/api/effects/{id} | |
| 触发验证 | POST /api/runs/{id}/verify | 覆盖+顺序检查，自动定级 |
| 验证状态 | GET /api/runs/{id}/verify | 最近验证结论 |
| 遗留路径 | GET /api/runs/{id}/residues | 未覆盖效果链 |
| 处理遗留 | POST /api/runs/{id}/residues/{rid}/resolve | 标记已处理 |
| 确认补偿 | POST /api/compensations/{id}/confirm | 工程师确认 |
| 豁免补偿 | POST /api/compensations/{id}/waive | 外部不可补偿步骤 |
| 创建快照草稿 | POST /api/runs/{id}/snapshots | |
| 快照列表/详情 | GET /api/runs/{id}/snapshots、/api/snapshots/{id} | |
| 发布快照 | POST /api/snapshots/{id}/publish | 冻结因果图指纹 |
| 统计 | GET /api/stats | 全库规模 |
| 健康检查 | GET /api/health | |

## 持久化与重启恢复

SQLite（modernc.org/sqlite 纯 Go 驱动，CGO_ENABLED=0 可离线构建），表：runs、steps、effects、compensations、retries、snapshots、residues。

- 效果按 `(run_id, effect_key)` 幂等——重复投递返回既有记录；
- 重试按 `(run_id, step_id, gen)` 唯一，代次倒退返回 GEN_REGRESS；
- 快照按 `(run_id, version)` 唯一，发布即不可变，旧版自动替代为 superseded；
- `--smoke-test` 关闭并重开同一数据库验证恢复路径。

## 模块责任

| 包 | 职责 |
|---|---|
| internal/ingest | 事件接收：步骤/效果/补偿/重试导入，指纹幂等、代次校验、封存拒写 |
| internal/causal | 因果图重建：效果节点、补偿边、逆依赖拓扑与环检测 |
| internal/verify | 完整性验证：覆盖检查、顺序定级、遗留定位汇总 |
| internal/diagnose | 遗留诊断：未覆盖效果展开为依赖链路径 |
| internal/snapshot | 快照发布：草稿→冻结→替代，因果图指纹 |
| internal/store | SQLite 持久化：建表迁移、CRUD、聚合统计 |
| internal/service | 编排：串联验证流水线与状态迁移 |
| internal/httpapi | HTTP 层：/api 路由、错误映射、中间件 |

## 关键不变量

- `runs.trace_id`、`effects(run_id, effect_key)`、`retries(run_id, step_id, gen)`、`snapshots(run_id, version)` 唯一；
- 重试代次严格递增；补偿不允许自环；补偿边必须引用已知效果；
- 验证通过才允许进入 closed；sealed 为终态且拒绝一切写入；
- 已发布快照不可修改，新发布自动替代旧版本。
