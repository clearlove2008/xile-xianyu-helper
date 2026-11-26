// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { get, postForm } from '../../../../shared/http/client';
import { defaultSettings } from '../../studio/api';
import type { OpportunityItem } from '../types';
import SellerBatchPanel from './SellerBatchPanel';

// 只替换 HTTP 边界；运行真实全文读取、批量队列与 FormData 发布组装，绝不联网发布。
vi.mock('../../../../shared/http/client', async importOriginal => ({
  ...await importOriginal<typeof import('../../../../shared/http/client')>(),
  get: vi.fn(), postForm: vi.fn(),
}));

/** startBatch 在组件中选择两个账号并确认批量任务。 */
const startBatch = (ids: string[]) => {
  vi.useFakeTimers();
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const items: OpportunityItem[] = ids.map(itemId => ({
    itemId, title: `截断摘要${itemId}...`, price: '1',
    imageUrl: 'data:image/png;base64,YQ==', itemUrl: '', seller: '88', area: '',
  }));
  render(<SellerBatchPanel items={items} accounts={[{ id: 'A' }, { id: 'B' }]}
    settings={defaultSettings()} onRunning={vi.fn()} busy={false} searchAccountId="reader" />);
  fireEvent.click(screen.getByLabelText('A'));
  fireEvent.click(screen.getByLabelText('B'));
  fireEvent.click(screen.getByRole('button', { name: '预览数量并开始批量上架' }));
};

afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); vi.resetAllMocks(); localStorage.clear(); });

it('两件商品到两个账号：最终请求保留各自长文、结尾、段落和表情，不使用摘要或串文案', async () => {
  const first = '  第一件\r\n\r\n[右]目录\n' + '完整内容📌\n'.repeat(2000) + '\n第一件结尾  ';
  const second = '第二件\n\n[钉子]说明\n[1]条目\n❤️ 第二件独立结尾';
  vi.mocked(get).mockImplementation(async (_url, params) => ({
    itemId: params?.item_id, title: '完整标题', description: params?.item_id === '1' ? first : second,
  }));
  vi.mocked(postForm).mockResolvedValue({ item_id: 'published' });
  startBatch(['1', '2']);
  await act(async () => { await vi.runAllTimersAsync(); });
  expect(get).toHaveBeenCalledTimes(2);
  expect(postForm).toHaveBeenCalledTimes(4);
  const expected = [first.replace('[右]', '➡️'), first.replace('[右]', '➡️'),
    '第二件\n\n📌说明\n1️⃣条目\n❤️ 第二件独立结尾', '第二件\n\n📌说明\n1️⃣条目\n❤️ 第二件独立结尾'];
  vi.mocked(postForm).mock.calls.forEach(([url, body], index) => {
    expect(url).toBe('/api/v1/items/publish');
    expect((body as FormData).get('description')).toBe(expected[index]);
    expect((body as FormData).get('cookie_id')).toBe(index % 2 ? 'B' : 'A');
  });
  expect(screen.getByRole('status').textContent).not.toContain('已暂停');
});

it('中途缺失全文时停止该商品和后续商品，不用摘要替代正文发送', async () => {
  vi.mocked(get).mockImplementation(async (_url, params) => ({
    itemId: params?.item_id, title: '摘要...', description: params?.item_id === '1' ? '第一件完整正文' : '',
  }));
  vi.mocked(postForm).mockResolvedValue({ item_id: 'published' });
  startBatch(['1', '2', '3']);
  await act(async () => { await vi.runAllTimersAsync(); });
  expect(get).toHaveBeenCalledTimes(2);
  expect(postForm).toHaveBeenCalledTimes(2);
  expect(screen.getByRole('status').textContent).toContain('未获取到完整原文');
  expect(localStorage.getItem('ydisks.seller-publish.v1:A:3')).toBeNull();
});
