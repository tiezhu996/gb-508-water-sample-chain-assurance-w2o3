import { CommonModule } from '@angular/common';
import { Component, Input } from '@angular/core';
import type { DomainRecord } from '../../types/domain';
import { EMPTY_CHAIN, openSamplesOf, type ChainLookup } from '../../utils/chain';
import { StatusBadgeComponent } from './status-badge.component';

// ChainRelationsComponent renders the release-chain links of one record
// (batch <-> samples, sample -> batch/method, review -> sample/method) plus the
// disposal snapshot, all resolved from freshly loaded backend data.
@Component({
  selector: 'app-chain-relations',
  standalone: true,
  imports: [CommonModule, StatusBadgeComponent],
  template: `
  <div class="chain-relations" [ngSwitch]="configKey">
    <ng-container *ngSwitchCase="'samplingBatch'">
      <span class="chain-link">未处置样本 <strong>{{ openSamples.length }}</strong> 个</span>
      <small *ngIf="openSamples.length" class="chain-detail">{{ openSampleCodes }}</small>
      <small *ngIf="!openSamples.length" class="chain-detail">全部样本已处置，可关闭</small>
    </ng-container>
    <ng-container *ngSwitchCase="'labSample'">
      <span class="chain-link">批次 <strong>{{ item.batchCode || '未关联' }}</strong><app-status-badge *ngIf="batch" [status]="batch.status"/></span>
      <span class="chain-link">方法 <strong>{{ item.methodCode || '未关联' }}</strong><app-status-badge *ngIf="method" [status]="method.status"/></span>
      <small *ngIf="item.status === 'disposed' && item.disposedReason" class="chain-detail">处置：{{ item.disposedReason }} · 方法 {{ item.disposedMethodCode || '-' }} · 批次 {{ item.disposedBatchCode || '-' }}</small>
    </ng-container>
    <ng-container *ngSwitchCase="'resultReview'">
      <span class="chain-link">样本 <strong>{{ item.sampleCode || '未关联' }}</strong><app-status-badge *ngIf="sample" [status]="sample.status"/></span>
      <span class="chain-link">方法 <strong>{{ item.methodCode || '未关联' }}</strong><app-status-badge *ngIf="method" [status]="method.status"/></span>
      <small *ngIf="item.reviewRequestedBy" class="chain-detail">提交 {{ item.reviewRequestedBy }}<span *ngIf="item.signedBy"> · 签发 {{ item.signedBy }}</span></small>
    </ng-container>
  </div>`
})
export class ChainRelationsComponent {
  @Input() configKey = '';
  @Input() item!: DomainRecord;
  @Input() chain: ChainLookup = EMPTY_CHAIN;
  get batch(): DomainRecord | undefined { return this.item.batchCode ? this.chain.batches[this.item.batchCode] : undefined; }
  get method(): DomainRecord | undefined { return this.item.methodCode ? this.chain.methods[this.item.methodCode] : undefined; }
  get sample(): DomainRecord | undefined { return this.item.sampleCode ? this.chain.samples[this.item.sampleCode] : undefined; }
  get openSamples(): DomainRecord[] { return openSamplesOf(this.item.code, Object.values(this.chain.samples)); }
  get openSampleCodes(): string { return this.openSamples.slice(0, 3).map((sample) => sample.code).join('、') + (this.openSamples.length > 3 ? ' 等' : ''); }
}
