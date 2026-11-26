import type { StudioAccount } from '../../studio/types';

/** OpportunitySearchPanel 只渲染查询输入与分页；由父组件控制异步请求和运行互斥。 */
export default function OpportunitySearchPanel(props: {
  mode: 'keyword' | 'seller'; onModeChange: (mode: 'keyword' | 'seller') => void;
  value: string; onValueChange: (value: string) => void;
  accounts: StudioAccount[]; accountId: string; onAccountChange: (id: string) => void;
  onSearch: (page: number) => void; busy: boolean; searching: boolean;
  page: number; hasMore: boolean; count: number; total: number;
}) {
  return <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
    <fieldset disabled={props.busy}>
      <div className="mb-3 flex gap-3" role="group" aria-label="挖掘方式">
        {(['keyword', 'seller'] as const).map(mode => <button key={mode} type="button" aria-pressed={props.mode === mode} onClick={() => props.onModeChange(mode)} className={`rounded-lg px-3 py-2 text-sm ${props.mode === mode ? 'bg-sky-600 text-white' : 'bg-slate-100'}`}>{mode === 'seller' ? '指定卖家' : '关键词搜索'}</button>)}
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <label className="block min-w-[220px] flex-1 text-xs font-bold text-slate-600">{props.mode === 'seller' ? '卖家主页链接或用户 ID' : '关键词'}
          <input value={props.value} onChange={event => props.onValueChange(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') props.onSearch(1); }} placeholder={props.mode === 'seller' ? 'https://www.goofish.com/personal?userId=…' : '例如：露营车、考研资料、iPhone 15'} className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
        </label>
        <label className="block text-xs font-bold text-slate-600">搜索账号
          <select value={props.accountId} onChange={event => props.onAccountChange(event.target.value)} className="mt-1 block w-40 rounded-xl border border-slate-200 px-3 py-2 text-sm">
            {!props.accounts.length && <option value="">无已登录账号</option>}
            {props.accounts.map(account => <option key={account.id} value={account.id}>{account.nickname || account.remark || account.username || account.id}</option>)}
          </select>
        </label>
        <button type="button" onClick={() => props.onSearch(1)} className="rounded-xl bg-sky-600 px-4 py-2 text-sm font-bold text-white disabled:opacity-50">{props.searching ? '读取中…' : '搜索'}</button>
      </div>
      {(props.count > 0 || props.hasMore || props.page > 1) && <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-slate-500">
        <span>平台总数 {props.total || '未知'} · 第 {props.page} 页 · 本页可用 {props.count} 件</span>
        <button type="button" disabled={props.page <= 1} onClick={() => props.onSearch(props.page - 1)} className="rounded-lg border px-2 py-1 disabled:opacity-40">上一页</button>
        <button type="button" disabled={!props.hasMore || props.page >= 50} onClick={() => props.onSearch(props.page + 1)} className="rounded-lg border px-2 py-1 disabled:opacity-40">下一页</button>
      </div>}
    </fieldset>
  </section>;
}
