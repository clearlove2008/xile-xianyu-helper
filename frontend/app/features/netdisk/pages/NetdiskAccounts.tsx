import { useCallback, useEffect, useState } from 'react';
import { CheckCircle2, Cloud, Edit3, Loader2, Plus, QrCode, RefreshCw, Trash2, XCircle } from 'lucide-react';
import {
  checkNetdiskAccount, createNetdiskAccount, deleteNetdiskAccount, fetchNetdiskAccounts, updateNetdiskAccount,
} from '../api';
import type { NetdiskAccount, NetdiskPlatform } from '../types';
import { netdiskPlatformLabel } from '../types';
import NetdiskQRLoginModal from '../components/NetdiskQRLoginModal';

// 网盘账号管理页：独立管理夸克/百度登录凭证，与闲鱼账号体系完全隔离。
const NetdiskAccounts = () => {
  const [accounts, setAccounts] = useState<NetdiskAccount[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [qrPlatform, setQrPlatform] = useState<NetdiskPlatform | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [form, setForm] = useState({ platform: 'quark' as NetdiskPlatform, cookie: '', remark: '' });
  const [checkingId, setCheckingId] = useState('');
  const [validMap, setValidMap] = useState<Record<string, boolean>>({});

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setAccounts(await fetchNetdiskAccounts());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载网盘账号失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const handleAdd = async () => {
    if (!form.cookie.trim()) { setError('请填写 Cookie'); return; }
    try {
      await createNetdiskAccount(form);
      setForm({ platform: 'quark', cookie: '', remark: '' });
      setShowAdd(false);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : '添加失败');
    }
  };

  const handleToggle = async (account: NetdiskAccount) => {
    try {
      await updateNetdiskAccount(account.id, { status: account.status === 'active' ? 'disabled' : 'active' });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新状态失败');
    }
  };

  const handleRemark = async (account: NetdiskAccount) => {
    const remark = window.prompt('修改备注', account.remark);
    if (remark === null) return;
    try {
      await updateNetdiskAccount(account.id, { remark });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : '修改备注失败');
    }
  };

  const handleDelete = async (account: NetdiskAccount) => {
    if (!window.confirm(`确认删除 ${netdiskPlatformLabel(account.platform)} 账号「${account.remark || account.id}」？`)) return;
    try {
      await deleteNetdiskAccount(account.id);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败');
    }
  };

  const handleCheck = async (account: NetdiskAccount) => {
    setCheckingId(account.id);
    try {
      const valid = await checkNetdiskAccount(account.id);
      setValidMap(prev => ({ ...prev, [account.id]: valid }));
    } catch (err) {
      setError(err instanceof Error ? err.message : '校验失败');
    } finally {
      setCheckingId('');
    }
  };

  return (
    <div className="mx-auto max-w-5xl space-y-6 p-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-black tracking-tight text-slate-950">网盘账号</h1>
          <p className="mt-1 text-sm text-slate-500">独立管理夸克与百度网盘登录凭证，Cookie 加密存储，与闲鱼账号互不干扰</p>
        </div>
        <div className="flex gap-2">
          <button type="button" onClick={() => setQrPlatform('quark')} className="flex items-center gap-1.5 rounded-lg bg-slate-800 px-3 py-2 text-sm font-semibold text-white hover:bg-slate-900"><QrCode className="h-4 w-4" />夸克扫码登录</button>
          <button type="button" onClick={() => setQrPlatform('baidu')} className="flex items-center gap-1.5 rounded-lg bg-slate-800 px-3 py-2 text-sm font-semibold text-white hover:bg-slate-900"><QrCode className="h-4 w-4" />百度扫码登录</button>
          <button type="button" onClick={() => setShowAdd(v => !v)} className="flex items-center gap-1.5 rounded-lg bg-brand px-3 py-2 text-sm font-semibold text-white hover:opacity-90"><Plus className="h-4 w-4" />手动添加</button>
        </div>
      </header>

      {error && <div className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div>}

      {showAdd && (
        <div className="space-y-3 rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="flex gap-3">
            <select value={form.platform} onChange={e => setForm({ ...form, platform: e.target.value as NetdiskPlatform })} className="rounded-lg border border-slate-300 px-3 py-2 text-sm">
              <option value="quark">夸克网盘</option>
              <option value="baidu">百度网盘</option>
            </select>
            <input value={form.remark} onChange={e => setForm({ ...form, remark: e.target.value })} placeholder="备注（可选）" className="flex-1 rounded-lg border border-slate-300 px-3 py-2 text-sm" />
          </div>
          <textarea value={form.cookie} onChange={e => setForm({ ...form, cookie: e.target.value })} placeholder="粘贴从浏览器复制的完整 Cookie" rows={3} className="w-full rounded-lg border border-slate-300 px-3 py-2 font-mono text-xs" />
          <div className="flex justify-end gap-2">
            <button type="button" onClick={() => setShowAdd(false)} className="rounded-lg px-3 py-2 text-sm text-slate-600 hover:bg-slate-100">取消</button>
            <button type="button" onClick={handleAdd} className="rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-white hover:opacity-90">保存</button>
          </div>
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center py-16 text-slate-400"><Loader2 className="mr-2 h-5 w-5 animate-spin" />加载中…</div>
      ) : accounts.length === 0 ? (
        <div className="rounded-xl border border-dashed border-slate-300 bg-white py-16 text-center text-slate-500">
          <Cloud className="mx-auto mb-3 h-10 w-10 text-slate-300" />
          <p>还没有网盘账号，扫码登录或手动添加一个</p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {accounts.map(account => (
            <div key={account.id} className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-2">
                  <span className={`rounded-lg px-2 py-1 text-xs font-bold ${account.platform === 'quark' ? 'bg-violet-100 text-violet-700' : 'bg-blue-100 text-blue-700'}`}>{netdiskPlatformLabel(account.platform)}</span>
                  <span className={`flex items-center gap-1 text-xs ${account.status === 'active' ? 'text-emerald-600' : 'text-slate-400'}`}>
                    {account.status === 'active' ? <CheckCircle2 className="h-3.5 w-3.5" /> : <XCircle className="h-3.5 w-3.5" />}
                    {account.status === 'active' ? '启用' : '停用'}
                  </span>
                </div>
                <span className="text-xs text-slate-400">{account.id.slice(-8)}</span>
              </div>
              <p className="mt-3 truncate font-semibold text-slate-800">{account.remark || '未命名账号'}</p>
              <p className="mt-1 text-xs text-slate-400">添加于 {new Date(account.createdAt).toLocaleString()}</p>
              {account.lastUsedAt && <p className="mt-0.5 text-xs text-slate-400">最近使用 {new Date(account.lastUsedAt).toLocaleString()}</p>}
              {validMap[account.id] !== undefined && (
                <p className={`mt-2 text-xs ${validMap[account.id] ? 'text-emerald-600' : 'text-rose-600'}`}>{validMap[account.id] ? '登录态有效' : '登录态已失效，请重新登录'}</p>
              )}
              <div className="mt-4 flex flex-wrap gap-2">
                <button type="button" onClick={() => void handleCheck(account)} disabled={checkingId === account.id} className="flex items-center gap-1 rounded-lg border border-slate-200 px-2 py-1.5 text-xs text-slate-600 hover:bg-slate-50 disabled:opacity-50">
                  {checkingId === account.id ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}校验
                </button>
                <button type="button" onClick={() => void handleRemark(account)} className="flex items-center gap-1 rounded-lg border border-slate-200 px-2 py-1.5 text-xs text-slate-600 hover:bg-slate-50"><Edit3 className="h-3.5 w-3.5" />备注</button>
                <button type="button" onClick={() => void handleToggle(account)} className="rounded-lg border border-slate-200 px-2 py-1.5 text-xs text-slate-600 hover:bg-slate-50">{account.status === 'active' ? '停用' : '启用'}</button>
                <button type="button" onClick={() => void handleDelete(account)} className="flex items-center gap-1 rounded-lg border border-rose-200 px-2 py-1.5 text-xs text-rose-600 hover:bg-rose-50"><Trash2 className="h-3.5 w-3.5" />删除</button>
              </div>
            </div>
          ))}
        </div>
      )}

      {qrPlatform && <NetdiskQRLoginModal platform={qrPlatform} onClose={() => setQrPlatform(null)} onSuccess={() => { setQrPlatform(null); void load(); }} />}
    </div>
  );
};

export default NetdiskAccounts;
