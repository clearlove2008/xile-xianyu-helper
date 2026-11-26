import React, { useEffect, useState } from 'react';
import { Save, Server } from 'lucide-react';
import { fetchServerSettings, pansouErrorMessage, updateServerSettings, type PanSouServerSettings } from '../api';

/** 默认服务端设置，用于首次加载失败时的兜底展示。 */
const DEFAULT_SERVER_SETTINGS: PanSouServerSettings = {
  enabled: true,
  enableTg: false,
  channels: '',
  proxy: '',
  cacheTtlMinutes: '60',
};

// ServerSettingsPanel 让管理员维护网盘搜索的服务端配置（启用状态、TG 频道、代理、缓存）。
const ServerSettingsPanel: React.FC = () => {
  const [settings, setSettings] = useState<PanSouServerSettings>(DEFAULT_SERVER_SETTINGS);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    const load = async (): Promise<void> => {
      try {
        const next = await fetchServerSettings();
        if (!cancelled) setSettings(next);
      } catch (loadError) {
        if (!cancelled) setError(pansouErrorMessage(loadError, '读取服务端设置失败'));
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  /** handleSave 保存服务端配置，保存后需重新进入页面或稍候生效。 */
  const handleSave = async (): Promise<void> => {
    setSaving(true);
    setMessage('');
    setError('');
    try {
      await updateServerSettings(settings);
      setMessage('已保存，下一次检索生效');
    } catch (saveError) {
      setError(pansouErrorMessage(saveError, '保存失败'));
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <section className="rounded-lg border border-gray-200 bg-white px-4 py-6 text-center text-xs text-gray-400">正在读取服务端设置…</section>;
  }

  return (
    <section className="rounded-lg border border-gray-200 bg-white p-4">
      <h2 className="flex items-center gap-1.5 text-sm font-bold text-gray-900">
        <Server className="h-4 w-4 text-gray-400" />
        服务端设置
        <span className="text-xs font-medium text-gray-400">仅管理员</span>
      </h2>
      <p className="mt-1 text-xs text-gray-500">这些配置作用于整个实例，保存后对下一次检索生效。</p>

      <div className="mt-3 space-y-3">
        <label className="flex items-center justify-between gap-3">
          <span className="text-xs font-bold text-gray-700">启用网盘搜索</span>
          <input
            type="checkbox"
            checked={settings.enabled}
            onChange={event => setSettings({ ...settings, enabled: event.target.checked })}
            className="h-4 w-4 rounded border-gray-300 text-gray-900 focus:ring-gray-900"
          />
        </label>

        <label className="flex items-center justify-between gap-3">
          <span className="min-w-0">
            <span className="block text-xs font-bold text-gray-700">启用 Telegram 频道抓取</span>
            <span className="mt-0.5 block text-[11px] text-gray-400">需要能访问 t.me，国内网络通常要配合代理</span>
          </span>
          <input
            type="checkbox"
            checked={settings.enableTg}
            onChange={event => setSettings({ ...settings, enableTg: event.target.checked })}
            className="h-4 w-4 rounded border-gray-300 text-gray-900 focus:ring-gray-900"
          />
        </label>

        <label className="block">
          <span className="text-xs font-bold text-gray-700">Telegram 频道</span>
          <input
            value={settings.channels}
            onChange={event => setSettings({ ...settings, channels: event.target.value })}
            disabled={!settings.enableTg}
            placeholder="tgsearchers3, yunpanx"
            className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-xs text-gray-900 placeholder:text-gray-400 focus:border-gray-900 focus:outline-none disabled:bg-gray-50 disabled:text-gray-400"
          />
        </label>

        <label className="block">
          <span className="text-xs font-bold text-gray-700">代理地址</span>
          <input
            value={settings.proxy}
            onChange={event => setSettings({ ...settings, proxy: event.target.value })}
            placeholder="socks5://127.0.0.1:1080 或 http://127.0.0.1:7890"
            className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-xs text-gray-900 placeholder:text-gray-400 focus:border-gray-900 focus:outline-none"
          />
          <span className="mt-1 block text-[11px] text-gray-400">留空表示直连。境外插件与 Telegram 频道都需要可用的代理。</span>
        </label>

        <label className="block">
          <span className="text-xs font-bold text-gray-700">结果缓存有效期（分钟）</span>
          <input
            value={settings.cacheTtlMinutes}
            onChange={event => setSettings({ ...settings, cacheTtlMinutes: event.target.value })}
            inputMode="numeric"
            className="mt-1 w-32 rounded-md border border-gray-300 px-2.5 py-1.5 text-xs text-gray-900 focus:border-gray-900 focus:outline-none"
          />
        </label>
      </div>

      <div className="mt-3 flex items-center gap-3">
        <button
          type="button"
          onClick={() => void handleSave()}
          disabled={saving}
          className="inline-flex items-center gap-1.5 rounded-md bg-gray-900 px-3 py-1.5 text-xs font-bold text-white transition hover:bg-gray-800 disabled:bg-gray-300"
        >
          <Save className="h-3.5 w-3.5" />
          {saving ? '保存中…' : '保存设置'}
        </button>
        {message !== '' && <span className="text-xs font-medium text-emerald-600">{message}</span>}
        {error !== '' && <span className="text-xs font-medium text-red-600">{error}</span>}
      </div>
    </section>
  );
};

export default ServerSettingsPanel;
