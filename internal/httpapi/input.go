package httpapi

import (
	"task289-compverify/internal/ingest"
)

// ingestStepInput 是 HTTP 层步骤导入请求体。
type ingestStepInput struct {
	Name    string `json:"name"`
	Ordinal int    `json:"ordinal"`
}

func (in ingestStepInput) toDomain() ingest.StepInput {
	return ingest.StepInput{Name: in.Name, Ordinal: in.Ordinal}
}

// ingestEffectInput 是 HTTP 层效果导入请求体。
type ingestEffectInput struct {
	StepName    string `json:"step_name"`
	EffectKey   string `json:"effect_key"`
	Description string `json:"description"`
	Gen         int    `json:"gen"`
}

func (in ingestEffectInput) toDomain() ingest.EffectInput {
	return ingest.EffectInput{
		StepName:    in.StepName,
		EffectKey:   in.EffectKey,
		Description: in.Description,
		Gen:         in.Gen,
	}
}

// ingestCompInput 是 HTTP 层补偿导入请求体。
type ingestCompInput struct {
	EffectKey string   `json:"effect_key"`
	Action    string   `json:"action"`
	DepsOn    []string `json:"deps_on"`
	Gen       int      `json:"gen"`
}

func (in ingestCompInput) toDomain() ingest.CompensationInput {
	return ingest.CompensationInput{
		EffectKey: in.EffectKey,
		Action:    in.Action,
		DepsOn:    in.DepsOn,
		Gen:       in.Gen,
	}
}

// ingestRetryInput 是 HTTP 层重试导入请求体。
type ingestRetryInput struct {
	StepName string `json:"step_name"`
	Gen      int    `json:"gen"`
	Note     string `json:"note"`
}

func (in ingestRetryInput) toDomain() ingest.RetryInput {
	return ingest.RetryInput{StepName: in.StepName, Gen: in.Gen, Note: in.Note}
}
