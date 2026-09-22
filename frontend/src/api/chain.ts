
import { request } from './client';
import type { ChainView } from '../types/domain';

export async function loadChain(entityPath: string, id: number) {
  return request<ChainView>(`/chain/${entityPath}/${id}`);
}
