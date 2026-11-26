import { useEffect, useRef, useState } from 'react';
import { generateImage, publishItem } from '../../studio/api';
import type { StudioAccount, StudioSettings } from '../../studio/types';
import type { OpportunityItem } from '../types';
import { runPublishBatch } from '../batch';
import { readOriginalCopy } from '../api';

/** SellerBatchPanel 将已勾选商品串行发布；确认、停止、逐项记录均由当前页面控制。 */
export default function SellerBatchPanel({ items, accounts, settings, onRunning, busy, searchAccountId }: {
  items: OpportunityItem[]; accounts: StudioAccount[]; settings: StudioSettings; onRunning: (running: boolean) => void; busy: boolean; searchAccountId: string;
}) {
  /** targets 是明确勾选的发布账号，默认不选择。 */
  const [targets, setTargets] = useState<string[]>([]);
  /** price 为空沿用原价，否则应用统一正数价格。 */
  const [price, setPrice] = useState('');
  /** source 决定沿用原图还是使用配置好的 AI 模型生成。 */
  const [source, setSource] = useState(true);
  /** running、messages 反映队列当前状态和逐项结果。 */
  const [running, setRunning] = useState(false);
  const [messages, setMessages] = useState<string[]>([]);
  /** stop 同步阻止双击与停止后的新任务，卸载时只取消后续任务。 */
  const stop = useRef(false);
  const active = useRef(false);
  useEffect(() => () => { stop.current = true; }, []);
  /** start 冻结本次任务输入；发布前确认数量，不自动触发。 */
  const start = async () => {
    if (active.current || busy) return;
    if (!items.length || !targets.length) { setMessages(['请先勾选商品和发布账号']); return; }
    if (price && (!/^\d+(\.\d{1,2})?$/.test(price) || Number(price) <= 0)) { setMessages(['统一售价必须为正数，最多两位小数']); return; }
    if (items.some(item => !price && (!/^\d+(\.\d{1,2})?$/.test(item.price) || Number(item.price) <= 0))) { setMessages(['部分原价无效，请填写统一售价']); return; }
    if (!source && (!settings.image.baseUrl || !settings.image.model)) { setMessages(['请先在 AI 出图中配置图片模型']); return; }
    if (!window.confirm(`将 ${items.length} 件商品发布到 ${targets.length} 个账号，最多 ${items.length * targets.length} 次发布。是否开始？`)) return;
    active.current = true; stop.current = false; setRunning(true); onRunning(true); setMessages([]);
    // drafts 缓存同一源商品的生成结果，多个目标账号共用，不重复付费生成。
    const drafts = new Map<string, { title: string; description: string; images: string[] }>();
    // tasks 将源商品与目标账号绑定，键使用平台数字 ID，避免串号发布。
    const tasks = new Map<string, { item: OpportunityItem; account: string }>(items.flatMap(item => targets.map(account => [`${account}:${item.itemId}`, { item, account }] as const)));
    try {
      await runPublishBatch([...tasks.keys()], {
        get: key => localStorage.getItem(`ydisks.seller-publish.v1:${key}`),
        set: (key, value) => localStorage.setItem(`ydisks.seller-publish.v1:${key}`, value),
      }, async key => {
        // task 是启动时冻结的目标，不读取用户后续切换的搜索结果。
        const task = tasks.get(key)!;
        let draft = drafts.get(task.item.itemId);
        if (!draft) {
          const { title, description, imageUrls } = await readOriginalCopy(task.item.itemId, searchAccountId);
          const images = source
            ? (imageUrls.length ? imageUrls : (task.item.imageUrl ? [task.item.imageUrl] : []))
            : [(await generateImage(settings, { image: '', headline: title, price: price || task.item.price, points: [], extra: '', copy: description, shipping: '' })).image];
          if (!images.length) throw new Error('商品没有可用图片');
          draft = { title, description, images }; drafts.set(task.item.itemId, draft);
        }
        if (stop.current) throw new Error('已停止，当前项未发布');
        const result = await publishItem({ cookieIds: [task.account], title: draft.title, description: draft.description, price: price || task.item.price, quantity: '1' }, draft.images, task.account);
        return result.item_id || '';
      }, () => stop.current, (key, status) => setMessages(previous => [...previous, `${tasks.get(key)?.item.title} → ${tasks.get(key)?.account}：${status}`]));
    } catch (error) { setMessages(previous => [...previous, String(error)]); }
    finally { active.current = false; setRunning(false); onRunning(false); }
  };
  return <section className="space-y-3 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
    <h2 className="font-bold text-slate-900">批量上架 · 已选 {items.length} 件</h2>
    <fieldset disabled={running || busy} className="space-y-3 disabled:opacity-60">
      <label className="block text-sm">统一售价（留空沿用各商品原价）<input aria-label="批量统一售价" value={price} onChange={event => setPrice(event.target.value.trim())} className="mt-1 w-full rounded-lg border px-3 py-2" /></label>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={source} onChange={event => setSource(event.target.checked)} />使用源商品全部图片（最多 9 张）</label>
      <p className="text-xs text-slate-500">发布前读取商品详情全文与全部图片，保留原始段落和换行，不自动改写；读取失败则停止。详情无图时回退到搜索封面。</p>
      <div className="flex flex-wrap gap-3">{accounts.map(account => <label key={account.id} className="flex items-center gap-1 text-sm"><input type="checkbox" checked={targets.includes(account.id)} onChange={event => setTargets(previous => event.target.checked ? [...previous, account.id] : previous.filter(id => id !== account.id))} />{account.nickname || account.remark || account.id}</label>)}</div>
      <button type="button" onClick={start} disabled={!items.length || !targets.length} className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-bold text-white disabled:opacity-40">预览数量并开始批量上架</button>
    </fieldset>
    {running && <button type="button" onClick={() => { stop.current = true; }} className="rounded-lg border px-3 py-2 text-sm">停止后续任务（当前请求完成后停止）</button>}
    <p className="text-xs text-slate-500">逐条发布，间隔至少 5 秒；失败立即暂停。已发布或结果待核对的商品在本浏览器中不会重复提交。运行时请保持此页打开。</p>
        <button type="button" disabled={running || busy} className="text-xs text-slate-500 underline" onClick={() => {
      if (!window.confirm('请先核对平台商品列表，确认这些待处理商品没有发布成功。仅解除当前所选商品和账号的待核对记录，成功记录不会清除。继续？')) return;
      for (const item of items) for (const account of targets) {
        const key = `ydisks.seller-publish.v1:${account}:${item.itemId}`;
        if (localStorage.getItem(key) === 'pending') localStorage.removeItem(key);
      }
      setMessages(previous => [...previous, '所选任务的待核对记录已解除，可重新发起']);
    }}>核对未发布后，解除所选待核对记录</button>
    <div role="status" className="max-h-60 overflow-y-auto text-xs">{messages.map((message, index) => <p key={index} className="py-1">{message}</p>)}</div>
  </section>;
}
