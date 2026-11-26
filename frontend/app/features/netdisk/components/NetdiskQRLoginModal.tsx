import { useEffect, useRef, useState } from 'react';
import { Loader2, QrCode, X } from 'lucide-react';
import {
  beginNetdiskQR, cancelNetdiskQR, createNetdiskAccount, fetchNetdiskQRStatus,
} from '../api';
import type { NetdiskPlatform, NetdiskQRStatus } from '../types';
import { netdiskPlatformLabel } from '../types';

// NetdiskQRLoginModalProps 描述扫码登录弹窗的属性。
interface NetdiskQRLoginModalProps {
  /** platform 是要登录的网盘平台。 */
  platform: NetdiskPlatform;
  /** onClose 是关闭弹窗的回调。 */
  onClose: () => void;
  /** onSuccess 是登录并保存账号成功后的回调。 */
  onSuccess: () => void;
}

// NetdiskQRLoginModal 展示二维码并轮询登录态，成功后自动保存账号。
const NetdiskQRLoginModal = ({ platform, onClose, onSuccess }: NetdiskQRLoginModalProps) => {
  const [status, setStatus] = useState<NetdiskQRStatus | null>(null);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const sessionRef = useRef<string>('');
  const cancelledRef = useRef(false);

  useEffect(() => {
    let alive = true;
    beginNetdiskQR(platform)
      .then((initial) => {
        if (!alive) return;
        sessionRef.current = initial.sessionId;
        setStatus(initial);
      })
      .catch((err: unknown) => {
        if (alive) setError(err instanceof Error ? err.message : '发起扫码登录失败');
      });
    return () => {
      alive = false;
      cancelledRef.current = true;
      if (sessionRef.current) void cancelNetdiskQR(sessionRef.current).catch(() => undefined);
    };
  }, [platform]);

  useEffect(() => {
    if (!sessionRef.current || status?.state === 'success') return;
    const timer = window.setInterval(async () => {
      try {
        const next = await fetchNetdiskQRStatus(sessionRef.current);
        if (cancelledRef.current) return;
        setStatus(next);
        if (next.state === 'success' && next.cookie) {
          setSaving(true);
          try {
            await createNetdiskAccount({ platform, cookie: next.cookie, remark: `扫码登录 ${new Date().toLocaleDateString()}` });
            onSuccess();
          } catch (err) {
            setError(err instanceof Error ? err.message : '保存账号失败');
          } finally {
            setSaving(false);
          }
        }
      } catch (err) {
        if (!cancelledRef.current) setError(err instanceof Error ? err.message : '查询登录状态失败');
      }
    }, 2000);
    return () => window.clearInterval(timer);
  }, [platform, status?.state, onSuccess]);

  const stateText = status?.state === 'waiting' ? '请用手机 App 扫描二维码'
    : status?.state === 'scanned' ? '已扫描，请在手机上确认登录'
      : status?.state === 'success' ? '登录成功，正在保存账号…'
        : status?.state === 'expired' ? '二维码已过期，请关闭后重试'
          : status?.state === 'failed' ? `登录失败：${status.error ?? '未知原因'}`
            : '正在生成二维码…';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4" role="dialog" aria-label={`${netdiskPlatformLabel(platform)}扫码登录`}>
      <div className="w-full max-w-sm rounded-2xl bg-white p-6 shadow-xl">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="flex items-center gap-2 text-lg font-bold text-slate-900"><QrCode className="h-5 w-5 text-brand" />{netdiskPlatformLabel(platform)}扫码登录</h3>
          <button type="button" onClick={onClose} aria-label="关闭" className="rounded-lg p-1 text-slate-400 hover:bg-slate-100"><X className="h-5 w-5" /></button>
        </div>
        <div className="flex min-h-[260px] items-center justify-center rounded-xl border border-slate-200 bg-slate-50">
          {status?.qrDataUrl ? (
            <img src={status.qrDataUrl} alt="登录二维码" className="max-h-[300px] max-w-full rounded-lg object-contain" />
          ) : error ? (
            <p className="px-4 text-center text-sm text-rose-600">{error}</p>
          ) : (
            <Loader2 className="h-8 w-8 animate-spin text-brand" />
          )}
        </div>
        <p className="mt-4 flex items-center justify-center gap-2 text-sm text-slate-600">
          {(status?.state === 'waiting' || saving) && <Loader2 className="h-4 w-4 animate-spin" />}
          {saving ? '正在保存账号…' : stateText}
        </p>
        <p className="mt-2 text-center text-xs text-slate-400">二维码由服务端获取后显示在这里，长时间无响应请关闭重试</p>
      </div>
    </div>
  );
};

export default NetdiskQRLoginModal;
