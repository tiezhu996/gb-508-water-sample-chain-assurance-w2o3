package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/config"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/constants"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/dto"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/model"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/repository"
	"gorm.io/gorm"
)

// fakeSecurityService satisfies SecurityService without touching persistence.
type fakeSecurityService struct{}

func (fakeSecurityService) Login(context.Context, dto.LoginRequest) (dto.LoginResponse, error) {
	return dto.LoginResponse{}, nil
}
func (fakeSecurityService) Audit(context.Context, string, string, string, string, uint, string, string, string) error {
	return nil
}
func (fakeSecurityService) ListAudits(context.Context, int, int, string) ([]model.AuditLog, int64, error) {
	return nil, 0, nil
}
func (fakeSecurityService) AuditSummary(context.Context, time.Duration) (model.AuditSummary, error) {
	return model.AuditSummary{}, nil
}
func (fakeSecurityService) EntityHistory(context.Context, string, uint, int) ([]model.AuditLog, error) {
	return nil, nil
}
func (fakeSecurityService) RuntimeConfig() config.PublicConfig { return config.PublicConfig{} }

// fakeRepository is an in-memory Store stand-in keyed by id and code.
type fakeRepository[T any] struct {
	byID   map[uint]T
	byCode map[string]T
	nextID uint
}

func newFakeRepository[T any]() *fakeRepository[T] {
	return &fakeRepository[T]{byID: map[uint]T{}, byCode: map[string]T{}, nextID: 1}
}

func baseOf[T any](item *T) *model.BaseModel {
	if record, ok := any(item).(model.DomainRecord); ok {
		return record.GetBase()
	}
	panic("fakeRepository only stores domain records")
}

func (r *fakeRepository[T]) seed(items ...T) {
	for _, item := range items {
		r.put(item)
	}
}

func (r *fakeRepository[T]) put(item T) T {
	base := baseOf(&item)
	if base.ID == 0 {
		base.ID = r.nextID
		r.nextID++
	}
	r.byID[base.ID] = item
	r.byCode[base.Code] = item
	return item
}

func (r *fakeRepository[T]) List(context.Context, dto.PageQuery) (repository.Page[T], error) {
	return repository.Page[T]{}, nil
}
func (r *fakeRepository[T]) Get(_ context.Context, id uint) (T, error) {
	item, ok := r.byID[id]
	if !ok {
		var zero T
		return zero, gorm.ErrRecordNotFound
	}
	return item, nil
}
func (r *fakeRepository[T]) GetByCode(_ context.Context, code string) (T, error) {
	item, ok := r.byCode[code]
	if !ok {
		var zero T
		return zero, gorm.ErrRecordNotFound
	}
	return item, nil
}
func (r *fakeRepository[T]) Create(_ context.Context, item *T) error {
	*item = r.put(*item)
	return nil
}
func (r *fakeRepository[T]) Update(_ context.Context, id, _ uint, item *T) error {
	if _, ok := r.byID[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	r.byID[id] = *item
	r.byCode[baseOf(item).Code] = *item
	return nil
}
func (r *fakeRepository[T]) Delete(_ context.Context, id uint) error {
	delete(r.byID, id)
	return nil
}
func (r *fakeRepository[T]) CountByStatus(context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}

type fakeSampleRepository struct {
	*fakeRepository[model.LabSample]
}

func (r *fakeSampleRepository) CountOpenByBatchCode(_ context.Context, batchCode string) (int64, error) {
	var total int64
	for _, item := range r.byCode {
		if item.BatchCode == batchCode && item.Status != string(constants.SampleStateDisposed) {
			total++
		}
	}
	return total, nil
}

type fakeBatchRepository struct {
	*fakeRepository[model.SamplingBatch]
}
type fakeMethodRepository struct {
	*fakeRepository[model.AssayMethod]
}
type fakeReviewRepository struct {
	*fakeRepository[model.ResultReview]
}

func sampleInput(code, batchCode, methodCode string) dto.CreateLabSample {
	return dto.CreateLabSample{
		Code: code, Name: "链路闸门测试样本", Facility: "验证实验室", Owner: "operator",
		Category: "常规", RiskLevel: "low", MetricUnit: "unit",
		EffectiveAt: time.Now().UTC(), BatchCode: batchCode, MethodCode: methodCode,
	}
}

func TestSampleReceptionGate(t *testing.T) {
	batches := &fakeBatchRepository{newFakeRepository[model.SamplingBatch]()}
	batches.seed(
		model.SamplingBatch{BaseModel: model.BaseModel{Code: "SB-OPEN", Status: "received"}},
		model.SamplingBatch{BaseModel: model.BaseModel{Code: "SB-PLANNED", Status: "planned"}},
	)
	methods := &fakeMethodRepository{newFakeRepository[model.AssayMethod]()}
	methods.seed(
		model.AssayMethod{BaseModel: model.BaseModel{Code: "AM-ACTIVE", Status: "active"}},
		model.AssayMethod{BaseModel: model.BaseModel{Code: "AM-DRAFT", Status: "draft"}},
	)
	samples := &fakeSampleRepository{newFakeRepository[model.LabSample]()}
	service := NewLabSampleService(samples, batches, methods, fakeSecurityService{})
	ctx := context.Background()

	if _, err := service.Create(ctx, sampleInput("LS-G1", "SB-PLANNED", "AM-ACTIVE"), "operator", "req"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected a not-received batch to block sample reception, got %v", err)
	}
	if _, err := service.Create(ctx, sampleInput("LS-G2", "SB-OPEN", "AM-DRAFT"), "operator", "req"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected an unusable method version to block sample reception, got %v", err)
	}
	created, err := service.Create(ctx, sampleInput("LS-G3", "SB-OPEN", "AM-ACTIVE"), "operator", "req")
	if err != nil {
		t.Fatalf("expected reception to pass once the gate is satisfied: %v", err)
	}

	// The method is retired after reception: acceptance must re-check and refuse.
	methods.put(model.AssayMethod{BaseModel: model.BaseModel{Code: "AM-ACTIVE", Status: "retired"}})
	if _, err := service.Transition(ctx, created.ID, dto.TransitionRequest{Status: "accepted", ExpectedVersion: created.Version, Reason: "接收样本"}, "operator", "req"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected a retired method to block acceptance, got %v", err)
	}
	current, getErr := service.Get(ctx, created.ID)
	if getErr != nil || current.Status != "received" {
		t.Fatalf("a rejected acceptance must not modify the record, got %s (%v)", current.Status, getErr)
	}

	methods.put(model.AssayMethod{BaseModel: model.BaseModel{Code: "AM-ACTIVE", Status: "active"}})
	accepted, err := service.Transition(ctx, created.ID, dto.TransitionRequest{Status: "accepted", ExpectedVersion: created.Version, Reason: "接收样本"}, "operator", "req")
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("expected acceptance once the gate passes, got %v (%s)", err, accepted.Status)
	}
}

func TestSampleDisposalSnapshotsChain(t *testing.T) {
	batches := &fakeBatchRepository{newFakeRepository[model.SamplingBatch]()}
	batches.seed(model.SamplingBatch{BaseModel: model.BaseModel{Code: "SB-OPEN", Status: "received"}})
	methods := &fakeMethodRepository{newFakeRepository[model.AssayMethod]()}
	methods.seed(model.AssayMethod{BaseModel: model.BaseModel{Code: "AM-ACTIVE", Status: "active"}})
	samples := &fakeSampleRepository{newFakeRepository[model.LabSample]()}
	samples.seed(model.LabSample{
		BaseModel: model.BaseModel{Code: "LS-D1", Status: "testing", Version: 3},
		BatchCode: "SB-OPEN", MethodCode: "AM-ACTIVE",
	})
	service := NewLabSampleService(samples, batches, methods, fakeSecurityService{})
	ctx := context.Background()

	if _, err := service.Transition(ctx, 1, dto.TransitionRequest{Status: "disposed", ExpectedVersion: 3, Reason: " "}, "operator", "req"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected an empty disposal reason to be rejected, got %v", err)
	}
	disposed, err := service.Transition(ctx, 1, dto.TransitionRequest{Status: "disposed", ExpectedVersion: 3, Reason: "保存期届满"}, "operator", "req")
	if err != nil {
		t.Fatalf("expected disposal to succeed: %v", err)
	}
	if disposed.DisposedReason != "保存期届满" || disposed.DisposedAt == nil {
		t.Fatalf("expected disposal reason and timestamp to be recorded, got %+v", disposed)
	}
	if disposed.DisposedMethodCode != "AM-ACTIVE" || disposed.DisposedBatchCode != "SB-OPEN" {
		t.Fatalf("expected the method and batch at disposal time to be snapshotted, got %+v", disposed)
	}
}

func TestBatchCloseRequiresAllSamplesDisposed(t *testing.T) {
	samples := &fakeSampleRepository{newFakeRepository[model.LabSample]()}
	samples.seed(
		model.LabSample{BaseModel: model.BaseModel{Code: "LS-C1", Status: "testing"}, BatchCode: "SB-CLOSE"},
		model.LabSample{BaseModel: model.BaseModel{Code: "LS-C2", Status: "disposed"}, BatchCode: "SB-CLOSE"},
	)
	batches := &fakeBatchRepository{newFakeRepository[model.SamplingBatch]()}
	batches.seed(model.SamplingBatch{BaseModel: model.BaseModel{Code: "SB-CLOSE", Status: "received", Version: 1}})
	service := NewSamplingBatchService(batches, samples, fakeSecurityService{})
	ctx := context.Background()

	if _, err := service.Transition(ctx, 1, dto.TransitionRequest{Status: "closed", ExpectedVersion: 1, Reason: "尝试关闭批次"}, "operator", "req"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected undisposed samples to block batch closing, got %v", err)
	}
	stored, err := samples.GetByCode(ctx, "LS-C1")
	if err != nil {
		t.Fatalf("read seeded sample: %v", err)
	}
	stored.Status = "disposed"
	samples.put(stored)
	closed, err := service.Transition(ctx, 1, dto.TransitionRequest{Status: "closed", ExpectedVersion: 1, Reason: "全部样本已处置"}, "operator", "req")
	if err != nil || closed.Status != "closed" {
		t.Fatalf("expected closing once every sample is disposed, got %v (%s)", err, closed.Status)
	}
}

func TestReviewSigningRechecksChain(t *testing.T) {
	samples := &fakeSampleRepository{newFakeRepository[model.LabSample]()}
	samples.seed(model.LabSample{BaseModel: model.BaseModel{Code: "LS-T1", Status: "testing"}})
	methods := &fakeMethodRepository{newFakeRepository[model.AssayMethod]()}
	methods.seed(model.AssayMethod{BaseModel: model.BaseModel{Code: "AM-ACTIVE", Status: "active"}})
	reviews := &fakeReviewRepository{newFakeRepository[model.ResultReview]()}
	reviews.seed(model.ResultReview{
		BaseModel:         model.BaseModel{Code: "RR-T1", Status: "peer_review", Version: 2},
		ReviewRequestedBy: "operator",
		SampleCode:        "LS-T1",
		MethodCode:        "AM-ACTIVE",
	})
	service := NewResultReviewService(reviews, samples, methods, fakeSecurityService{})
	ctx := context.Background()
	sign := dto.TransitionRequest{Status: "signed", ExpectedVersion: 2, Reason: "独立签发"}
	assertUntouched := func() {
		current, err := reviews.Get(ctx, 1)
		if err != nil {
			t.Fatalf("read review: %v", err)
		}
		if current.Status != "peer_review" || current.SignedBy != "" || current.Version != 2 {
			t.Fatalf("a rejected signing must not modify the record, got %+v", current)
		}
	}

	methods.put(model.AssayMethod{BaseModel: model.BaseModel{Code: "AM-ACTIVE", Status: "retired"}})
	if _, err := service.Transition(ctx, 1, sign, "reviewer", "req"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected a retired method to block signing, got %v", err)
	}
	assertUntouched()

	methods.put(model.AssayMethod{BaseModel: model.BaseModel{Code: "AM-ACTIVE", Status: "active"}})
	stored, err := samples.GetByCode(ctx, "LS-T1")
	if err != nil {
		t.Fatalf("read seeded sample: %v", err)
	}
	stored.Status = "hold"
	samples.put(stored)
	if _, err := service.Transition(ctx, 1, sign, "reviewer", "req"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected a non-testing sample to block signing, got %v", err)
	}
	assertUntouched()

	stored.Status = "testing"
	samples.put(stored)
	signed, err := service.Transition(ctx, 1, sign, "reviewer", "req")
	if err != nil {
		t.Fatalf("expected signing once every gate passes: %v", err)
	}
	if signed.Status != "signed" || signed.SignedBy != "reviewer" {
		t.Fatalf("expected a signed review with the signer recorded, got %+v", signed)
	}
}

func TestReviewCreateRequiresExistingChain(t *testing.T) {
	samples := &fakeSampleRepository{newFakeRepository[model.LabSample]()}
	methods := &fakeMethodRepository{newFakeRepository[model.AssayMethod]()}
	reviews := &fakeReviewRepository{newFakeRepository[model.ResultReview]()}
	service := NewResultReviewService(reviews, samples, methods, fakeSecurityService{})
	_, err := service.Create(context.Background(), dto.CreateResultReview{
		Code: "RR-X1", Name: "缺失链路的复核", Facility: "验证实验室", Owner: "operator",
		Category: "常规", RiskLevel: "low", EffectiveAt: time.Now().UTC(),
		SampleCode: "LS-MISSING", MethodCode: "AM-MISSING",
	}, "operator", "req")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected missing chain links to be rejected, got %v", err)
	}
}
