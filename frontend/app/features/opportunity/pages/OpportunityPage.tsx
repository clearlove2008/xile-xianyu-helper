import React, { useCallback, useEffect, useState } from 'react';
import { ExternalLink, Loader2, Rocket, Search } from 'lucide-react';
import { PageHeader } from '../../../../shared/ui/PageHeader';
import {
  defaultSettings, generateImage, listAccounts, loadSettings, publishItem, studioErrorMessage,
} from '../../studio/api';
import type { StudioAccount, StudioSettings } from '../../studio/types';
import { searchOpportunities, readOriginalCopy } from '../api';
import type { OpportunityItem } from '../types';
/** SellerBatchPanel 按需加载批量操作，保持关键词搜索首屏包体预算。 */
const SellerBatchPanel = React.lazy(() => import('./SellerBatchPanel'));
/** OpportunitySearchPanel 按需加载两种查询方式，保持页面预算。 */
const OpportunitySearchPanel = React.lazy(() => import('./OpportunitySearchPanel'));

/** accountLabel 返回账号展示名：昵称 > 备注 > 用户名 > 账号 ID。 */
const accountLabel = (account: StudioAccount): string =>
  account.nickname || account.remark || account.username || account.id;

/** FlowStep 是一键上架流程的步骤标识。 */
type FlowStep = '' | 'rewrite' | 'generate' | 'publish';

/** PublishOutcome 是一键上架对单个账号的发布结果。 */
interface PublishOutcome {
  accountId: string;
  label: string;
  ok: boolean;
  detail: string;
}

/** OneClickDraft 是一键上架准备的中间产物（原始全文与全部图片）。 */
interface OneClickDraft {
  title: string;
  description: string;
  price: string;
  images: string[];
}

/** OpportunityPage 是商机挖掘：搜索闲鱼在售商品，读取全文 + 生图 + 发布。 */
const OpportunityPage: React.FC = () => {
  /** mode、seller 分别保存查询模式与卖家主页输入。 */
  const [mode, setMode] = useState<'keyword' | 'seller'>('keyword');
  const [seller, setSeller] = useState('');
  /** batchItems 只保存用户明确勾选的当前页商品；查询变化时清空。 */
  const [batchItems, setBatchItems] = useState<OpportunityItem[]>([]);
  /** batchRunning 阻止队列运行期间切换查询或启动单品发布。 */
  const [batchRunning, setBatchRunning] = useState(false);
  /** keyword 是搜索关键词。 */
  const [keyword, setKeyword] = useState('');
  /** searchAccountId 是发起搜索的账号。 */
  const [searchAccountId, setSearchAccountId] = useState('');
  /** accounts 是已登录账号清单，同时用于搜索与发布。 */
  const [accounts, setAccounts] = useState<StudioAccount[]>([]);
  /** items 是当前页搜索结果。 */
  const [items, setItems] = useState<OpportunityItem[]>([]);
  /** page 是当前页码。 */
  const [page, setPage] = useState(1);
  /** hasMore 表示是否有下一页。 */
  const [hasMore, setHasMore] = useState(false);
  /** total 是平台报告的总命中数。 */
  const [total, setTotal] = useState(0);
  /** searching 表示搜索请求进行中。 */
  const [searching, setSearching] = useState(false);
  /** error 是最近一次操作的错误提示。 */
  const [error, setError] = useState('');
  /** notice 是最近一次操作的成功提示。 */
  const [notice, setNotice] = useState('');
  /** settings 保存本机模型配置（与 AI 出图页共用一份）。 */
  const [settings, setSettings] = useState<StudioSettings>(defaultSettings);

  /** selected 是当前选中准备一键上架的源商品。 */
  const [selected, setSelected] = useState<OpportunityItem | null>(null);
  /** publishAccountIds 是一键上架的发布账号勾选。 */
  const [publishAccountIds, setPublishAccountIds] = useState<string[]>([]);
  /** oneClickPrice 是一键上架的售价（默认取源商品价）。 */
  const [oneClickPrice, setOneClickPrice] = useState('');
  /** useSourceImage 表示用源商品图直接发布（不 AI 生图）。 */
  const [useSourceImage, setUseSourceImage] = useState(false);
  /** running 表示一键上架流程进行中。 */
  const [running, setRunning] = useState(false);
  /** step 是当前执行到的步骤。 */
  const [step, setStep] = useState<FlowStep>('');
  /** draft 保存原文与生图的中间产物。 */
  const [draft, setDraft] = useState<OneClickDraft | null>(null);
  /** outcomes 保存逐账号发布结果。 */
  const [outcomes, setOutcomes] = useState<PublishOutcome[]>([]);

  useEffect(() => {
    const saved = loadSettings();
    setSettings(saved);
    listAccounts().then(list => {
      setAccounts(list);
      if (list.length > 0) {
        setSearchAccountId(prev => prev || list[0].id);
      }
    }).catch(() => undefined);
  }, []);

  /** doSearch 执行一次搜索。 */
  const doSearch = useCallback(async (targetPage: number) => {
    const kw = mode === 'seller' ? seller.trim() : keyword.trim();
    if (!kw) {
      setError(mode === 'seller' ? '请输入卖家主页链接或数字用户 ID' : '请输入搜索关键词');
      return;
    }
    if (!searchAccountId) {
      setError('没有可用的已登录账号，请先到账号管理登录');
      return;
    }
    setSearching(true);
    setBatchItems([]);
    setError('');
    setNotice('');
    try {
      const data = await searchOpportunities(mode === 'seller' ? '' : kw, searchAccountId, targetPage, 20, mode === 'seller' ? kw : '');
      setItems(data.items ?? []);
      setPage(data.page || targetPage);
      setHasMore(Boolean(data.hasMore));
      setTotal(data.total || 0);
      if ((data.items ?? []).length === 0) {
        setNotice('没有搜到相关商品，换个关键词试试');
      }
    } catch (err) {
      setItems([]);
      setHasMore(false);
      setError(studioErrorMessage(err, '搜索失败'));
    } finally {
      setSearching(false);
    }
  }, [keyword, seller, mode, searchAccountId]);

  /** togglePublishAccount 勾选或取消一个发布账号。 */
  const togglePublishAccount = useCallback((accountId: string) => {
    setPublishAccountIds(prev => prev.includes(accountId)
      ? prev.filter(id => id !== accountId)
      : [...prev, accountId]);
  }, []);

  /** pickSelected 选中一个源商品并初始化一键上架表单。 */
  const pickSelected = useCallback((item: OpportunityItem) => {
    setSelected(item);
    setDraft(null);
    setOutcomes([]);
    setOneClickPrice(item.price || '');
    setError('');
    setNotice('');
  }, []);

  /** runOneClick 执行一键上架：读取全文 → 生图 → 逐账号发布。 */
  const runOneClick = useCallback(async () => {
    if (!selected) return;
    const targets = accounts.filter(item => publishAccountIds.includes(item.id));
    if (targets.length === 0) {
      setError('请先勾选要发布的账号');
      return;
    }
    const price = oneClickPrice.trim();
    if (!price || !/^\d+(\.\d{1,2})?$/.test(price) || Number(price) <= 0) {
      setError('请填写正确的售价（如 39.9）');
      return;
    }
    setRunning(true);
    setError('');
    setNotice('');
    setOutcomes([]);
    // collected 累积逐账号结果。
    const collected: PublishOutcome[] = [];
    try {
      // 原文与图片都来自详情接口，保持段落结构，不使用摘要或自动改写。
      setStep('rewrite');
      const { title, description, imageUrls } = await readOriginalCopy(selected.itemId, searchAccountId);

      // 第二步：图片。AI 生图或使用源商品全部图片（详情无图时回退到搜索封面）。
      let images: string[];
      setStep('generate');
      if (useSourceImage) {
        images = imageUrls.length ? imageUrls : (selected.imageUrl ? [selected.imageUrl] : []);
        if (!images.length) {
          setError('源商品没有可用的图片，请改用 AI 生图');
          return;
        }
      } else {
        if (!settings.image.baseUrl.trim() || !settings.image.model.trim()) {
          setError('还没有配置图片模型，请先到「AI 出图」页完成模型设置（或勾选「用源商品图」直接发布）');
          return;
        }
        const result = await generateImage(settings, {
          image: '', headline: title, price, points: [], extra: '', copy: description, shipping: '',
        });
        images = [result.image];
      }
      setDraft({ title, description, price, images });

      // 第三步：逐账号发布，间隔 1.5 秒错峰。
      setStep('publish');
      for (let index = 0; index < targets.length; index += 1) {
        const target = targets[index];
        const label = accountLabel(target);
        try {
          const payload = await publishItem(
            { cookieIds: [target.id], title, description, price, quantity: '1' },
            images,
            target.id,
          );
          const stock = payload.stock_warning ? `；${payload.stock_warning}` : '';
          collected.push({ accountId: target.id, label, ok: true, detail: `商品 ID ${payload.item_id ?? '见商品列表'}${stock}` });
        } catch (err) {
          collected.push({ accountId: target.id, label, ok: false, detail: studioErrorMessage(err, '发布失败') });
        }
        setOutcomes([...collected]);
        if (index < targets.length - 1) {
          await new Promise(resolve => setTimeout(resolve, 1500));
        }
      }
      const succeeded = collected.filter(item => item.ok).length;
      setNotice(succeeded > 0
        ? `一键上架完成：成功 ${succeeded} 个账号，失败 ${collected.length - succeeded} 个`
        : '发布失败，请查看下方明细');
    } catch (err) {
      setError(studioErrorMessage(err, '读取原文失败，已停止上架'));
    } finally {
      setStep('');
      setRunning(false);
    }
  }, [selected, accounts, publishAccountIds, oneClickPrice, settings, useSourceImage, searchAccountId]);

  /** stepText 把步骤标识转成进度文案。 */
  const stepText = (current: FlowStep): string => {
    switch (current) {
      case 'rewrite': return '① 读取完整原文…';
      case 'generate': return '② 生成封面图…（上游出图通常 20–40 秒）';
      case 'publish': return '③ 发布到勾选账号…';
      default: return '';
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader title="商机挖掘" description="搜索闲鱼在售商品，读取完整原文后上架；可使用原图或 AI 生图。" />

      {error && <div className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm font-semibold text-rose-700">{error}</div>}
      {notice && <div className="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm font-semibold text-emerald-700">{notice}</div>}

      <React.Suspense fallback={<p>正在加载查询…</p>}>
        <OpportunitySearchPanel mode={mode} onModeChange={next => { setMode(next); setItems([]); setBatchItems([]); setPage(1); setSelected(null); }}
          value={mode === 'seller' ? seller : keyword} onValueChange={value => mode === 'seller' ? setSeller(value) : setKeyword(value)}
          accounts={accounts} accountId={searchAccountId} onAccountChange={setSearchAccountId} onSearch={doSearch}
          busy={running || batchRunning || searching} searching={searching} page={page} hasMore={hasMore} count={items.length} total={total} />
      </React.Suspense>

      <div className="grid gap-4 xl:grid-cols-[1fr_400px]">
        <section className="space-y-3">
          {items.length > 0 && <label className="flex items-center gap-2 text-sm"><input type="checkbox" disabled={running || batchRunning} checked={items.length > 0 && batchItems.length === items.length} onChange={event => setBatchItems(event.target.checked ? items : [])} />全选当前页 · 已选 {batchItems.length} 件</label>}
          {items.length === 0 ? (
            <div className="flex min-h-[240px] flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-slate-200 bg-slate-50 text-center">
              <Search className="h-6 w-6 text-slate-400" />
              <div className="text-sm font-bold text-slate-600">{mode === 'seller' ? '输入卖家主页或用户 ID，读取公开在售商品' : '输入关键词开始挖掘商机'}</div>
              <div className="text-xs text-slate-500">搜索用已登录账号的闲鱼通道直连平台，速度快、免登录</div>
            </div>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2">
              {items.map(item => {
                const active = selected?.itemId === item.itemId;
                return (
                  <div key={item.itemId}
                    className={`rounded-2xl border bg-white p-3 shadow-sm transition ${active ? 'border-emerald-400 ring-2 ring-emerald-100' : 'border-slate-200 hover:border-slate-300'}`}>
                    <label className="mb-2 flex items-center gap-2 text-xs"><input type="checkbox" aria-label={`选择商品 ${item.title}`} disabled={running || batchRunning} checked={batchItems.some(selectedItem => selectedItem.itemId === item.itemId)} onChange={event => setBatchItems(previous => event.target.checked ? [...previous, item] : previous.filter(selectedItem => selectedItem.itemId !== item.itemId))} />加入批量上架</label>
                    <div className="flex gap-3">
                      {item.imageUrl ? (
                        <img src={item.imageUrl} alt={item.title} loading="lazy"
                          className="h-20 w-20 shrink-0 rounded-xl border border-slate-100 object-cover" />
                      ) : (
                        <div className="flex h-20 w-20 shrink-0 items-center justify-center rounded-xl border border-slate-100 bg-slate-50 text-[10px] text-slate-400">无图</div>
                      )}
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-bold text-slate-800" title={item.title}>{item.title || '无标题'}</p>
                        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] font-medium text-slate-500">
                          <span className="text-rose-600 font-black text-sm">¥{item.price || '?'}</span>
                          {item.seller && <span>{item.seller}</span>}
                          {item.area && <span>· {item.area}</span>}
                        </div>
                        <div className="mt-2 flex items-center gap-2">
                          <button type="button" disabled={running || batchRunning} onClick={() => pickSelected(item)}
                            className="inline-flex items-center gap-1 rounded-lg bg-emerald-600 px-2.5 py-1.5 text-xs font-bold text-white hover:bg-emerald-700">
                            <Rocket className="h-3.5 w-3.5" />一键上架
                          </button>
                          {item.itemUrl && (
                            <a href={item.itemUrl} target="_blank" rel="noreferrer"
                              className="inline-flex items-center gap-1 rounded-lg border border-slate-200 px-2 py-1.5 text-xs font-bold text-slate-600 hover:bg-slate-50">
                              <ExternalLink className="h-3 w-3" />看原品
                            </a>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </section>

        <aside className="space-y-3">
          <React.Suspense fallback={<p className="text-sm text-slate-500">正在加载批量上架…</p>}><SellerBatchPanel items={batchItems} accounts={accounts} settings={settings} onRunning={setBatchRunning} busy={running} searchAccountId={searchAccountId} /></React.Suspense>
          {selected ? (
            <section className="space-y-3 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
              <div className="flex items-center justify-between">
                <h2 className="text-base font-black text-slate-900">一键上架</h2>
                <button type="button" onClick={() => setSelected(null)}
                  className="text-xs font-bold text-slate-400 hover:text-slate-600">关闭</button>
              </div>
              <div className="rounded-xl bg-slate-50 p-3">
                <p className="truncate text-sm font-bold text-slate-800">{selected.title}</p>
                <p className="mt-1 text-xs font-semibold text-slate-500">源价 ¥{selected.price || '?'}{selected.area ? ` · ${selected.area}` : ''}</p>
              </div>

              <label className="block">
                <span className="text-xs font-bold text-slate-600">售价</span>
                <input value={oneClickPrice} onChange={event => setOneClickPrice(event.target.value)} placeholder="如 39.9"
                  className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
              </label>

              <label className="flex cursor-pointer items-center gap-2 rounded-xl border border-slate-200 px-3 py-2">
                <input type="checkbox" checked={useSourceImage} onChange={event => setUseSourceImage(event.target.checked)}
                  className="h-4 w-4 accent-emerald-600" />
                <span className="text-xs font-bold text-slate-700">用源商品全部图片发布（最多 9 张，不 AI 生图）</span>
              </label>
              <p className="text-[11px] font-medium text-slate-500">
                图片从商品详情读取，包含主图和细节图；详情无图时回退到搜索封面。
                描述保留原商品完整原文、换行与段落，不自动改写。
                {!useSourceImage && <> 默认按原文 AI 生成新封面，需要在「AI 出图」页配置图片模型。</>}
              </p>

              <div className="rounded-xl border border-slate-200 bg-slate-50 p-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold text-slate-600">发布账号（{publishAccountIds.length}/{accounts.length}）</span>
                  <div className="flex items-center gap-1">
                    <button type="button" onClick={() => setPublishAccountIds(accounts.map(item => item.id))}
                      className="rounded-lg border border-slate-200 bg-white px-2 py-1 text-[11px] font-bold text-slate-600 hover:bg-slate-100">全选</button>
                    <button type="button" onClick={() => setPublishAccountIds([])}
                      className="rounded-lg border border-slate-200 bg-white px-2 py-1 text-[11px] font-bold text-slate-600 hover:bg-slate-100">清空</button>
                  </div>
                </div>
                <div className="mt-2 grid gap-2">
                  {accounts.map(item => {
                    const checked = publishAccountIds.includes(item.id);
                    return (
                      <label key={item.id}
                        className={`flex cursor-pointer items-center gap-2 rounded-lg border px-2.5 py-2 text-sm transition ${checked ? 'border-emerald-300 bg-emerald-50' : 'border-slate-200 bg-white hover:border-slate-300'}`}>
                        <input type="checkbox" checked={checked} onChange={() => togglePublishAccount(item.id)}
                          className="h-4 w-4 shrink-0 accent-emerald-600" />
                        <span className="truncate font-semibold text-slate-700">{accountLabel(item)}</span>
                      </label>
                    );
                  })}
                </div>
              </div>

              <button type="button" disabled={running || batchRunning} onClick={async () => {
                setRunning(true); setError('');
                try {
                  const original = await readOriginalCopy(selected.itemId, searchAccountId);
                  const images = original.imageUrls.length ? original.imageUrls : (selected.imageUrl ? [selected.imageUrl] : []);
                  setDraft({ title: original.title, description: original.description, images, price: oneClickPrice });
                } catch (err) { setError(studioErrorMessage(err, '读取完整原文失败')); }
                finally { setRunning(false); }
              }} className="w-full rounded-xl border border-sky-200 px-3 py-2 text-sm text-sky-700">读取完整原文预览</button>
              <button type="button" onClick={runOneClick} disabled={running || batchRunning}
                className="inline-flex w-full items-center justify-center gap-2 rounded-xl bg-emerald-600 px-4 py-2.5 text-sm font-black text-white hover:bg-emerald-700 disabled:opacity-60">
                {running ? <Loader2 className="h-4 w-4 animate-spin" /> : <Rocket className="h-4 w-4" />}
                {running ? '一键上架进行中…' : '开始一键上架'}
              </button>
              {running && step && (
                <p className="text-xs font-bold text-sky-700">{stepText(step)}</p>
              )}

              {draft && draft.images.length > 0 && (
                <div className="flex gap-2 overflow-x-auto rounded-xl border border-slate-200 bg-slate-50 p-2">
                  {draft.images.map((url, index) => (
                    <img key={`${url}-${index}`} src={url} alt={`商品图 ${index + 1}`} loading="lazy"
                      className="h-24 w-24 shrink-0 rounded-lg border border-slate-200 bg-white object-cover" />
                  ))}
                </div>
              )}
              {draft && (
                <div className="rounded-xl bg-slate-50 p-3 text-xs text-slate-600">
                  <p className="font-bold text-slate-700">标题：{draft.title}</p>
                  <p className="mt-1 text-xs text-slate-500">完整原文 · {Array.from(draft.description).length} 字符</p><p className="mt-1 whitespace-pre-wrap break-words">{draft.description}</p>
                </div>
              )}
              {outcomes.length > 0 && (
                <ul className="space-y-1">
                  {outcomes.map(item => (
                    <li key={item.accountId}
                      className={`rounded-lg px-3 py-2 text-xs font-semibold ${item.ok ? 'bg-emerald-50 text-emerald-700' : 'bg-rose-50 text-rose-700'}`}>
                      {item.ok ? '✓' : '✕'} {item.label}：{item.detail}
                    </li>
                  ))}
                </ul>
              )}
              <p className="text-[11px] font-medium text-slate-500">库存固定 1 件（平台限制新建多件库存），售罄后由自动补货巡检补回。同一账号重复发同一商品请自行把控频率，避免平台判定重复铺货。</p>
            </section>
          ) : (
            <section className="flex min-h-[240px] flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-slate-200 bg-slate-50 text-center">
              <Rocket className="h-6 w-6 text-slate-400" />
              <div className="text-sm font-bold text-slate-600">点击结果里的「一键上架」</div>
              <div className="text-xs text-slate-500">流程：读取完整原文 → 准备图片 → 发布到勾选账号</div>
            </section>
          )}
        </aside>
      </div>
    </div>
  );
};

export default OpportunityPage;
