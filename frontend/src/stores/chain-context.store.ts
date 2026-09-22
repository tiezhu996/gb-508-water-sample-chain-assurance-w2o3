import { Injectable } from '@angular/core';
import { BehaviorSubject } from 'rxjs';
import { request } from '../api/client';
import type { DomainRecord } from '../types/domain';
import { buildChainLookup, EMPTY_CHAIN, type ChainLookup } from '../utils/chain';

export interface ChainContextState extends ChainLookup { loading: boolean }

// ChainContextStore loads the three chain dimensions once per refresh so batch,
// sample and review pages can resolve their cross-entity relations.
@Injectable({ providedIn: 'root' })
export class ChainContextStore {
  private readonly subject = new BehaviorSubject<ChainContextState>({ ...EMPTY_CHAIN, loading: false });
  readonly state$ = this.subject.asObservable();
  get snapshot(): ChainContextState { return this.subject.value; }
  async load(): Promise<void> {
    this.subject.next({ ...this.subject.value, loading: true });
    try {
      const [batches, samples, methods] = await Promise.all([
        request<DomainRecord[]>('/sampling-batches?page=1&pageSize=100'),
        request<DomainRecord[]>('/samples?page=1&pageSize=100'),
        request<DomainRecord[]>('/methods?page=1&pageSize=100'),
      ]);
      this.subject.next({ ...buildChainLookup(batches.data, samples.data, methods.data), loading: false });
    } catch {
      this.subject.next({ ...this.subject.value, loading: false });
    }
  }
}
