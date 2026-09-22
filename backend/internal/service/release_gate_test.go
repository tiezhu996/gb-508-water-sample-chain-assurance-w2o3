package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/config"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/dto"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/model"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// releaseGateFixture wires the real repositories and services against an
// in-memory database so the release constraints are verified end to end.
type releaseGateFixture struct {
	db      *gorm.DB
	batches SamplingBatchService
	samples LabSampleService
	reviews ResultReviewService
}

func newReleaseGateFixture(t *testing.T) releaseGateFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.AuditLog{}, &model.SamplingBatch{}, &model.LabSample{}, &model.AssayMethod{}, &model.ResultReview{}); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	batchRepo := repository.NewSamplingBatchRepository(db)
	sampleRepo := repository.NewLabSampleRepository(db)
	methodRepo := repository.NewAssayMethodRepository(db)
	reviewRepo := repository.NewResultReviewRepository(db)
	return releaseGateFixture{
		db:      db,
		batches: NewSamplingBatchService(batchRepo, sampleRepo, security),
		samples: NewLabSampleService(sampleRepo, batchRepo, methodRepo, security),
		reviews: NewResultReviewService(reviewRepo, sampleRepo, methodRepo, security),
	}
}

func (f releaseGateFixture) seedBatch(t *testing.T, code, status string) model.SamplingBatch {
	t.Helper()
	item := model.SamplingBatch{
		BaseModel: model.BaseModel{Code: code, Name: "批次" + code, Status: status, Version: 1},
		Facility:  "lab", Owner: "owner", Category: "常规", RiskLevel: "low", EffectiveAt: time.Now().UTC(),
	}
	if err := f.db.Create(&item).Error; err != nil {
		t.Fatalf("seed batch %s: %v", code, err)
	}
	return item
}

func (f releaseGateFixture) seedMethod(t *testing.T, code, status string) model.AssayMethod {
	t.Helper()
	item := model.AssayMethod{
		BaseModel: model.BaseModel{Code: code, Name: "方法" + code, Status: status, Version: 1},
		Facility:  "lab", Owner: "owner", Category: "常规", RiskLevel: "low", EffectiveAt: time.Now().UTC(),
	}
	if err := f.db.Create(&item).Error; err != nil {
		t.Fatalf("seed method %s: %v", code, err)
	}
	return item
}

func (f releaseGateFixture) createSample(t *testing.T, code, batchCode, methodCode string) (model.LabSample, error) {
	t.Helper()
	return f.samples.Create(context.Background(), dto.CreateLabSample{
		Code: code, Name: "样本" + code, Facility: "lab", Owner: "owner", Category: "常规",
		RiskLevel: "low", EffectiveAt: time.Now().UTC(), BatchCode: batchCode, MethodCode: methodCode,
	}, "tester", "req")
}

func (f releaseGateFixture) setSampleStatus(t *testing.T, code, status string) {
	t.Helper()
	if err := f.db.Model(&model.LabSample{}).Where("code = ?", code).Update("status", status).Error; err != nil {
		t.Fatalf("force sample %s to %s: %v", code, status, err)
	}
}

func (f releaseGateFixture) setMethodStatus(t *testing.T, code, status string) {
	t.Helper()
	if err := f.db.Model(&model.AssayMethod{}).Where("code = ?", code).Update("status", status).Error; err != nil {
		t.Fatalf("force method %s to %s: %v", code, status, err)
	}
}

func TestSampleReceptionRequiresReceivedBatchAndActiveMethod(t *testing.T) {
	f := newReleaseGateFixture(t)
	f.seedBatch(t, "SB-T1", "collecting")
	f.seedMethod(t, "AM-T1", "draft")

	if _, err := f.createSample(t, "LS-T1", "SB-T1", "AM-T1"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected block when batch is not received, got %v", err)
	}
	f.seedBatch(t, "SB-T2", "received")
	if _, err := f.createSample(t, "LS-T2", "SB-T2", "AM-T1"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected block when method is not active, got %v", err)
	}
	f.setMethodStatus(t, "AM-T1", "active")
	if _, err := f.createSample(t, "LS-T3", "SB-T2", "AM-T1"); err != nil {
		t.Fatalf("expected reception to pass with received batch and active method, got %v", err)
	}
}

func TestBatchCloseBlockedUntilSamplesDisposed(t *testing.T) {
	f := newReleaseGateFixture(t)
	ctx := context.Background()
	batch := f.seedBatch(t, "SB-C1", "received")
	f.seedMethod(t, "AM-C1", "active")
	sample, err := f.createSample(t, "LS-C1", "SB-C1", "AM-C1")
	if err != nil {
		t.Fatalf("seed sample: %v", err)
	}

	if _, err := f.batches.Transition(ctx, batch.ID, dto.TransitionRequest{Status: "closed", ExpectedVersion: batch.Version, Reason: "尝试关闭"}, "tester", "req"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected close to be blocked while sample undisposed, got %v", err)
	}
	current, err := f.batches.Get(ctx, batch.ID)
	if err != nil || current.Status != "received" {
		t.Fatalf("blocked close must leave the batch untouched, got status %q err %v", current.Status, err)
	}

	f.setSampleStatus(t, "LS-C1", "testing")
	if _, err := f.samples.Transition(ctx, sample.ID, dto.TransitionRequest{Status: "disposed", ExpectedVersion: sample.Version, Reason: "检测完成，留样期满处置"}, "tester", "req"); err != nil {
		t.Fatalf("dispose sample: %v", err)
	}
	latest, err := f.batches.Get(ctx, batch.ID)
	if err != nil {
		t.Fatalf("reload batch: %v", err)
	}
	if _, err := f.batches.Transition(ctx, batch.ID, dto.TransitionRequest{Status: "closed", ExpectedVersion: latest.Version, Reason: "全部样本已处置"}, "tester", "req"); err != nil {
		t.Fatalf("expected close to pass once samples are disposed, got %v", err)
	}
}

func TestSampleDisposalKeepsReasonMethodAndBatch(t *testing.T) {
	f := newReleaseGateFixture(t)
	ctx := context.Background()
	f.seedBatch(t, "SB-D1", "received")
	f.seedMethod(t, "AM-D1", "active")
	sample, err := f.createSample(t, "LS-D1", "SB-D1", "AM-D1")
	if err != nil {
		t.Fatalf("seed sample: %v", err)
	}
	f.setSampleStatus(t, "LS-D1", "hold")

	disposed, err := f.samples.Transition(ctx, sample.ID, dto.TransitionRequest{Status: "disposed", ExpectedVersion: sample.Version, Reason: "超过保存期，按规范处置"}, "tester", "req")
	if err != nil {
		t.Fatalf("dispose sample: %v", err)
	}
	if disposed.DisposalReason != "超过保存期，按规范处置" {
		t.Fatalf("disposal reason not preserved: %q", disposed.DisposalReason)
	}
	if disposed.DisposedBatchCode != "SB-D1" || disposed.DisposedMethodCode != "AM-D1" {
		t.Fatalf("disposal snapshot mismatch: batch %q method %q", disposed.DisposedBatchCode, disposed.DisposedMethodCode)
	}
	if disposed.DisposedAt == nil {
		t.Fatalf("disposal time not recorded")
	}
}

func TestReviewSigningRechecksMethodSampleAndSigner(t *testing.T) {
	f := newReleaseGateFixture(t)
	ctx := context.Background()
	f.seedBatch(t, "SB-R1", "received")
	f.seedMethod(t, "AM-R1", "active")
	if _, err := f.createSample(t, "LS-R1", "SB-R1", "AM-R1"); err != nil {
		t.Fatalf("seed sample: %v", err)
	}
	f.setSampleStatus(t, "LS-R1", "testing")

	createReview := func(code string) model.ResultReview {
		t.Helper()
		review, err := f.reviews.Create(ctx, dto.CreateResultReview{
			Code: code, Name: "复核" + code, Facility: "lab", Owner: "owner", Category: "常规",
			RiskLevel: "low", EffectiveAt: time.Now().UTC(), SampleCode: "LS-R1", MethodCode: "AM-R1",
		}, "tester", "req")
		if err != nil {
			t.Fatalf("create review %s: %v", code, err)
		}
		return review
	}
	submit := func(review model.ResultReview) model.ResultReview {
		t.Helper()
		updated, err := f.reviews.Transition(ctx, review.ID, dto.TransitionRequest{Status: "peer_review", ExpectedVersion: review.Version, Reason: "提交复核"}, "operator", "req")
		if err != nil {
			t.Fatalf("submit review %d: %v", review.ID, err)
		}
		return updated
	}

	// Method retired after drafting: signing must be rejected without changes.
	retired := submit(createReview("RR-R1"))
	f.setMethodStatus(t, "AM-R1", "retired")
	if _, err := f.reviews.Transition(ctx, retired.ID, dto.TransitionRequest{Status: "signed", ExpectedVersion: retired.Version, Reason: "签发"}, "reviewer", "req"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected block when method is no longer active, got %v", err)
	}
	untouched, _ := f.reviews.Get(ctx, retired.ID)
	if untouched.Status != "peer_review" || untouched.SignedBy != "" {
		t.Fatalf("blocked signing must not modify the record, got status %q signedBy %q", untouched.Status, untouched.SignedBy)
	}
	f.setMethodStatus(t, "AM-R1", "active")

	// Sample no longer in testing: signing must be rejected.
	notTesting := submit(createReview("RR-R2"))
	f.setSampleStatus(t, "LS-R1", "hold")
	if _, err := f.reviews.Transition(ctx, notTesting.ID, dto.TransitionRequest{Status: "signed", ExpectedVersion: notTesting.Version, Reason: "签发"}, "reviewer", "req"); !errors.Is(err, ErrReleaseBlocked) {
		t.Fatalf("expected block when sample is not testing, got %v", err)
	}
	f.setSampleStatus(t, "LS-R1", "testing")

	// Same person cannot sign their own submission.
	selfSign := submit(createReview("RR-R3"))
	if _, err := f.reviews.Transition(ctx, selfSign.ID, dto.TransitionRequest{Status: "signed", ExpectedVersion: selfSign.Version, Reason: "签发"}, "operator", "req"); err == nil {
		t.Fatalf("expected same-person signing to be rejected")
	}

	// All constraints satisfied: an independent reviewer signs successfully.
	ready := submit(createReview("RR-R4"))
	signed, err := f.reviews.Transition(ctx, ready.ID, dto.TransitionRequest{Status: "signed", ExpectedVersion: ready.Version, Reason: "独立签发"}, "reviewer", "req")
	if err != nil {
		t.Fatalf("expected signing to pass, got %v", err)
	}
	if signed.Status != "signed" || signed.SignedBy != "reviewer" {
		t.Fatalf("unexpected signed state: %q by %q", signed.Status, signed.SignedBy)
	}
}
