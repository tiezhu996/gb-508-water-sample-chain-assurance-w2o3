package repository

import (
	"context"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/constants"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/dto"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/model"
	"gorm.io/gorm"
)

// LabSampleRepository owns all persistence operations for 实验室样本.
type LabSampleRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.LabSample], error)
	Get(context.Context, uint) (model.LabSample, error)
	GetByCode(context.Context, string) (model.LabSample, error)
	Create(context.Context, *model.LabSample) error
	Update(context.Context, uint, uint, *model.LabSample) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
	CountOpenByBatchCode(context.Context, string) (int64, error)
}

type labSampleRepository struct {
	store *Store[model.LabSample]
}

func NewLabSampleRepository(db *gorm.DB) LabSampleRepository {
	return &labSampleRepository{store: NewStore[model.LabSample](db)}
}

func (r *labSampleRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.LabSample], error) {
	return r.store.List(ctx, q)
}
func (r *labSampleRepository) Get(ctx context.Context, id uint) (model.LabSample, error) {
	return r.store.Get(ctx, id)
}
func (r *labSampleRepository) GetByCode(ctx context.Context, code string) (model.LabSample, error) {
	return r.store.GetByCode(ctx, code)
}
func (r *labSampleRepository) Create(ctx context.Context, item *model.LabSample) error {
	return r.store.Create(ctx, item)
}
func (r *labSampleRepository) Update(ctx context.Context, id, version uint, item *model.LabSample) error {
	return r.store.Update(ctx, id, version, item)
}
func (r *labSampleRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *labSampleRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}

// CountOpenByBatchCode counts samples of a batch that are not disposed yet; a
// batch may only close when this reaches zero.
func (r *labSampleRepository) CountOpenByBatchCode(ctx context.Context, batchCode string) (int64, error) {
	var total int64
	err := r.store.db.WithContext(ctx).Model(&model.LabSample{}).
		Where("batch_code = ? AND status <> ?", batchCode, string(constants.SampleStateDisposed)).
		Count(&total).Error
	return total, err
}
