// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import { imageToFile } from './api';
afterEach(() => vi.unstubAllGlobals());
it('下载网址图片，不把网址作为 Base64 解码', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, blob: async () => new Blob(['image'], { type: 'image/png' }) }));
  const file = await imageToFile('https://example.com/cover.png', 'cover.png');
  expect(file.type).toBe('image/png');
  expect(file.size).toBe(5);
  expect(fetch).toHaveBeenCalledWith('https://example.com/cover.png', { credentials: 'omit' });
});
it('保留 Base64 图片发布', async () => {
  expect((await imageToFile('data:image/png;base64,aGVsbG8=', 'cover.png')).size).toBe(5);
});
it('拒绝无效 Base64 并显示业务提示', async () => {
  await expect(imageToFile('invalid!!!', 'cover.png')).rejects.toThrow('Base64');
});
it('图片跨域失败时显示可理解的提示', async () => {
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));
  await expect(imageToFile('https://example.com/cover.png', 'cover.png')).rejects.toThrow('跨域');
});
it('拒绝将 HTML 错误页上传为图片', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, blob: async () => new Blob(['error'], { type: 'text/html' }) }));
  await expect(imageToFile('https://example.com/cover.png', 'cover.png')).rejects.toThrow('有效图片');
});
