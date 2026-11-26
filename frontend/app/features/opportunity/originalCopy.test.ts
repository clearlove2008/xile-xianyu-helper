import { expect, it, vi } from 'vitest';
import { get } from '../../../shared/http/client';
import { readOriginalCopy } from './api';
vi.mock('../../../shared/http/client', () => ({ get: vi.fn() }));

it('标题限长但完整描述保持逐字一致，不改写或截断', async () => {
  const original = '  第一段\r\n\r\n一、目录\n  - 条目\n' + '完整长文\n'.repeat(2000) + '最后一句  ';
  vi.mocked(get).mockResolvedValue({ itemId: '123', title: '长标题'.repeat(30), description: original });
  const result = await readOriginalCopy('123', 'account');
  expect(result.description).toBe(original); expect(Array.from(result.title)).toHaveLength(30);
});
it('全文缺失时停止，不使用摘要兜底', async () => {
  vi.mocked(get).mockResolvedValue({ itemId: '123', title: '截断摘要...', description: '' });
  await expect(readOriginalCopy('123', 'account')).rejects.toThrow('完整原文');
});
it('不接受其他商品的详情', async () => {
  vi.mocked(get).mockResolvedValue({ itemId: '456', description: '其他商品' });
  await expect(readOriginalCopy('123', 'account')).rejects.toThrow('完整原文');
});

it('把平台专用表情代码转成标准表情，保留全文和段落', async () => {
  const original = '  [右]【形式】网课+电子讲义\r\n[右]【适用】手机/电脑/平板都能看\r\n\r\n💗发货方式\n\n[钉子]内容包含四个模块\n[1]英语一/英语二\n\n[2]数学一/数学二/数学三\n\n[3]政治  ';
  vi.mocked(get).mockResolvedValue({ itemId: '123', title: '[右]课程', description: original });
  const result = await readOriginalCopy('123', 'account');
  expect(result.title).toBe('➡️课程');
  expect(result.description).toBe('  ➡️【形式】网课+电子讲义\r\n➡️【适用】手机/电脑/平板都能看\r\n\r\n💗发货方式\n\n📌内容包含四个模块\n1️⃣英语一/英语二\n\n2️⃣数学一/数学二/数学三\n\n3️⃣政治  ');
});

it('返回详情里的全部商品图，缺失时给出空列表', async () => {
  vi.mocked(get).mockResolvedValue({ itemId: '123', title: '商品', description: '正文', imageUrls: ['https://img.alicdn.com/1.jpg', 'http://img.alicdn.com/2.jpg'] });
  expect((await readOriginalCopy('123', 'account')).imageUrls).toEqual(['https://img.alicdn.com/1.jpg', 'http://img.alicdn.com/2.jpg']);
  vi.mocked(get).mockResolvedValue({ itemId: '123', title: '商品', description: '正文' });
  expect((await readOriginalCopy('123', 'account')).imageUrls).toEqual([]);
});

it('不误删普通括号正文、未知代码或已有 Unicode 表情', async () => {
  const original = '[PDF][课程目录][2027][未知表情]【适用】❤️ ➡️ 📌 1️⃣ 👨‍👩‍👧‍👦 👍🏽\n最后一句';
  vi.mocked(get).mockResolvedValue({ itemId: '123', title: '课程', description: original });
  expect((await readOriginalCopy('123', 'account')).description).toBe(original);
});
