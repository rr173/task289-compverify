// Package ingest 负责接收外部事件：步骤、效果、补偿、重试，并做指纹幂等与合法性校验。
//
// 事件流是补偿链验证的输入源头：所有下游分析（因果图、覆盖检查、顺序验证、
// 遗留定位）都建立在 ingest 成功落库的事件之上。本包保证：
//
//  1. 效果按 (run_id, effect_key) 幂等——重复投递返回既有记录而非报错；
//  2. 重试代次严格递增——代次倒退即拒绝（GEN_REGRESS）；
//  3. 封存运行拒绝一切写入。
package ingest

import (
	"fmt"

	"task289-compverify/internal/model"
	"task289-compverify/internal/store"
)

// Service 是事件接收门面。
type Service struct {
	store *store.Store
}

// New 构造事件接收服务。
func New(s *store.Store) *Service { return &Service{store: s} }

// StepInput 是一次步骤事件的入参。
type StepInput struct {
	Name    string `json:"name"`
	Ordinal int    `json:"ordinal"`
}

// EffectInput 是一次效果记录的入参。
type EffectInput struct {
	StepName    string `json:"step_name"`
	EffectKey   string `json:"effect_key"`
	Description string `json:"description"`
	Gen         int    `json:"gen"`
}

// CompensationInput 是一次补偿边事件入参。
type CompensationInput struct {
	EffectKey string   `json:"effect_key"`
	Action    string   `json:"action"`
	DepsOn    []string `json:"deps_on"`
	Gen       int      `json:"gen"`
}

// RetryInput 是一次重试事件入参。
type RetryInput struct {
	StepName string `json:"step_name"`
	Gen      int    `json:"gen"`
	Note     string `json:"note"`
}

// IngestStep 写入一个步骤。步骤名在运行内唯一。
func (svc *Service) IngestStep(runID int64, in StepInput) (model.Step, error) {
	if err := svc.assertWritable(runID); err != nil {
		return model.Step{}, err
	}
	if in.Name == "" {
		return model.Step{}, model.ErrBadInput("step name required")
	}
	if in.Ordinal <= 0 {
		return model.Step{}, model.ErrBadInput("step ordinal must be positive")
	}
	return svc.store.CreateStep(runID, in.Name, in.Ordinal)
}

// IngestEffect 写入一条效果记录；同 run 同 effect_key 幂等返回既有记录。
func (svc *Service) IngestEffect(runID int64, in EffectInput) (model.Effect, error) {
	if err := svc.assertWritable(runID); err != nil {
		return model.Effect{}, err
	}
	if in.EffectKey == "" {
		return model.Effect{}, model.ErrBadInput("effect_key required")
	}
	if in.StepName == "" {
		return model.Effect{}, model.ErrBadInput("step_name required")
	}
	step, err := svc.store.StepByName(runID, in.StepName)
	if err != nil {
		return model.Effect{}, err
	}
	if in.Gen <= 0 {
		in.Gen = step.MaxGen
		if in.Gen == 0 {
			in.Gen = 1
		}
	}
	return svc.store.CreateEffect(runID, step.ID, in.EffectKey, in.Description, in.Gen)
}

// IngestCompensation 写入一条补偿边。
func (svc *Service) IngestCompensation(runID int64, in CompensationInput) (model.Compensation, error) {
	if err := svc.assertWritable(runID); err != nil {
		return model.Compensation{}, err
	}
	if in.Action == "" {
		return model.Compensation{}, model.ErrBadInput("compensation action required")
	}
	if in.EffectKey == "" {
		return model.Compensation{}, model.ErrBadInput("effect_key required")
	}
	eff, err := svc.store.EffectByKey(runID, in.EffectKey)
	if err != nil {
		return model.Compensation{}, model.ErrUnknownEffect()
	}
	if in.Gen <= 0 {
		in.Gen = 1
	}
	return svc.store.CreateCompensation(runID, eff.ID, in.Action, in.DepsOn, in.Gen)
}

// IngestRetry 记录一次重试事件；代次必须严格递增。
func (svc *Service) IngestRetry(runID int64, in RetryInput) (model.RetryEvent, error) {
	if err := svc.assertWritable(runID); err != nil {
		return model.RetryEvent{}, err
	}
	if in.StepName == "" {
		return model.RetryEvent{}, model.ErrBadInput("step_name required")
	}
	step, err := svc.store.StepByName(runID, in.StepName)
	if err != nil {
		return model.RetryEvent{}, err
	}
	if in.Gen <= 0 {
		return model.RetryEvent{}, model.ErrBadInput("retry gen must be positive")
	}
	ev, err := svc.store.CreateRetry(runID, step.ID, in.Gen, in.Note)
	if err != nil {
		return model.RetryEvent{}, err
	}
	return ev, nil
}

// Fingerprint 计算事件的稳定指纹（供审计日志使用）。
func (svc *Service) Fingerprint(runID int64, kind, key string) string {
	return fmt.Sprintf("%d/%s/%s", runID, kind, key)
}

// assertWritable 拒绝封存运行的写入。
func (svc *Service) assertWritable(runID int64) error {
	run, err := svc.store.GetRun(runID)
	if err != nil {
		return err
	}
	if run.Status == model.RunSealed {
		return model.ErrSealed("run is sealed")
	}
	return nil
}
