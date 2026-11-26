// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import OpportunityPage from './OpportunityPage';
import { searchOpportunities } from '../api';
import { publishItem } from '../../studio/api';

vi.mock('../api', () => ({ searchOpportunities: vi.fn().mockResolvedValue({ items: [{ itemId: '123', title: '测试商品', price: '10', imageUrl: 'https://example.com/image.png', itemUrl: '', seller: '88', area: '' }], page: 1, hasMore: true, total: 1 }) }));
vi.mock('../../studio/api', () => ({
  defaultSettings: () => ({ image: {}, text: {} }), loadSettings: () => ({ image: {}, text: {} }),
  listAccounts: async () => [{ id: '7', nickname: '测试账号' }], publishItem: vi.fn(), generateImage: vi.fn(), polishCopy: vi.fn(),
  studioErrorMessage: (error: Error) => error.message,
}));
afterEach(() => { cleanup(); vi.clearAllMocks(); vi.restoreAllMocks(); localStorage.clear(); });

it('卖家模式将主页传给服务端，并支持当前页全选', async () => {
  render(<OpportunityPage />);
  await screen.findByRole('option', { name: '测试账号' });
  fireEvent.click(screen.getByRole('button', { name: '指定卖家' }));
  fireEvent.change(screen.getByPlaceholderText('https://www.goofish.com/personal?userId=…'), { target: { value: 'https://www.goofish.com/personal?userId=88' } });
  fireEvent.click(screen.getByRole('button', { name: '搜索' }));
  await waitFor(() => expect(searchOpportunities).toHaveBeenCalledWith('', '7', 1, 20, 'https://www.goofish.com/personal?userId=88'));
  fireEvent.click(await screen.findByLabelText(/全选当前页/));
  expect(await screen.findByText('批量上架 · 已选 1 件')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: '关键词搜索' }));
  expect(await screen.findByText('批量上架 · 已选 0 件')).toBeTruthy();
});

it('批量发布需要用户确认，取消不执行外部发布', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(false);
  render(<OpportunityPage />);
  await screen.findByRole('option', { name: '测试账号' });
  fireEvent.change(screen.getByPlaceholderText('例如：露营车、考研资料、iPhone 15'), { target: { value: '测试' } });
  fireEvent.click(screen.getByRole('button', { name: '搜索' }));
  fireEvent.click(await screen.findByLabelText(/全选当前页/));
  fireEvent.click(screen.getByLabelText('测试账号'));
  fireEvent.click(screen.getByRole('button', { name: '预览数量并开始批量上架' }));
  expect(window.confirm).toHaveBeenCalled(); expect(publishItem).not.toHaveBeenCalled();
});
