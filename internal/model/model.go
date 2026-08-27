// Package model 定义分布式事务补偿链完整性验证服务的核心实体与领域错误。
package model

import (
	"fmt"
	"time"
)

// Run 表示一次分布式事务运行，承载步骤效果、补偿边与重试代次的验证生命周期。
//
// 状态机：
//
//	receiving → compensating → has_residue → closed → sealed
//	    │            │  └───────────────┘
//	    │            └─→ closed（补偿覆盖完整、顺序合法时）
//	    └─→ sealed（封存后只读，禁止任何写入）
type Run struct {
	ID          int64     `json:"id"`
	TraceID     string    `json:"trace_id"` // 外部业务追踪号，唯一
	Description string    `json:"description"`
	Status      RunStatus `json:"status"`
	StepCount   int       `json:"step_count"`
	EffCount    int       `json:"effect_count"`
	CompCount   int       `json:"compensation_count"`
	RetryCount  int       `json:"retry_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RunStatus 是事务运行的合法状态。
type RunStatus string

const (
	RunReceiving     RunStatus = "receiving"     // 接收中：事件持续写入
	RunCompensating  RunStatus = "compensating"  // 补偿中：已开始完整性验证
	RunHasResidue    RunStatus = "has_residue"   // 存在遗留：发现未补偿副作用
	RunClosed        RunStatus = "closed"        // 已闭合：补偿覆盖完整且顺序合法
	RunSealed        RunStatus = "sealed"        // 封存：只读，验证结论固定
)

// ValidRunTransitions 定义运行状态机的合法迁移。
var ValidRunTransitions = map[RunStatus][]RunStatus{
	RunReceiving:    {RunCompensating, RunSealed},
	RunCompensating: {RunHasResidue, RunClosed, RunSealed},
	RunHasResidue:   {RunCompensating, RunClosed, RunSealed},
	RunClosed:       {RunSealed},
	RunSealed:       {}, // 终态
}

// CanTransition 判断 from → to 是否为合法状态迁移。
func CanTransition(from, to RunStatus) bool {
	for _, s := range ValidRunTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Step 表示事务中的一个执行步骤。同一步骤可因重试出现多个代次（generation）。
type Step struct {
	ID        int64     `json:"id"`
	RunID     int64     `json:"run_id"`
	Name      string    `json:"name"`   // 步骤名，如 "deduct_balance"
	Ordinal   int       `json:"ordinal"` // 步骤在事务内的声明顺序（1..N）
	MaxGen    int       `json:"max_gen"` // 已观测到的最大重试代次
	CreatedAt time.Time `json:"created_at"`
}

// Effect 表示步骤执行后产生的可观测副作用记录。
//
// 状态机：
//
//	pending → effective → compensated
//	    │          │
//	    └─→ duplicate（幂等去重）   └─→ residual（验证后发现未被补偿）
type Effect struct {
	ID         int64        `json:"id"`
	RunID      int64        `json:"run_id"`
	StepID     int64        `json:"step_id"`
	EffectKey  string       `json:"effect_key"`  // 效果指纹（幂等键）
	Description string      `json:"description"` // 效果描述，如 "扣减用户余额 100"
	Status     EffectStatus `json:"status"`
	Gen        int          `json:"gen"` // 产生该效果的代次
	CreatedAt  time.Time    `json:"created_at"`
}

// EffectStatus 是效果记录的合法状态。
type EffectStatus string

const (
	EffPending     EffectStatus = "pending"     // 待关联：等待补偿边
	EffEffective   EffectStatus = "effective"   // 已生效：副作用已落地
	EffCompensated EffectStatus = "compensated" // 已补偿：被补偿边覆盖
	EffDuplicate   EffectStatus = "duplicate"   // 重复：指纹与既有记录冲突
	EffResidual    EffectStatus = "residual"    // 遗留：验证发现未被补偿
)

// Compensation 表示针对某个效果的一条补偿动作（补偿边）。
//
// 状态机：
//
//	candidate → valid ─────→ confirmed
//	    │         │
//	    └─→ order_conflict    └─→ waived（工程师豁免外部不可补偿步骤）
type Compensation struct {
	ID          int64             `json:"id"`
	RunID       int64             `json:"run_id"`
	EffectID    int64             `json:"effect_id"`
	Action      string            `json:"action"`       // 补偿动作，如 "restore_balance"
	DepsOn      []string          `json:"deps_on"`      // 逆依赖：本补偿执行前必须完成的补偿动作
	Gen         int               `json:"gen"`          // 补偿所属代次
	Status      CompensationStatus `json:"status"`
	Reason      string            `json:"reason,omitempty"` // 冲突/豁免原因
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// CompensationStatus 是补偿边的合法状态。
type CompensationStatus string

const (
	CompCandidate    CompensationStatus = "candidate"       // 候选：已导入未验证
	CompValid        CompensationStatus = "valid"           // 有效：覆盖对应效果且顺序合法
	CompOrderConflict CompensationStatus = "order_conflict" // 顺序冲突：违反逆依赖顺序
	CompWaived       CompensationStatus = "waived"          // 豁免：工程师确认无需补偿
	CompConfirmed    CompensationStatus = "confirmed"       // 确认：工程师确认该补偿边
)

// RetryEvent 表示一次重试尝试。重试会提高代次并可能产生新的效果记录。
type RetryEvent struct {
	ID        int64     `json:"id"`
	RunID     int64     `json:"run_id"`
	StepID    int64     `json:"step_id"`
	Gen       int       `json:"gen"` // 重试代次，必须严格递增
	TriggeredAt time.Time `json:"triggered_at"`
	Note      string    `json:"note,omitempty"`
}

// Snapshot 表示一次验证快照：冻结因果图、覆盖结论与遗留路径。
//
// 状态机：
//
//	draft → published → superseded
//	   └─→ published（一经发布即不可变）
type Snapshot struct {
	ID          int64          `json:"id"`
	RunID       int64          `json:"run_id"`
	Version     int            `json:"version"` // 快照版本号，同一运行内递增
	Status      SnapshotStatus `json:"status"`
	Summary     string         `json:"summary"` // 验证结论摘要
	GraphDigest string         `json:"graph_digest"` // 因果图指纹
	CreatedAt   time.Time      `json:"created_at"`
	PublishedAt *time.Time     `json:"published_at,omitempty"`
}

// SnapshotStatus 是快照的合法状态。
type SnapshotStatus string

const (
	SnapDraft      SnapshotStatus = "draft"
	SnapPublished  SnapshotStatus = "published"
	SnapSuperseded SnapshotStatus = "superseded"
)

// Residue 表示验证后定位到的遗留状态路径：一条未被补偿覆盖的效果链。
type Residue struct {
	ID         int64     `json:"id"`
	RunID      int64     `json:"run_id"`
	Path       []int64   `json:"path"`        // 效果 ID 链（按依赖顺序）
	HeadEffect string    `json:"head_effect"` // 链头效果描述
	Cause      string    `json:"cause"`       // 遗留原因归类
	Resolved   bool      `json:"resolved"`
	ResolveNote string   `json:"resolve_note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// VerifyReport 是一次完整性验证的结论。
type VerifyReport struct {
	RunID          int64         `json:"run_id"`
	OK             bool          `json:"ok"`
	Covered        int           `json:"covered"`
	Uncovered      int           `json:"uncovered"`
	OrderConflicts int           `json:"order_conflicts"`
	Residues       []Residue     `json:"residues"`
	GeneratedAt    time.Time     `json:"generated_at"`
}

// 领域错误：所有错误均携带稳定代码，便于 HTTP 层映射。
type DomainError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *DomainError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// 常用领域错误构造器。
func ErrBadInput(msg string) error            { return &DomainError{Code: "BAD_INPUT", Message: msg} }
func ErrNotFound(kind string, id int64) error {
	return &DomainError{Code: "NOT_FOUND", Message: fmt.Sprintf("%s %d not found", kind, id)}
}
func ErrConflict(msg string) error            { return &DomainError{Code: "CONFLICT", Message: msg} }
func ErrSealed(msg string) error              { return &DomainError{Code: "SEALED", Message: msg} }
func ErrIllegalTransition(from, to RunStatus) error {
	return &DomainError{Code: "ILLEGAL_TRANSITION", Message: fmt.Sprintf("cannot transition %s -> %s", from, to)}
}
func ErrGenerationRegress() error {
	return &DomainError{Code: "GEN_REGRESS", Message: "retry generation must strictly increase"}
}
func ErrSelfLoopCompensation() error {
	return &DomainError{Code: "SELF_LOOP", Message: "compensation cannot depend on itself"}
}
func ErrUnknownEffect() error {
	return &DomainError{Code: "UNKNOWN_EFFECT", Message: "compensation references an unknown effect"}
}
func ErrDuplicateEvent(key string) error {
	return &DomainError{Code: "DUPLICATE_EVENT", Message: fmt.Sprintf("duplicate event fingerprint %s", key)}
}
