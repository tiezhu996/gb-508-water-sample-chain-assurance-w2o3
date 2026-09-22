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
	// BatchCode and MethodCode are the release-chain links established at
	// reception: the sampling batch the sample belongs to and the assay
	// method version selected for it. They stay immutable after creation.
	BatchCode  string `json:"batchCode" gorm:"size:64;index"`
	MethodCode string `json:"methodCode" gorm:"size:64;index"`
	// Disposal snapshot preserves the reason plus the method and batch that
	// were in effect when the sample was disposed, so later method or
	// batch changes cannot rewrite disposal history.
	DisposalReason     string     `json:"disposalReason" gorm:"size:500"`
	DisposedBatchCode  string     `json:"disposedBatchCode" gorm:"size:64"`
	DisposedMethodCode string     `json:"disposedMethodCode" gorm:"size:64"`
	DisposedAt         *time.Time `json:"disposedAt"`
}

func (item *LabSample) GetBase() *BaseModel { return &item.BaseModel }

func (item LabSample) TableName() string { return "lab_samples" }

var LabSampleInitialStatus = "received"
