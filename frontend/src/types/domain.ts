
export interface DomainRecord {
  id: number;
  code: string;
  name: string;
  status: string;
  version: number;
  description: string;
  facility: string;
  owner: string;
  category: string;
  riskLevel: 'low' | 'medium' | 'high' | 'critical';
  metricValue: number;
  metricUnit: string;
  effectiveAt: string;
  evidence: string;
  relatedCode: string;
  batchCode?: string;
  methodCode?: string;
  sampleCode?: string;
  disposalReason?: string;
  disposedBatchCode?: string;
  disposedMethodCode?: string;
  disposedAt?: string;
  reviewRequestedBy?: string;
  peerReviewedBy?: string;
  signedBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ChainLink { kind: string; code: string; name: string; status: string; exists: boolean }
export interface ChainCheck { key: string; label: string; passed: boolean; reason: string }
export interface ChainBlock { at: string; actor: string; action: string; reason: string }
export interface DisposalSnapshot { reason: string; batchCode: string; methodCode: string; disposedAt?: string }
export interface ChainView {
  entityType: string; entityId: number; code: string; status: string;
  links: ChainLink[]; checks: ChainCheck[]; blocks: ChainBlock[]; disposal?: DisposalSnapshot;
}

export interface PageMeta { page: number; pageSize: number; total: number }
export interface ApiEnvelope<T> { data: T; error?: string; message?: string; meta?: PageMeta }
export interface UserSession { token: string; username: string; displayName: string; role: string; expiresIn: number }
export interface AuditLog {
  id: number; requestId: string; actor: string; action: string; entityType: string;
  entityId: number; beforeState: string; afterState: string; detail: string; createdAt: string;
}
export interface EntityConfig { key: string; path: string; label: string; statuses: readonly string[] }
