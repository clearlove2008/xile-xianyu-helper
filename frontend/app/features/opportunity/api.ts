import { get } from '../../../shared/http/client';
import type { OpportunitySearchResponse } from './types';
import { normalizePublishEmoji } from './publishEmoji';

/** SEARCH_TIMEOUT_MS 是一次搜索的等待上限（含平台风控时的排队）。 */
const SEARCH_TIMEOUT_MS = 60_000;

/** searchOpportunities 按关键词搜索闲鱼在售商品；cookieId 决定用哪个账号发起搜索。 */
export const searchOpportunities = async (
  keyword: string,
  cookieId: string,
  page = 1,
  pageSize = 20,
  seller = "",
): Promise<OpportunitySearchResponse> => {
  const data = await get<OpportunitySearchResponse>('/api/v1/opportunity/search', {
    keyword,
    ...(seller ? { seller } : {}),
    cookie_id: cookieId,
    page: String(page),
    page_size: String(pageSize),
  }, { timeoutMs: SEARCH_TIMEOUT_MS });
  return data;
};

/** readOriginalCopy 读取详情全文与全部商品图，缺失时拒绝上架，绝不回退到列表摘要或自动 AI 改写。 */
export const readOriginalCopy = async (itemId: string, cookieId: string): Promise<{ title: string; description: string; imageUrls: string[] }> => {
  const detail = await get<{ itemId: string; title: string; description: string; imageUrls?: string[] }>('/api/v1/opportunity/detail', { item_id: itemId, cookie_id: cookieId }, { timeoutMs: SEARCH_TIMEOUT_MS });
  if (detail.itemId !== itemId || typeof detail.description !== 'string' || !detail.description.trim()) throw new Error('未获取到完整原文，已停止上架');
  return {
    title: Array.from(normalizePublishEmoji(detail.title || detail.description.split('\n')[0])).slice(0, 30).join(''),
    description: normalizePublishEmoji(detail.description),
    imageUrls: Array.isArray(detail.imageUrls) ? detail.imageUrls.filter(url => typeof url === 'string' && url.trim()).map(url => url.trim()) : [],
  };
};
