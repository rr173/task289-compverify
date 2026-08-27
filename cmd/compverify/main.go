// Command compverify 是分布式事务补偿链完整性验证服务的入口。
//
// 用法：
//
//	go run ./cmd/compverify --addr :8080 --db compverify.db   # 启动长驻服务
//	go run ./cmd/compverify --smoke-test                       # 端到端自检（不启动长驻服务）
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"task289-compverify/internal/httpapi"
	"task289-compverify/internal/ingest"
	"task289-compverify/internal/service"
	"task289-compverify/internal/store"
)

func main() {
	var (
		addr      = flag.String("addr", ":8080", "HTTP listen address")
		dbPath    = flag.String("db", "compverify.db", "SQLite database path")
		smokeTest = flag.Bool("smoke-test", false, "run end-to-end self test and exit")
	)
	flag.Parse()

	// smoke 模式默认使用唯一临时库，避免污染默认库与重复运行冲突；
	// 显式 --db 时沿用指定路径（用于验证指定库的重启恢复）。
	if *smokeTest && !flagWasSet("db") {
		*dbPath = filepath.Join(os.TempDir(), fmt.Sprintf("compverify-smoke-%d.db", os.Getpid()))
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if *smokeTest {
		if err := runSmoke(st, *dbPath); err != nil {
			log.Fatalf("smoke-test failed: %v", err)
		}
		fmt.Println("SMOKE TEST PASSED")
		return
	}

	app := service.New(st)
	srv := httpapi.New(app, *addr)
	log.Fatal(srv.ListenAndServe())
}

// flagWasSet 判断某 flag 是否被显式设置。
func flagWasSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// runSmoke 执行端到端自检：
//  1. 真实创建运行、导入步骤/效果/补偿/重试；
//  2. 执行完整性验证（应发现未覆盖效果 → has_residue）；
//  3. 补充补偿后再次验证（应 closed）；
//  4. 关闭并重新打开同一数据库，验证持久化与重启恢复；
//  5. 发布快照并封存。
//
// 全部断言通过返回 nil，否则返回错误。
func runSmoke(st *store.Store, dbPath string) error {
	app := service.New(st)

	// 1. 创建运行并导入事件流。
	run, err := app.CreateRun("smoke-txn-001", "smoke test transaction")
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	runID := run.ID

	for _, s := range []struct {
		name    string
		ordinal int
	}{
		{"reserve_stock", 1},
		{"deduct_balance", 2},
		{"dispatch_order", 3},
	} {
		if _, err := app.Ingest.IngestStep(runID, stepInput(s.name, s.ordinal)); err != nil {
			return fmt.Errorf("ingest step %s: %w", s.name, err)
		}
	}
	if _, err := app.Ingest.IngestEffect(runID, effectInput("reserve_stock", "stock.reserved", "库存扣减 10 件", 1)); err != nil {
		return fmt.Errorf("ingest effect: %w", err)
	}
	if _, err := app.Ingest.IngestEffect(runID, effectInput("deduct_balance", "balance.deducted", "余额扣减 ¥100", 1)); err != nil {
		return fmt.Errorf("ingest effect: %w", err)
	}
	// 效果幂等：同 key 重复导入应返回既有记录而不报错。
	if _, err := app.Ingest.IngestEffect(runID, effectInput("deduct_balance", "balance.deducted", "余额扣减 ¥100（重放）", 1)); err != nil {
		return fmt.Errorf("idempotent effect ingest: %w", err)
	}
	// 重试：dispatch 在代次 1 失败，重试到代次 2。
	if _, err := app.Ingest.IngestRetry(runID, retryInput("dispatch_order", 1, "timeout")); err != nil {
		return fmt.Errorf("ingest retry gen1: %w", err)
	}
	if _, err := app.Ingest.IngestRetry(runID, retryInput("dispatch_order", 2, "retry after timeout")); err != nil {
		return fmt.Errorf("ingest retry gen2: %w", err)
	}

	// 2. 只补偿 stock，故意漏掉 balance → 验证应发现遗留。
	if _, err := app.Ingest.IngestCompensation(runID, compInput("stock.reserved", "release_stock", nil, 1)); err != nil {
		return fmt.Errorf("ingest comp: %w", err)
	}
	rep1, err := app.VerifyRun(runID)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if rep1.OK {
		return fmt.Errorf("expected has_residue on first verify, got OK")
	}
	runAfter1, _ := st.GetRun(runID)
	if runAfter1.Status != "has_residue" {
		return fmt.Errorf("expected run status has_residue, got %s", runAfter1.Status)
	}

	// 3. 补上 balance 的补偿 → 再次验证应 closed。
	if _, err := app.Ingest.IngestCompensation(runID, compInput("balance.deducted", "refund_balance", nil, 1)); err != nil {
		return fmt.Errorf("ingest comp2: %w", err)
	}
	rep2, err := app.VerifyRun(runID)
	if err != nil {
		return fmt.Errorf("verify2: %w", err)
	}
	if !rep2.OK {
		return fmt.Errorf("expected closed after full compensation, got %+v", rep2)
	}
	runAfter2, _ := st.GetRun(runID)
	if runAfter2.Status != "closed" {
		return fmt.Errorf("expected run status closed, got %s", runAfter2.Status)
	}

	// 4. 重启恢复：关闭并重新打开同一数据库。
	if err := st.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}
	st2, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("reopen store: %w", err)
	}
	defer st2.Close()
	app2 := service.New(st2)
	reloaded, err := st2.GetRun(runID)
	if err != nil {
		return fmt.Errorf("reload run: %w", err)
	}
	if reloaded.Status != "closed" || reloaded.StepCount != 3 {
		return fmt.Errorf("reload mismatch: status=%s steps=%d", reloaded.Status, reloaded.StepCount)
	}
	effects, err := st2.ListEffects(runID)
	if err != nil || len(effects) != 2 {
		return fmt.Errorf("reload effects: n=%d err=%v", len(effects), err)
	}
	comps, err := st2.ListCompensations(runID)
	if err != nil || len(comps) != 2 {
		return fmt.Errorf("reload comps: n=%d err=%v", len(comps), err)
	}

	// 5. 发布快照并封存（用 app2 验证重启后的完整链路）。
	draft, err := app2.Snapshot.CreateDraft(runID, "smoke snapshot")
	if err != nil {
		return fmt.Errorf("create draft: %w", err)
	}
	published, err := app2.PublishSnapshot(runID, draft.ID, "smoke closed snapshot")
	if err != nil {
		return fmt.Errorf("publish snapshot: %w", err)
	}
	if published.Status != "published" {
		return fmt.Errorf("snapshot not published: %s", published.Status)
	}
	if published.GraphDigest == "" || published.GraphDigest == "pending" {
		return fmt.Errorf("snapshot digest missing")
	}
	if err := app2.TransitionRun(runID, "sealed"); err != nil {
		return fmt.Errorf("seal run: %w", err)
	}
	// 封存后拒绝写入。
	if _, err := app2.Ingest.IngestEffect(runID, effectInput("dispatch_order", "order.dispatched", "发货", 2)); err == nil {
		return fmt.Errorf("expected write rejection on sealed run")
	}
	return nil
}

// ---- smoke helpers（使用领域类型，避免依赖 httpapi 层）----

func stepInput(name string, ordinal int) ingest.StepInput {
	return ingest.StepInput{Name: name, Ordinal: ordinal}
}

func effectInput(step, key, desc string, gen int) ingest.EffectInput {
	return ingest.EffectInput{StepName: step, EffectKey: key, Description: desc, Gen: gen}
}

func compInput(effectKey, action string, deps []string, gen int) ingest.CompensationInput {
	return ingest.CompensationInput{EffectKey: effectKey, Action: action, DepsOn: deps, Gen: gen}
}

func retryInput(step string, gen int, note string) ingest.RetryInput {
	return ingest.RetryInput{StepName: step, Gen: gen, Note: note}
}
