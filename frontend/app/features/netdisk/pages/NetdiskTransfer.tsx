import { useEffect, useState } from 'react';
import { CheckCircle2, Copy, Link2, Loader2, Rocket } from 'lucide-react';
import { fetchNetdiskAccounts, netdiskTransfer } from '../api';
import type { NetdiskAccount, NetdiskShareResult } from '../types';
import { netdiskPlatformLabel } from '../types';

// 网盘转存页：粘贴他人分享链接，选目标账号，生成自己的分享链接。
const NetdiskTransfer = () => {
  const [accounts, setAccounts] = useState<NetdiskAccount[]>([]);
  const [accountId, setAccountId] = useState('');
  const [shareUrl, setShareUrl] = useState('');
  const [passcode, setPasscode] = useState('');
  const [newPasscode, setNewPasscode] = useState('');
  const [periodDays, setPeriodDays] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState<NetdiskShareResult | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    void fetchNetdiskAccounts().then((list) => {
      const active = list.filter(item => item.status === 'active');
      setAccounts(active);
      if (active.length > 0) setAccountId(active[0].id);
    }).catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载网盘账号失败');
    });
  }, []);

  const handleTransfer = async () => {
    setError('');
    setResult(null);
    if (!accountId) { setError('请先选择目标网盘账号（可在「网盘账号」页添加）'); return; }
    if (!shareUrl.trim()) { setError('请粘贴他人的分享链接'); return; }
    setBusy(true);
    try {
      setResult(await netdiskTransfer({
        accountId, shareUrl: shareUrl.trim(), passcode: passcode.trim(),
        newPasscode: newPasscode.trim(), periodDays,
      }));
    } catch (err) {
      setError(err instanceof Error ? err.message : '转存失败');
    } finally {
      setBusy(false);
    }
  };

  const handleCopy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setError('复制失败，请手动选择文本复制');
    }
  };

  const selected = accounts.find(item => item.id === accountId);

  return (
    <div className="mx-auto max-w-3xl space-y-6 p-6">
      <header>
        <h1 className="text-2xl font-black tracking-tight text-slate-950">网盘转存</h1>
        <p className="mt-1 text-sm text-slate-500">把他人的分享一键转存到你的网盘，并生成属于你自己的分享链接</p>
      </header>

      {error && <div className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div>}

      <div className="space-y-5 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <div>
          <label htmlFor="netdisk-account" className="mb-1.5 block text-sm font-semibold text-slate-700">目标账号</label>
          <select id="netdisk-account" value={accountId} onChange={e => setAccountId(e.target.value)} className="w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm">
            {accounts.length === 0 && <option value="">暂无启用的网盘账号</option>}
            {accounts.map(account => (
              <option key={account.id} value={account.id}>{netdiskPlatformLabel(account.platform)} · {account.remark || account.id.slice(-8)}</option>
            ))}
          </select>
          {accounts.length === 0 && <p className="mt-1.5 text-xs text-amber-600">请先到「网盘账号」页扫码登录或添加账号</p>}
        </div>

        <div>
          <label htmlFor="netdisk-share-url" className="mb-1.5 block text-sm font-semibold text-slate-700">他人的分享链接</label>
          <input id="netdisk-share-url" value={shareUrl} onChange={e => setShareUrl(e.target.value)} placeholder={selected?.platform === 'baidu' ? 'https://pan.baidu.com/s/xxxx?pwd=abcd' : 'https://pan.quark.cn/s/xxxxxx'} className="w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm" />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label htmlFor="netdisk-passcode" className="mb-1.5 block text-sm font-semibold text-slate-700">源提取码<span className="font-normal text-slate-400">（若链接未带）</span></label>
            <input id="netdisk-passcode" value={passcode} onChange={e => setPasscode(e.target.value)} placeholder="如 abcd" maxLength={8} className="w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm" />
          </div>
          <div>
            <label htmlFor="netdisk-new-passcode" className="mb-1.5 block text-sm font-semibold text-slate-700">新提取码<span className="font-normal text-slate-400">（留空随机）</span></label>
            <input id="netdisk-new-passcode" value={newPasscode} onChange={e => setNewPasscode(e.target.value)} placeholder="4 位，留空随机生成" maxLength={8} className="w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm" />
          </div>
        </div>

        <div>
          <label htmlFor="netdisk-period" className="mb-1.5 block text-sm font-semibold text-slate-700">新分享有效期</label>
          <select id="netdisk-period" value={periodDays} onChange={e => setPeriodDays(Number(e.target.value))} className="w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm">
            <option value={0}>永久有效</option>
            <option value={1}>1 天</option>
            <option value={7}>7 天</option>
            <option value={30}>30 天</option>
          </select>
        </div>

        <button type="button" onClick={() => void handleTransfer()} disabled={busy} className="flex w-full items-center justify-center gap-2 rounded-lg bg-brand px-4 py-3 text-sm font-bold text-white hover:opacity-90 disabled:opacity-60">
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Rocket className="h-4 w-4" />}
          {busy ? '正在转存并生成分享链接…' : '一键转存并生成我的分享链接'}
        </button>
      </div>

      {result && (
        <div className="space-y-3 rounded-2xl border border-emerald-200 bg-emerald-50/60 p-6">
          <h2 className="flex items-center gap-2 text-lg font-bold text-emerald-800"><CheckCircle2 className="h-5 w-5" />转存成功</h2>
          <p className="text-sm text-slate-600">{result.title} · 共 {result.fileCount} 个文件 · {netdiskPlatformLabel(result.platform)}</p>
          <div className="flex items-center gap-2 rounded-lg border border-emerald-200 bg-white px-4 py-3">
            <Link2 className="h-4 w-4 shrink-0 text-emerald-600" />
            <span className="flex-1 break-all font-mono text-sm text-slate-800">{result.shareUrl}</span>
            <button type="button" onClick={() => void handleCopy(result.shareUrl)} className="flex items-center gap-1 rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-emerald-700"><Copy className="h-3.5 w-3.5" />{copied ? '已复制' : '复制链接'}</button>
          </div>
          {result.passcode && <p className="text-sm text-slate-700">提取码：<span className="rounded bg-white px-2 py-1 font-mono font-bold text-emerald-700">{result.passcode}</span></p>}
        </div>
      )}
    </div>
  );
};

export default NetdiskTransfer;
