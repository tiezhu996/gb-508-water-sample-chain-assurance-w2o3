import type { DomainRecord } from '../types/domain';

// ChainLookup indexes the release chain (batch -> sample -> method -> review)
// by business code so pages can render relations and blocking reasons from
// fresh backend data after every reload.
export interface ChainLookup {
  batches: Record<string, DomainRecord>;
  samples: Record<string, DomainRecord>;
  methods: Record<string, DomainRecord>;
}

export const EMPTY_CHAIN: ChainLookup = { batches: {}, samples: {}, methods: {} };

export function buildChainLookup(batches: DomainRecord[], samples: DomainRecord[], methods: DomainRecord[]): ChainLookup {
  const index = (records: DomainRecord[]) => Object.fromEntries(records.map((record) => [record.code, record]));
  return { batches: index(batches), samples: index(samples), methods: index(methods) };
}

export function methodUsable(status: string | undefined): boolean {
  return status === 'active';
}

export function openSamplesOf(batchCode: string, samples: DomainRecord[]): DomainRecord[] {
  return samples.filter((sample) => sample.batchCode === batchCode && sample.status !== 'disposed');
}

// transitionBlockers mirrors the backend release gates so the workbench can
// explain why an action is blocked; the backend remains the authority.
export function transitionBlockers(configKey: string, item: DomainRecord, target: string | null, chain: ChainLookup, actor: string): string[] {
  if (!target) return [];
  const blockers: string[] = [];
  if (configKey === 'labSample' && target === 'accepted') {
    const batch = item.batchCode ? chain.batches[item.batchCode] : undefined;
    if (!batch) blockers.push(`所属批次 ${item.batchCode || '未关联'} 不存在`);
    else if (batch.status !== 'received') blockers.push(`批次 ${batch.code} 状态为 ${batch.status}，须先收货`);
    const method = item.methodCode ? chain.methods[item.methodCode] : undefined;
    if (!method) blockers.push(`检测方法 ${item.methodCode || '未关联'} 不存在`);
    else if (!methodUsable(method.status)) blockers.push(`方法 ${method.code} 版本状态为 ${method.status}，须为 active`);
  }
  if (configKey === 'samplingBatch' && target === 'closed') {
    const open = openSamplesOf(item.code, Object.values(chain.samples));
    if (open.length) blockers.push(`仍有 ${open.length} 个样本未处置：${open.slice(0, 3).map((sample) => sample.code).join('、')}${open.length > 3 ? ' 等' : ''}`);
  }
  if (configKey === 'resultReview' && target === 'signed') {
    const method = item.methodCode ? chain.methods[item.methodCode] : undefined;
    if (!method) blockers.push(`检测方法 ${item.methodCode || '未关联'} 不存在`);
    else if (!methodUsable(method.status)) blockers.push(`方法 ${method.code} 已失效（${method.status}），禁止签发`);
    const sample = item.sampleCode ? chain.samples[item.sampleCode] : undefined;
    if (!sample) blockers.push(`关联样本 ${item.sampleCode || '未关联'} 不存在`);
    else if (sample.status !== 'testing') blockers.push(`样本 ${sample.code} 状态为 ${sample.status}，须处于检测状态`);
    if (item.reviewRequestedBy && item.reviewRequestedBy === actor) blockers.push('复核人与签发人必须为不同人员');
  }
  return blockers;
}
