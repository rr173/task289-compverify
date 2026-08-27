// Package snapshot 发布不可变验证快照：固定因果图指纹与验证结论。
//
// 快照是补偿链完整性验证的最终产物：工程师确认全部补偿边后，
// 发布快照把"因果图 + 覆盖结论 + 遗留路径"冻结为不可变版本。
// 新版本发布时旧版本自动标记为 superseded，但内容绝不改写。
package snapshot

import (
	"time"

	"task289-compverify/internal/model"
	"task289-compverify/internal/store"
)

// Service 管理快照生命周期。
type Service struct {
	store *store.Store
}

// New 构造快照服务。
func New(s *store.Store) *Service { return &Service{store: s} }

// CreateDraft 创建草稿快照（版本自动递增）。
func (s *Service) CreateDraft(runID int64, summary string) (model.Snapshot, error) {
	return s.store.CreateSnapshotDraft(runID, summary, "")
}

// Publish 发布快照：写入最终图摘要与指纹后冻结。
// 同一运行既有 published 快照会被自动替代为 superseded。
func (s *Service) Publish(runID, snapID int64, summary, digest string) (model.Snapshot, error) {
	snap, err := s.store.GetSnapshot(snapID)
	if err != nil {
		return model.Snapshot{}, err
	}
	if snap.RunID != runID {
		return model.Snapshot{}, model.ErrBadInput("snapshot does not belong to run")
	}
	if summary != "" {
		if _, err := s.store.Exec(
			`UPDATE snapshots SET summary=? WHERE id=?`, summary, snapID); err != nil {
			return model.Snapshot{}, err
		}
	}
	if digest != "" {
		if _, err := s.store.Exec(
			`UPDATE snapshots SET graph_digest=? WHERE id=?`, digest, snapID); err != nil {
			return model.Snapshot{}, err
		}
	}
	return s.store.PublishSnapshot(snapID)
}

// List 列出运行的快照。
func (s *Service) List(runID int64) ([]model.Snapshot, error) {
	snaps, err := s.store.ListSnapshots(runID)
	if err != nil {
		return nil, err
	}
	return snaps, nil
}

// Get 读取单个快照。
func (s *Service) Get(id int64) (model.Snapshot, error) {
	return s.store.GetSnapshot(id)
}

// LatestPublished 返回运行最近发布的快照。
func (s *Service) LatestPublished(runID int64) (*model.Snapshot, error) {
	snaps, err := s.store.ListSnapshots(runID)
	if err != nil {
		return nil, err
	}
	for i := range snaps {
		if snaps[i].Status == model.SnapDraft {
			return &snaps[i], nil
		}
	}
	return nil, nil
}

// FrozenAt 返回快照发布时间（nil 表示未发布）。
func FrozenAt(snap model.Snapshot) *time.Time {
	return snap.PublishedAt
}
