package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/constants"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/dto"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/model"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/repository"
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
	if reason := s.receptionBlockReason(ctx, batchCode, methodCode); reason != "" {
		_ = s.security.Audit(ctx, actor, requestID, "release_blocked", "LabSample", 0, "", model.LabSampleInitialStatus, reason)
		return model.LabSample{}, fmt.Errorf("%w: %s", ErrReleaseBlocked, reason)
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

// receptionBlockReason enforces the release constraint at sample reception:
// the sampling batch must already be received and the selected method version
// must be active. It returns a human-readable blocking reason, or "" when the
// reception may proceed.
func (s *labSampleService) receptionBlockReason(ctx context.Context, batchCode, methodCode string) string {
	if batchCode == "" || methodCode == "" {
		return "样本接收必须选择所属批次与检测方法版本"
	}
	batch, err := s.batches.GetByCode(ctx, batchCode)
	if err != nil {
		return fmt.Sprintf("所属批次 %s 不存在", batchCode)
	}
	if batch.Status != string(constants.BatchStateReceived) {
		return fmt.Sprintf("批次 %s 当前状态为 %s，必须已收到(received)才能接收样本", batch.Code, batch.Status)
	}
	method, err := s.methods.GetByCode(ctx, methodCode)
	if err != nil {
		return fmt.Sprintf("检测方法 %s 不存在", methodCode)
	}
	if method.Status != string(constants.AssayMethodActive) {
		return fmt.Sprintf("检测方法 %s 当前状态为 %s，必须处于有效(active)版本", method.Code, method.Status)
	}
	return ""
}

func (s *labSampleService) Update(ctx context.Context, id uint, input dto.UpdateLabSample, actor, requestID string) (model.LabSample, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.LabSample{}, err
	}
	if err := validateLabSampleBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
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
	before := current.Status
	current.Status = target
	if target == string(constants.SampleStateDisposed) {
		// Disposal requires an explicit reason and preserves the method version
		// and batch that were in effect at that moment.
		if strings.TrimSpace(input.Reason) == "" {
			return model.LabSample{}, fmt.Errorf("%w: 样本处置必须填写处置原因", ErrInvalidInput)
		}
		now := time.Now().UTC()
		current.DisposalReason = strings.TrimSpace(input.Reason)
		current.DisposedBatchCode = current.BatchCode
		current.DisposedMethodCode = current.MethodCode
		current.DisposedAt = &now
	}
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
