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

type ResultReviewService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.ResultReview], error)
	Get(context.Context, uint) (model.ResultReview, error)
	Create(context.Context, dto.CreateResultReview, string, string) (model.ResultReview, error)
	Update(context.Context, uint, dto.UpdateResultReview, string, string) (model.ResultReview, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.ResultReview, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
}

type resultReviewService struct {
	repository repository.ResultReviewRepository
	samples    repository.LabSampleRepository
	methods    repository.AssayMethodRepository
	security   SecurityService
}

func NewResultReviewService(repo repository.ResultReviewRepository, samples repository.LabSampleRepository, methods repository.AssayMethodRepository, security SecurityService) ResultReviewService {
	return &resultReviewService{repository: repo, samples: samples, methods: methods, security: security}
}

func (s *resultReviewService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.ResultReview], error) {
	return s.repository.List(ctx, query)
}

func (s *resultReviewService) Get(ctx context.Context, id uint) (model.ResultReview, error) {
	return s.repository.Get(ctx, id)
}

func (s *resultReviewService) Create(ctx context.Context, input dto.CreateResultReview, actor, requestID string) (model.ResultReview, error) {
	if err := validateResultReviewBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ResultReview{}, err
	}
	sampleCode := strings.ToUpper(strings.TrimSpace(input.SampleCode))
	methodCode := strings.ToUpper(strings.TrimSpace(input.MethodCode))
	if err := s.ensureLinked(ctx, sampleCode, methodCode); err != nil {
		return model.ResultReview{}, err
	}
	item := model.ResultReview{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.ResultReviewInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
		SampleCode:  sampleCode, MethodCode: methodCode,
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.ResultReview{}, fmt.Errorf("create 结果复核: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "ResultReview", item.ID, "", item.Status, "created 结果复核")
	return item, nil
}

func (s *resultReviewService) Update(ctx context.Context, id uint, input dto.UpdateResultReview, actor, requestID string) (model.ResultReview, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ResultReview{}, err
	}
	if err := validateResultReviewBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ResultReview{}, err
	}
	sampleCode := strings.ToUpper(strings.TrimSpace(input.SampleCode))
	methodCode := strings.ToUpper(strings.TrimSpace(input.MethodCode))
	if err := s.ensureLinked(ctx, sampleCode, methodCode); err != nil {
		return model.ResultReview{}, err
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
	current.SampleCode = sampleCode
	current.MethodCode = methodCode
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ResultReview{}, fmt.Errorf("update 结果复核: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "ResultReview", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *resultReviewService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (model.ResultReview, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ResultReview{}, err
	}
	target := strings.TrimSpace(input.Status)
	if err := validateResultReviewTransition(current, target, actor); err != nil {
		return model.ResultReview{}, err
	}
	if target == string(constants.ReviewStateSigned) {
		// 结果签发放行约束：签发前再次核对方法仍有效且样本处于检测状态。
		// 任一条件不满足都拒绝，且原记录不做任何改动。
		if err := s.ensureSignable(ctx, current); err != nil {
			return model.ResultReview{}, err
		}
	}
	before := current.Status
	current.Status = target
	switch target {
	case string(constants.ReviewStatePeerReview):
		current.ReviewRequestedBy = strings.TrimSpace(actor)
		current.PeerReviewedBy = ""
		current.SignedBy = ""
	case string(constants.ReviewStateSigned):
		current.PeerReviewedBy = strings.TrimSpace(actor)
		current.SignedBy = strings.TrimSpace(actor)
	case string(constants.ReviewStateDraft):
		current.ReviewRequestedBy = ""
		current.PeerReviewedBy = ""
		current.SignedBy = ""
	}
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ResultReview{}, fmt.Errorf("transition 结果复核: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "ResultReview", id, before, target, input.Reason); err != nil {
		return model.ResultReview{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, id)
}

func validateResultReviewTransition(current model.ResultReview, target, actor string) error {
	if !constants.CanTransition(constants.ResultReviewTransitions, current.Status, target) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return fmt.Errorf("%w: transition actor is required", ErrInvalidInput)
	}
	if target == string(constants.ReviewStateSigned) {
		if current.ReviewRequestedBy == "" {
			return fmt.Errorf("%w: peer-review submitter is missing", ErrInvalidInput)
		}
		if strings.EqualFold(current.ReviewRequestedBy, actor) {
			return fmt.Errorf("%w: signer must differ from peer-review submitter", ErrInvalidInput)
		}
	}
	return nil
}

// ensureLinked validates that the review references an existing sample and
// method; state gates are re-checked at signing time.
func (s *resultReviewService) ensureLinked(ctx context.Context, sampleCode, methodCode string) error {
	if _, err := s.samples.GetByCode(ctx, sampleCode); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: lab sample %s does not exist", ErrInvalidInput, sampleCode)
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

// ensureSignable re-reads the linked method and sample right before signing so
// a retired method or a sample that left the testing state blocks the release.
func (s *resultReviewService) ensureSignable(ctx context.Context, current model.ResultReview) error {
	method, err := s.methods.GetByCode(ctx, current.MethodCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: assay method %s does not exist", ErrReleaseBlocked, current.MethodCode)
		}
		return err
	}
	if !constants.AssayMethodUsable(method.Status) {
		return fmt.Errorf("%w: assay method %s is %s, signing requires an active version", ErrReleaseBlocked, method.Code, method.Status)
	}
	sample, err := s.samples.GetByCode(ctx, current.SampleCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: lab sample %s does not exist", ErrReleaseBlocked, current.SampleCode)
		}
		return err
	}
	if sample.Status != string(constants.SampleStateTesting) {
		return fmt.Errorf("%w: lab sample %s is %s, signing requires testing state", ErrReleaseBlocked, sample.Code, sample.Status)
	}
	return nil
}

func (s *resultReviewService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "ResultReview", id, current.Status, "deleted", "soft deleted 结果复核")
}

func (s *resultReviewService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateResultReviewBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
