package model

import "time"

// LabSample models 实验室样本 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
type LabSample struct {
	BaseModel
	Facility    string    `json:"facility" gorm:"size:120;index"`
	Owner       string    `json:"owner" gorm:"size:120;index"`
	Category    string    `json:"category" gorm:"size:80;index"`
	RiskLevel   string    `json:"riskLevel" gorm:"size:32;index"`
	MetricValue float64   `json:"metricValue"`
	MetricUnit  string    `json:"metricUnit" gorm:"size:24"`
	EffectiveAt time.Time `json:"effectiveAt"`
	Evidence    string    `json:"evidence" gorm:"size:2000"`
	RelatedCode string    `json:"relatedCode" gorm:"size:64;index"`
	// BatchCode and MethodCode link the sample into the release chain:
	// sampling batch -> lab sample -> assay method -> result review.
	BatchCode  string `json:"batchCode" gorm:"size:64;index"`
	MethodCode string `json:"methodCode" gorm:"size:64;index"`
	// Disposal snapshot fields preserve the reason, the method version and the
	// owning batch exactly as they were when the sample was disposed.
	DisposedReason     string     `json:"disposedReason" gorm:"size:500"`
	DisposedAt         *time.Time `json:"disposedAt"`
	DisposedMethodCode string     `json:"disposedMethodCode" gorm:"size:64"`
	DisposedBatchCode  string     `json:"disposedBatchCode" gorm:"size:64"`
}

func (item *LabSample) GetBase() *BaseModel { return &item.BaseModel }

func (item LabSample) TableName() string { return "lab_samples" }

var LabSampleInitialStatus = "received"
