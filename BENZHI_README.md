# BENZHI 评测说明

基于 Go 实现的分布式事务补偿链完整性验证后端服务，一款后端服务，完成事务步骤效果与补偿事件导入、补偿覆盖与逆依赖顺序验证、遗留状态定位与不可变验证快照发布。

## 启动

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/compverify --addr :8080 --db compverify.db
```

## 自检（不启动长驻服务）

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/compverify --smoke-test
```

自检真实创建事务运行、导入步骤/效果/补偿/重试事件，执行两轮完整性验证（先发现遗留、补齐补偿后闭合），关闭并重新打开同一 SQLite 数据库验证持久化与重启恢复，随后发布快照并封存运行，最终以 0 退出码结束。

## 构建门禁

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...
```

## HTTP API（前缀 /api）

| 能力 | 入口 |
|---|---|
| 创建/列出/查询运行 | POST /api/runs、GET /api/runs、GET /api/runs/{id} |
| 运行状态迁移 | PATCH /api/runs/{id}/status |
| 导入步骤/效果/补偿/重试事件 | POST /api/runs/{id}/steps、/effects、/compensations、/retries |
| 因果图与步骤/效果查询 | GET /api/runs/{id}/graph、/api/steps/{id}、/api/effects/{id} |
| 完整性验证与状态 | POST/GET /api/runs/{id}/verify |
| 遗留路径与处理 | GET /api/runs/{id}/residues、POST /api/runs/{id}/residues/{rid}/resolve |
| 补偿边确认/豁免 | POST /api/compensations/{id}/confirm、/waive |
| 快照草稿/列表/发布 | POST/GET /api/runs/{id}/snapshots、POST /api/snapshots/{id}/publish、GET /api/snapshots/{id} |
| 统计与健康 | GET /api/stats、GET /api/health |

## 持久化

SQLite（modernc.org/sqlite 纯 Go 驱动，CGO 无关），表：runs、steps、effects、compensations、retries、snapshots、residues。效果按 (run_id, effect_key) 幂等，重试代次严格递增，封存运行拒绝写入，冻结快照不可改写。
