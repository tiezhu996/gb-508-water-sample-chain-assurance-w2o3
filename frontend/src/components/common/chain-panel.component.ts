
import { CommonModule } from '@angular/common';
import { Component, EventEmitter, Input, Output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import type { ChainView } from '../../types/domain';
import { formatDate } from '../../utils/format';
import { StatusBadgeComponent } from './status-badge.component';

const KIND_LABELS: Record<string, string> = { batch: '采样批次', sample: '实验室样本', method: '检测方法' };

@Component({
  selector: 'app-chain-panel', standalone: true, imports: [CommonModule, MatButtonModule, StatusBadgeComponent],
  template: `
<div *ngIf="view" class="modal-backdrop"><section class="modal chain-modal" role="dialog" aria-label="链路关系与放行核对">
  <h2>链路关系 · {{ view.code }}</h2>
  <p class="muted">当前状态 <app-status-badge [status]="view.status"/>，以下放行核对与阻断记录由服务端持久化，刷新后仍可回读。</p>
  <h3>链路关系</h3>
  <ul class="chain-links"><li *ngFor="let link of view.links">
    <strong>{{ kindLabel(link.kind) }}</strong>
    <span *ngIf="link.exists; else missing">{{ link.code }} · {{ link.name }} <app-status-badge [status]="link.status"/></span>
    <ng-template #missing><span class="chain-fail">{{ link.code || '未填写' }}（未找到）</span></ng-template>
  </li></ul>
  <h3>放行核对</h3>
  <ul class="chain-checks"><li *ngFor="let check of view.checks" [class.chain-fail]="!check.passed">
    <strong>{{ check.passed ? '✓' : '✗' }} {{ check.label }}</strong>
    <span *ngIf="check.reason">{{ check.reason }}</span>
  </li></ul>
  <ng-container *ngIf="view.disposal as disposal"><h3>处置快照</h3>
    <p>原因：{{ disposal.reason || '-' }}<br/>处置时方法：{{ disposal.methodCode || '-' }} · 所属批次：{{ disposal.batchCode || '-' }}<br/>处置时间：{{ formatDate(disposal.disposedAt || '') }}</p>
  </ng-container>
  <h3>阻断记录</h3>
  <ul *ngIf="view.blocks.length; else noBlocks" class="chain-blocks"><li *ngFor="let block of view.blocks">
    <strong>{{ formatDate(block.at) }} · {{ block.actor }}</strong><span>{{ block.reason }}</span>
  </li></ul>
  <ng-template #noBlocks><p class="muted">暂无被阻断的放行尝试。</p></ng-template>
  <footer><button mat-flat-button color="primary" (click)="close.emit()">关闭</button></footer>
</section></div>`,
})
export class ChainPanelComponent {
  @Input() view: ChainView | null = null;
  @Output() close = new EventEmitter<void>();
  readonly formatDate = formatDate;
  kindLabel(kind: string) { return KIND_LABELS[kind] || kind; }
}
