package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/constants"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/dto"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/repository"
)

// ChainService builds the release-chain view for the workbench: linked
// entities, live constraint checks and persisted block records, so a page
// refresh can always re-read the same chain state and blocking reasons.
type ChainService interface {
	Inspect(context.Context, string, uint) (dto.ChainView, error)
}

type chainService struct {
	batches  repository.SamplingBatchRepository
	samples  repository.LabSampleRepository
	methods  repository.AssayMethodRepository
	reviews  repository.ResultReviewRepository
	security SecurityService
}

func NewChainService(batches repository.SamplingBatchRepository, samples repository.LabSampleRepository, methods repository.AssayMethodRepository, reviews repository.ResultReviewRepository, security SecurityService) ChainService {
	return &chainService{batches: batches, samples: samples, methods: methods, reviews: reviews, security: security}
}

func (s *chainService) Inspect(ctx context.Context, entityType string, id uint) (dto.ChainView, error) {
	switch strings.TrimSpace(entityType) {
	case "sampling-batches":
		return s.inspectBatch(ctx, id)
	case "samples":
		return s.inspectSample(ctx, id)
	case "reviews":
		return s.inspectReview(ctx, id)
	default:
		return dto.ChainView{}, fmt.Errorf("%w: unsupported chain entity type %q", ErrInvalidInput, entityType)
	}
}

func (s *chainService) inspectBatch(ctx context.Context, id uint) (dto.ChainView, error) {
	batch, err := s.batches.Get(ctx, id)
	if err != nil {
		return dto.ChainView{}, err
	}
	samples, err := s.samples.ListByBatch(ctx, batch.Code, 20)
	if err != nil {
		return dto.ChainView{}, fmt.Errorf("list batch samples: %w", err)
	}
	links := make([]dto.ChainLink, 0, len(samples))
	undisposed := 0
	for _, sample := range samples {
		links = append(links, dto.ChainLink{
			Kind: "sample", Code: sample.Code, Name: sample.Name, Status: sample.Status, Exists: true,
		})
		if sample.Status != string(constants.SampleStateDisposed) {
			undisposed++
		}
	}
	closeCheck := dto.ChainCheck{Key: "no_undisposed_samples", Label: "关闭前样本均已处置", Passed: undisposed == 0}
	if undisposed > 0 {
		closeCheck.Reason = fmt.Sprintf("批次仍有 %d 份未处置样本，不得关闭", undisposed)
	}
	view := dto.ChainView{
		EntityType: "SamplingBatch", EntityID: batch.ID, Code: batch.Code, Status: batch.Status,
		Links: links, Checks: []dto.ChainCheck{closeCheck},
	}
	return s.attachBlocks(ctx, view, "SamplingBatch")
}

func (s *chainService) inspectSample(ctx context.Context, id uint) (dto.ChainView, error) {
	sample, err := s.samples.Get(ctx, id)
	if err != nil {
		return dto.ChainView{}, err
	}
	batchLink := dto.ChainLink{Kind: "batch", Code: sample.BatchCode}
	batchCheck := dto.ChainCheck{Key: "batch_received", Label: "所属批次已收到"}
	if batch, err := s.batches.GetByCode(ctx, sample.BatchCode); err != nil {
		batchLink.Exists = false
		batchCheck.Reason = fmt.Sprintf("所属批次 %s 不存在", sample.BatchCode)
	} else {
		batchLink.Exists, batchLink.Name, batchLink.Status = true, batch.Name, batch.Status
		batchCheck.Passed = batch.Status == string(constants.BatchStateReceived)
		if !batchCheck.Passed {
			batchCheck.Reason = fmt.Sprintf("批次 %s 当前状态为 %s，必须已收到(received)", batch.Code, batch.Status)
		}
	}
	methodLink := dto.ChainLink{Kind: "method", Code: sample.MethodCode}
	methodCheck := dto.ChainCheck{Key: "method_active", Label: "检测方法版本有效"}
	if method, err := s.methods.GetByCode(ctx, sample.MethodCode); err != nil {
		methodLink.Exists = false
		methodCheck.Reason = fmt.Sprintf("检测方法 %s 不存在", sample.MethodCode)
	} else {
		methodLink.Exists, methodLink.Name, methodLink.Status = true, method.Name, method.Status
		methodCheck.Passed = method.Status == string(constants.AssayMethodActive)
		if !methodCheck.Passed {
			methodCheck.Reason = fmt.Sprintf("方法 %s 当前状态为 %s，必须处于有效(active)版本", method.Code, method.Status)
		}
	}
	view := dto.ChainView{
		EntityType: "LabSample", EntityID: sample.ID, Code: sample.Code, Status: sample.Status,
		Links:  []dto.ChainLink{batchLink, methodLink},
		Checks: []dto.ChainCheck{batchCheck, methodCheck},
	}
	if sample.Status == string(constants.SampleStateDisposed) {
		view.Disposal = &dto.DisposalSnapshot{
			Reason: sample.DisposalReason, BatchCode: sample.DisposedBatchCode,
			MethodCode: sample.DisposedMethodCode, DisposedAt: sample.DisposedAt,
		}
	}
	return s.attachBlocks(ctx, view, "LabSample")
}

func (s *chainService) inspectReview(ctx context.Context, id uint) (dto.ChainView, error) {
	review, err := s.reviews.Get(ctx, id)
	if err != nil {
		return dto.ChainView{}, err
	}
	sampleLink := dto.ChainLink{Kind: "sample", Code: review.SampleCode}
	sampleCheck := dto.ChainCheck{Key: "sample_testing", Label: "样本处于检测状态"}
	if sample, err := s.samples.GetByCode(ctx, review.SampleCode); err != nil {
		sampleLink.Exists = false
		sampleCheck.Reason = fmt.Sprintf("关联样本 %s 不存在", review.SampleCode)
	} else {
		sampleLink.Exists, sampleLink.Name, sampleLink.Status = true, sample.Name, sample.Status
		sampleCheck.Passed = sample.Status == string(constants.SampleStateTesting)
		if !sampleCheck.Passed {
			sampleCheck.Reason = fmt.Sprintf("样本 %s 当前状态为 %s，必须处于检测状态(testing)", sample.Code, sample.Status)
		}
	}
	methodLink := dto.ChainLink{Kind: "method", Code: review.MethodCode}
	methodCheck := dto.ChainCheck{Key: "method_active", Label: "签发时方法仍有效"}
	if method, err := s.methods.GetByCode(ctx, review.MethodCode); err != nil {
		methodLink.Exists = false
		methodCheck.Reason = fmt.Sprintf("检测方法 %s 不存在", review.MethodCode)
	} else {
		methodLink.Exists, methodLink.Name, methodLink.Status = true, method.Name, method.Status
		methodCheck.Passed = method.Status == string(constants.AssayMethodActive)
		if !methodCheck.Passed {
			methodCheck.Reason = fmt.Sprintf("方法 %s 当前状态为 %s，必须处于有效(active)版本", method.Code, method.Status)
		}
	}
	reviewerCheck := dto.ChainCheck{
		Key: "reviewer_assigned", Label: "复核人与签发人分离",
		Passed: strings.TrimSpace(review.ReviewRequestedBy) != "",
	}
	if !reviewerCheck.Passed {
		reviewerCheck.Reason = "尚未提交复核，无法核对复核人与签发人分离"
	}
	view := dto.ChainView{
		EntityType: "ResultReview", EntityID: review.ID, Code: review.Code, Status: review.Status,
		Links:  []dto.ChainLink{sampleLink, methodLink},
		Checks: []dto.ChainCheck{methodCheck, sampleCheck, reviewerCheck},
	}
	return s.attachBlocks(ctx, view, "ResultReview")
}

// attachBlocks loads persisted release_blocked audit entries so blocked
// attempts remain readable after a page refresh.
func (s *chainService) attachBlocks(ctx context.Context, view dto.ChainView, entityType string) (dto.ChainView, error) {
	history, err := s.security.EntityHistory(ctx, entityType, view.EntityID, 50)
	if err != nil {
		return dto.ChainView{}, fmt.Errorf("load release blocks: %w", err)
	}
	blocks := make([]dto.ChainBlock, 0)
	for _, entry := range history {
		if entry.Action != "release_blocked" {
			continue
		}
		blocks = append(blocks, dto.ChainBlock{
			At: entry.CreatedAt, Actor: entry.Actor, Action: entry.Action, Reason: entry.Detail,
		})
	}
	view.Blocks = blocks
	return view, nil
}
