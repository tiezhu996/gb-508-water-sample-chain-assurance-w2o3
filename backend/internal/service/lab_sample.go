package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/constants"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/dto"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/model"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/repository"
	"gorm.io/gorm"
)

type LabSampleService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.LabSample], error)
	Get(context.Context, uint) (model.LabSample, error)
	Create(context.Context, dto.CreateLabSample, string, string) (model.LabSample, error)
	Update(context.Context, uint, dto.UpdateLabSample, string, string) (model.LabSample, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.LabSample, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
}

type labSampleService struct {
	repository repository.LabSampleRepository
	batches    repository.SamplingBatchRepository
	methods    repository.AssayMethodRepository
	security   SecurityService
}

func NewLabSampleService(repo repository.LabSampleRepository, batches repository.SamplingBatchRepository, methods repository.AssayMethodRepository, security SecurityService) LabSampleService {
	return &labSampleService{repository: repo, batches: batches, methods: methods, security: security}
}

func (s *labSampleService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.LabSample], error) {
	return s.repository.List(ctx, query)
}

func (s *labSampleService) Get(ctx context.Context, id uint) (model.LabSample, error) {
	return s.repository.Get(ctx, id)
}

func (s *labSampleService) Create(ctx context.Context, input dto.CreateLabSample, actor, requestID string) (model.LabSample, error) {
	if err := validateLabSampleBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.LabSample{}, err
	}
	batchCode := strings.ToUpper(strings.TrimSpace(input.BatchCode))
	methodCode := strings.ToUpper(strings.TrimSpace(input.MethodCode))
	if err := s.ensureReceivable(ctx, batchCode, methodCode); err != nil {
		return model.LabSample{}, err
	}
	item := model.LabSample{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.LabSampleInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
		BatchCode:   batchCode, MethodCode: methodCode,
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.LabSample{}, fmt.Errorf("create 实验室样本: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "LabSample", item.ID, "", item.Status, "created 实验室样本")
	return item, nil
}

func (s *labSampleService) Update(ctx context.Context, id uint, input dto.UpdateLabSample, actor, requestID string) (model.LabSample, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.LabSample{}, err
	}
	if err := validateLabSampleBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.LabSample{}, err
	}
	batchCode := strings.ToUpper(strings.TrimSpace(input.BatchCode))
	methodCode := strings.ToUpper(strings.TrimSpace(input.MethodCode))
	if err := s.ensureLinksExist(ctx, batchCode, methodCode); err != nil {
		return model.LabSample{}, err
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = strings.TrimSpace(input.Description)
	current.Facility = strings.TrimSpace(input.Facility)
	current.Owner = strings.TrimSpace(input.Owner)
	current.Category = strings.TrimSpace(input.Category)
	current.RiskLevel = input.RiskLevel
	current.MetricValue = input.MetricValue
	current.MetricUnit = strings.TrimSpace(input.MetricUnit)
	current.EffectiveAt = input.EffectiveAt.UTC()
	current.Evidence = strings.TrimSpace(input.Evidence)
	current.RelatedCode = strings.ToUpper(strings.TrimSpace(input.RelatedCode))
	current.BatchCode = batchCode
	current.MethodCode = methodCode
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.LabSample{}, fmt.Errorf("update 实验室样本: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "LabSample", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *labSampleService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (model.LabSample, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.LabSample{}, err
	}
	target := strings.TrimSpace(input.Status)
	if !constants.CanTransition(constants.LabSampleTransitions, current.Status, target) {
		return model.LabSample{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	switch target {
	case string(constants.SampleStateAccepted):
		// 样本接收放行约束：批次必须已收到且所选方法版本仍有效。
		if err := s.ensureReceivable(ctx, current.BatchCode, current.MethodCode); err != nil {
			return model.LabSample{}, err
		}
	case string(constants.SampleStateDisposed):
		// 样本处置必须填写原因，并快照当时使用的方法与所属批次。
		if strings.TrimSpace(input.Reason) == "" {
			return model.LabSample{}, fmt.Errorf("%w: disposal reason is required", ErrInvalidInput)
		}
		now := time.Now().UTC()
		current.DisposedReason = strings.TrimSpace(input.Reason)
		current.DisposedAt = &now
		current.DisposedMethodCode = current.MethodCode
		current.DisposedBatchCode = current.BatchCode
	}
	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.LabSample{}, fmt.Errorf("transition 实验室样本: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "LabSample", id, before, target, input.Reason); err != nil {
		return model.LabSample{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, id)
}

// ensureReceivable enforces the 样本接收 release gate: the owning batch must
// already be received and the selected method version must be usable.
func (s *labSampleService) ensureReceivable(ctx context.Context, batchCode, methodCode string) error {
	batch, err := s.batches.GetByCode(ctx, batchCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: sampling batch %s does not exist", ErrReleaseBlocked, batchCode)
		}
		return err
	}
	if !constants.SamplingBatchReceived(batch.Status) {
		return fmt.Errorf("%w: sampling batch %s is %s, expected received before sample acceptance", ErrReleaseBlocked, batch.Code, batch.Status)
	}
	method, err := s.methods.GetByCode(ctx, methodCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: assay method %s does not exist", ErrReleaseBlocked, methodCode)
		}
		return err
	}
	if !constants.AssayMethodUsable(method.Status) {
		return fmt.Errorf("%w: assay method %s is %s, expected an active version", ErrReleaseBlocked, method.Code, method.Status)
	}
	return nil
}

// ensureLinksExist validates that relinked batch/method codes resolve; state
// gates are only re-checked at the acceptance transition.
func (s *labSampleService) ensureLinksExist(ctx context.Context, batchCode, methodCode string) error {
	if _, err := s.batches.GetByCode(ctx, batchCode); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: sampling batch %s does not exist", ErrInvalidInput, batchCode)
		}
		return err
	}
	if _, err := s.methods.GetByCode(ctx, methodCode); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: assay method %s does not exist", ErrInvalidInput, methodCode)
		}
		return err
	}
	return nil
}

func (s *labSampleService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "LabSample", id, current.Status, "deleted", "soft deleted 实验室样本")
}

func (s *labSampleService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateLabSampleBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
