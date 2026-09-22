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
	if reason := s.reviewLinkBlockReason(ctx, sampleCode, methodCode); reason != "" {
		_ = s.security.Audit(ctx, actor, requestID, "release_blocked", "ResultReview", 0, "", model.ResultReviewInitialStatus, reason)
		return model.ResultReview{}, fmt.Errorf("%w: %s", ErrReleaseBlocked, reason)
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

// reviewLinkBlockReason validates the release chain when a review is drafted:
// the linked sample must exist and the selected method version must be active,
// so the signing gate can later re-check the same links ("再次核对"). It
// returns a human-readable blocking reason, or "" when the draft may proceed.
func (s *resultReviewService) reviewLinkBlockReason(ctx context.Context, sampleCode, methodCode string) string {
	if sampleCode == "" || methodCode == "" {
		return "结果复核必须关联检测样本与方法版本"
	}
	if _, err := s.samples.GetByCode(ctx, sampleCode); err != nil {
		return fmt.Sprintf("关联样本 %s 不存在", sampleCode)
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

func (s *resultReviewService) Update(ctx context.Context, id uint, input dto.UpdateResultReview, actor, requestID string) (model.ResultReview, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ResultReview{}, err
	}
	if err := validateResultReviewBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
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
		// Release gate: re-verify the chain immediately before signing. Any
		// failure rejects the signing and leaves the record untouched.
		if reason := s.signingBlockReason(ctx, current); reason != "" {
			_ = s.security.Audit(ctx, actor, requestID, "release_blocked", "ResultReview", id, current.Status, current.Status, reason)
			return model.ResultReview{}, fmt.Errorf("%w: %s", ErrReleaseBlocked, reason)
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

// signingBlockReason re-validates the release chain at signing time: the
// method version must still be active and the linked sample must still be in
// testing. The reviewer/signer distinction is enforced separately in
// validateResultReviewTransition. It returns a human-readable blocking
// reason, or "" when the signing may proceed.
func (s *resultReviewService) signingBlockReason(ctx context.Context, current model.ResultReview) string {
	method, err := s.methods.GetByCode(ctx, current.MethodCode)
	if err != nil {
		return fmt.Sprintf("签发前核对失败，检测方法 %s 不存在", current.MethodCode)
	}
	if method.Status != string(constants.AssayMethodActive) {
		return fmt.Sprintf("签发前核对失败，检测方法 %s 已失效（当前状态 %s）", method.Code, method.Status)
	}
	sample, err := s.samples.GetByCode(ctx, current.SampleCode)
	if err != nil {
		return fmt.Sprintf("签发前核对失败，关联样本 %s 不存在", current.SampleCode)
	}
	if sample.Status != string(constants.SampleStateTesting) {
		return fmt.Sprintf("签发前核对失败，样本 %s 当前状态为 %s，必须处于检测状态(testing)", sample.Code, sample.Status)
	}
	return ""
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
