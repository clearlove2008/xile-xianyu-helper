/** BatchJournal 在发布前写入待核对状态，成功后保存商品 ID；刷新页面后也不自动重发。 */
export interface BatchJournal { get(key: string): string | null; set(key: string, value: string): void }

/** runPublishBatch 串行执行去重任务，任何异常立即停队列；取消只阻止后续发布，不中断可能已成功的当前请求。 */
export const runPublishBatch = async (
  keys: string[], journal: BatchJournal, publish: (key: string) => Promise<string>,
  cancelled: () => boolean, report: (key: string, status: string) => void,
  wait: () => Promise<void> = () => new Promise(resolve => setTimeout(resolve, 5000)),
): Promise<void> => {
  // unique 消除同一商品与目标账号组合的重复勾选。
  const unique = [...new Set(keys)];
  for (const key of unique) {
    if (cancelled()) return;
    if (journal.get(key)) { report(key, '已发布或待核对，跳过'); continue; }
    journal.set(key, 'pending');
    try {
      // itemID 必须存在，缺失时不能把不确定结果当作成功后自动重试。
      const itemID = await publish(key);
      if (!itemID) throw new Error('发布响应缺少商品 ID，请先到商品列表核对');
      journal.set(key, itemID);
      report(key, `成功：${itemID}`);
    } catch (error) {
      report(key, `已暂停，请核对后处理：${error instanceof Error ? error.message : '发布失败'}`);
      return;
    }
    if (cancelled()) return;
    await wait();
  }
};
