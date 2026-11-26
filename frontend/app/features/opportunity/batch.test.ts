import { expect, it, vi } from 'vitest';
import { runPublishBatch } from './batch';

/** fixture 返回内存发布记录，用于验证刷新后跳过与待核对保护。 */
const fixture = () => {
  const records = new Map<string, string>();
  return { records, journal: { get: (key: string) => records.get(key) ?? null, set: (key: string, value: string) => { records.set(key, value); } } };
};
it('去重并跳过已发布或待核对任务', async () => {
  const { records, journal } = fixture(); records.set('old', 'published'); records.set('unknown', 'pending');
  const publish = vi.fn().mockResolvedValue('item-id');
  await runPublishBatch(['new', 'new', 'old', 'unknown'], journal, publish, () => false, vi.fn(), async () => {});
  expect(publish).toHaveBeenCalledTimes(1); expect(records.get('new')).toBe('item-id');
});
it('发布异常保留待核对记录并停止后续任务', async () => {
  const { records, journal } = fixture(); const publish = vi.fn().mockRejectedValue(new Error('timeout'));
  await runPublishBatch(['first', 'second'], journal, publish, () => false, vi.fn(), async () => {});
  expect(publish).toHaveBeenCalledTimes(1); expect(records.get('first')).toBe('pending'); expect(records.has('second')).toBe(false);
});
it('缺少商品 ID 不当作成功，不继续下一项', async () => {
  const { records, journal } = fixture(); const publish = vi.fn().mockResolvedValue('');
  await runPublishBatch(['first', 'second'], journal, publish, () => false, vi.fn(), async () => {});
  expect(publish).toHaveBeenCalledTimes(1); expect(records.get('first')).toBe('pending');
});
it('取消不执行任何后续发布', async () => {
  const { journal } = fixture(); const publish = vi.fn();
  await runPublishBatch(['first'], journal, publish, () => true, vi.fn(), async () => {});
  expect(publish).not.toHaveBeenCalled();
});
it('记录写入失败时不执行外部发布', async () => {
  const publish = vi.fn();
  await expect(runPublishBatch(['first'], { get: () => null, set: () => { throw new Error('storage full'); } }, publish, () => false, vi.fn())).rejects.toThrow('storage full');
  expect(publish).not.toHaveBeenCalled();
});
