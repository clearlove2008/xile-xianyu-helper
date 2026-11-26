import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Download, ImagePlus, Loader2, Send, Settings2, Sparkles, Wand2 } from 'lucide-react';
import { PageHeader } from '../../../../shared/ui/PageHeader';
import {
  defaultSettings, fetchSystemPrompt, generateImage, listAccounts, loadSettings, polishCopy, previewPrompt, publishItem,
  saveSettings, studioErrorMessage, testConnection,
} from '../api';
import type {
  StudioAccount, StudioGenerateForm, StudioGenerateResult, StudioModelConfig, StudioPublishTarget, StudioSettings,
} from '../types';

/** accountLabel 返回账号展示名：昵称 > 备注 > 用户名 > 账号 ID。 */
const accountLabel = (account: StudioAccount): string =>
  account.nickname || account.remark || account.username || account.id;

/** emptyForm 返回一份空的出图输入。 */
const emptyForm = (): StudioGenerateForm => ({
  image: '', headline: '', price: '', points: [], extra: '', copy: '', shipping: '',
});

/** ModelGroup 渲染一组模型配置（图片或文字），两组之间互不共享密钥。 */
const ModelGroup: React.FC<{
  /** title 是分组标题。 */ title: string;
  /** hint 是分组说明。 */ hint: string;
  /** value 是当前配置。 */ value: StudioModelConfig;
  /** onChange 是配置变更回调。 */ onChange: (next: StudioModelConfig) => void;
  /** showFallback 表示是否展示备用模型输入。 */ showFallback: boolean;
  /** modelPlaceholder 是模型名输入框的示例文案。 */ modelPlaceholder: string;
  /** onTest 是测试连接回调。 */ onTest: () => void;
  /** testing 表示是否正在测试。 */ testing: boolean;
}> = ({ title, hint, value, onChange, showFallback, modelPlaceholder, onTest, testing }) => (
  <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 className="text-base font-black text-slate-900">{title}</h2>
        <p className="mt-1 text-xs font-medium text-slate-500">{hint}</p>
      </div>
      <button
        type="button"
        onClick={onTest}
        disabled={testing}
        className="inline-flex items-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-xs font-bold text-slate-700 hover:bg-slate-50 disabled:opacity-50"
      >
        {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Settings2 className="h-3.5 w-3.5" />}
        测试连接
      </button>
    </div>
    <div className="mt-4 grid gap-3 md:grid-cols-2">
      <label className="block">
        <span className="text-xs font-bold text-slate-600">Base URL</span>
        <input
          value={value.baseUrl}
          onChange={event => onChange({ ...value, baseUrl: event.target.value })}
          placeholder="https://api.example.com/v1"
          className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400"
        />
      </label>
      <label className="block">
        <span className="text-xs font-bold text-slate-600">密钥</span>
        <input
          type="password"
          value={value.apiKey}
          onChange={event => onChange({ ...value, apiKey: event.target.value })}
          placeholder="sk-..."
          className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400"
        />
      </label>
      <label className="block">
        <span className="text-xs font-bold text-slate-600">模型名</span>
        <input
          value={value.model}
          onChange={event => onChange({ ...value, model: event.target.value })}
          placeholder={modelPlaceholder}
          className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400"
        />
      </label>
      {showFallback && (
        <label className="block">
          <span className="text-xs font-bold text-slate-600">备用模型</span>
          <input
            value={value.fallback}
            onChange={event => onChange({ ...value, fallback: event.target.value })}
            placeholder="gpt-image-2.5-sunburst"
            className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400"
          />
        </label>
      )}
    </div>
  </section>
);

/** StudioPage 是 AI 出图工作台：改图、生成、预览与一键上架。 */
const StudioPage: React.FC = () => {
  /** settings 保存本机模型配置与系统提示词。 */
  const [settings, setSettings] = useState<StudioSettings>(defaultSettings);
  /** tab 表示当前面板。 */
  const [tab, setTab] = useState<'create' | 'settings'>('create');
  /** form 保存出图输入。 */
  const [form, setForm] = useState<StudioGenerateForm>(emptyForm);
  /** pointsText 是卖点清单的文本形态，每行一个。 */
  const [pointsText, setPointsText] = useState('');
  /** result 保存生成结果。 */
  const [result, setResult] = useState<StudioGenerateResult | null>(null);
  /** busy 表示出图请求进行中。 */
  const [busy, setBusy] = useState(false);
  /** error 是最近一次操作的错误提示。 */
  const [error, setError] = useState('');
  /** notice 是最近一次操作的成功提示。 */
  const [notice, setNotice] = useState('');
  /** testingGroup 记录正在测试连接的分组。 */
  const [testingGroup, setTestingGroup] = useState<'' | 'image' | 'text'>('');
  /** models 保存测试连接返回的模型清单。 */
  const [models, setModels] = useState<string[]>([]);
  /** promptPreview 保存提示词预览内容。 */
  const [promptPreview, setPromptPreview] = useState('');
  /** accounts 是可用发布账号。 */
  const [accounts, setAccounts] = useState<StudioAccount[]>([]);
  /** publish 保存发布表单；cookieIds 是勾选的发布账号，可为 1 个或多个。 */
  const [publish, setPublish] = useState({ cookieIds: [] as string[], title: '', description: '', price: '', quantity: '1' });
  /** publishing 表示发布请求进行中。 */
  const [publishing, setPublishing] = useState(false);
  /** publishProgress 是多账号发布时的当前进度文案。 */
  const [publishProgress, setPublishProgress] = useState('');
  /** publishResults 保存本次多账号发布逐个账号的结果。 */
  const [publishResults, setPublishResults] = useState<StudioPublishTarget[]>([]);
  /** defaultPrompt 保存服务端默认系统提示词，用于一键恢复。 */
  const [defaultPrompt, setDefaultPrompt] = useState('');
  /** polishing 表示文案润色请求进行中。 */
  const [polishing, setPolishing] = useState(false);
  /** fileRef 指向隐藏的文件选择框。 */
  const fileRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    const saved = loadSettings();
    setSettings(saved);
    saveSettings(saved);
    fetchSystemPrompt().then(setDefaultPrompt).catch(() => undefined);
    listAccounts().then(list => {
      // 发布账号由用户显式勾选，这里只装载账号清单，不预设任何账号，避免误发到非预期账号。
      setAccounts(list);
    }).catch(() => undefined);
  }, []);

  /** toggleAccount 勾选或取消一个发布账号。 */
  const toggleAccount = useCallback((accountId: string) => {
    setPublish(prev => ({
      ...prev,
      cookieIds: prev.cookieIds.includes(accountId)
        ? prev.cookieIds.filter(id => id !== accountId)
        : [...prev.cookieIds, accountId],
    }));
  }, []);

  /** persist 保存设置到本机并同步状态。 */
  const persist = useCallback((next: StudioSettings) => {
    setSettings(next);
    saveSettings(next);
  }, []);

  /** readImage 把选中的图片读成 data URL。 */
  const readImage = useCallback((file: File) => {
    const reader = new FileReader();
    reader.onload = () => {
      setForm(prev => ({ ...prev, image: String(reader.result ?? '') }));
    };
    reader.readAsDataURL(file);
  }, []);

  /** onPaste 支持直接把截图贴进页面。 */
  const onPaste = useCallback((event: React.ClipboardEvent<HTMLDivElement>) => {
    const item = Array.from(event.clipboardData.items).find(entry => entry.type.startsWith('image/'));
    const file = item?.getAsFile();
    if (file) readImage(file);
  }, [readImage]);

  /** handleTest 测试一组配置的连通性。 */
  const handleTest = useCallback(async (group: 'image' | 'text') => {
    setTestingGroup(group);
    setError('');
    setModels([]);
    try {
      const list = await testConnection(settings[group]);
      setModels(list);
      setNotice(`${group === 'image' ? '图片' : '文字'}模型连通，可用模型 ${list.length} 个`);
    } catch (err) {
      setError(studioErrorMessage(err, '连接失败'));
    } finally {
      setTestingGroup('');
    }
  }, [settings]);

  /** handlePreview 预览提示词。 */
  const handlePreview = useCallback(async () => {
    setError('');
    try {
      setPromptPreview(await previewPrompt(form, settings.systemPrompt || defaultPrompt));
    } catch (err) {
      setError(studioErrorMessage(err, '生成提示词失败'));
    }
  }, [form, settings.systemPrompt, defaultPrompt]);

  /** handlePolish 用文字模型把当前文案改写成闲鱼口吻。 */
  const handlePolish = useCallback(async () => {
    if (!settings.text.baseUrl.trim() || !settings.text.model.trim()) {
      setError('请先在设置页配置文字模型的地址与模型名');
      return;
    }
    if (!form.copy.trim()) {
      setError('请先填写完整文案，再点润色');
      return;
    }
    setPolishing(true);
    setError('');
    try {
      const text = await polishCopy(settings, form.copy, form.shipping);
      setForm(prev => ({ ...prev, copy: text }));
      setNotice('文案已按闲鱼口吻改写');
    } catch (err) {
      setError(studioErrorMessage(err, '润色失败'));
    } finally {
      setPolishing(false);
    }
  }, [settings, form.copy, form.shipping]);

  /** handleGenerate 执行一次出图。 */
  const handleGenerate = useCallback(async () => {
    setBusy(true);
    setError('');
    setNotice('');
    setResult(null);
    try {
      const effective: StudioSettings = { ...settings, systemPrompt: settings.systemPrompt || defaultPrompt };
      const data = await generateImage(effective, form);
      setResult(data);
      setPublish(prev => ({
        ...prev,
        title: prev.title || form.headline,
        price: prev.price || form.price,
        description: prev.description || [form.headline, pointsText, form.copy].filter(Boolean).join('\n'),
      }));
      setNotice(`出图完成，用时 ${data.seconds.toFixed(0)} 秒${data.fallback ? `（已回退到 ${data.model}）` : ''}`);
    } catch (err) {
      setError(studioErrorMessage(err, '出图失败'));
    } finally {
      setBusy(false);
    }
  }, [settings, form, pointsText, defaultPrompt]);

  /** handlePublish 把结果图依次发布到勾选的账号；逐个账号独立收集结果，单个失败不影响其余账号。 */
  const handlePublish = useCallback(async () => {
    if (!result) return;
    // targets 是本次勾选的账号，按账号清单顺序发布。
    const targets = accounts.filter(item => publish.cookieIds.includes(item.id));
    if (targets.length === 0) {
      setError('请先勾选要发布的账号');
      return;
    }
    if (!publish.title.trim() || !publish.price.trim()) {
      setError('标题和价格不能为空');
      return;
    }
    if (!/^\d+$/.test(publish.quantity.trim()) || Number(publish.quantity) <= 0) {
      setError('库存必须是大于 0 的整数');
      return;
    }
    setPublishing(true);
    setError('');
    setNotice('');
    setPublishResults([]);
    // collected 累积逐账号结果，边发布边展示，多账号时能看到进行到第几个。
    const collected: StudioPublishTarget[] = [];
    try {
      for (let index = 0; index < targets.length; index += 1) {
        // target 是当前正在发布的账号。
        const target = targets[index];
        // label 是当前账号的展示名，用于进度与结果提示。
        const label = accountLabel(target);
        setPublishProgress(`正在发布 ${index + 1}/${targets.length}：${label}`);
        try {
          const payload = await publishItem(
            { ...publish, description: publish.description.trim() || publish.title },
            result.image,
            target.id,
          );
          const stock = payload.stock_warning ? `；${payload.stock_warning}` : '';
          collected.push({ accountId: target.id, label, ok: true, detail: `商品 ID ${payload.item_id ?? '见商品列表'}${stock}` });
        } catch (err) {
          collected.push({ accountId: target.id, label, ok: false, detail: studioErrorMessage(err, '发布失败') });
        }
        setPublishResults([...collected]);
        // 多账号之间错峰 1.5 秒，避免同一时刻对平台连续提交触发风控。
        if (index < targets.length - 1) {
          await new Promise(resolve => setTimeout(resolve, 1500));
        }
      }
      // 成功数用于汇总提示；失败明细已在结果列表里逐条展示。
      const succeeded = collected.filter(item => item.ok).length;
      setNotice(`发布完成：成功 ${succeeded} 个，失败 ${collected.length - succeeded} 个`);
    } finally {
      setPublishProgress('');
      setPublishing(false);
    }
  }, [result, publish, accounts]);

  /** downloadHref 是结果图的下载地址。 */
  const downloadHref = useMemo(() => result?.image ?? '', [result]);

  return (
    <div className="space-y-6">
      <PageHeader title="AI 出图" description="改图、生成商品主图，确认后直接上架；模型地址与密钥只存在本机浏览器。">
        <div className="flex items-center gap-2 rounded-xl bg-slate-100 p-1">
          {(['create', 'settings'] as const).map(key => (
            <button
              key={key}
              type="button"
              onClick={() => setTab(key)}
              className={`rounded-lg px-4 py-2 text-xs font-bold transition ${tab === key ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-500'}`}
            >
              {key === 'create' ? '出图' : '设置'}
            </button>
          ))}
        </div>
      </PageHeader>

      {error && <div className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm font-semibold text-rose-700">{error}</div>}
      {notice && <div className="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm font-semibold text-emerald-700">{notice}</div>}

      {tab === 'settings' ? (
        <div className="space-y-4">
          <ModelGroup
            title="图片模型"
            hint="用于改图与文生图，支持 OpenAI 兼容的 /images/edits 与 /images/generations。"
            value={settings.image}
            onChange={next => persist({ ...settings, image: next })}
            showFallback
            modelPlaceholder="gpt-image-2"
            onTest={() => handleTest('image')}
            testing={testingGroup === 'image'}
          />
          <ModelGroup
            title="文字模型"
            hint="可选。用于把文案润色成闲鱼口吻；留空不影响出图，出图指令由内置模板组装。"
            value={settings.text}
            onChange={next => persist({ ...settings, text: next })}
            showFallback={false}
            modelPlaceholder="如 gpt-4o-mini"
            onTest={() => handleTest('text')}
            testing={testingGroup === 'text'}
          />
          {models.length > 0 && (
            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
              <div className="text-xs font-bold text-slate-600">可用模型（{models.length}）</div>
              <div className="mt-2 flex flex-wrap gap-2">
                {models.map(name => (
                  <span key={name} className="rounded-lg bg-white px-2 py-1 text-xs font-semibold text-slate-700 shadow-sm">{name}</span>
                ))}
              </div>
            </div>
          )}
          <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 className="text-base font-black text-slate-900">系统提示词</h2>
                <p className="mt-1 text-xs font-medium text-slate-500">决定模型如何理解"只改指定文字、不擅自加元素"。</p>
              </div>
              <button
                type="button"
                onClick={() => persist({ ...settings, systemPrompt: defaultPrompt })}
                className="rounded-xl border border-slate-200 px-3 py-2 text-xs font-bold text-slate-700 hover:bg-slate-50"
              >
                恢复默认
              </button>
            </div>
            <textarea
              value={settings.systemPrompt || defaultPrompt}
              onChange={event => persist({ ...settings, systemPrompt: event.target.value })}
              rows={10}
              className="mt-3 w-full rounded-xl border border-slate-200 px-3 py-2 font-mono text-xs leading-relaxed outline-none focus:border-sky-400"
            />
          </section>
        </div>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          <section className="space-y-3 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
            <div
              onPaste={onPaste}
              onDragOver={event => event.preventDefault()}
              onDrop={event => {
                event.preventDefault();
                const file = event.dataTransfer.files[0];
                if (file) readImage(file);
              }}
              className="flex min-h-[180px] cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed border-slate-200 bg-slate-50 p-4 text-center"
              onClick={() => fileRef.current?.click()}
            >
              {form.image ? (
                <img src={form.image} alt="原图" className="max-h-56 rounded-lg object-contain" />
              ) : (
                <>
                  <ImagePlus className="h-6 w-6 text-slate-400" />
                  <div className="text-sm font-bold text-slate-700">点击、拖入或 Ctrl+V 粘贴原图</div>
                  <div className="text-xs text-slate-500">不上传原图则按文案直接生成新封面</div>
                </>
              )}
            </div>
            <input
              ref={fileRef}
              type="file"
              accept="image/*"
              className="hidden"
              onChange={event => {
                const file = event.target.files?.[0];
                if (file) readImage(file);
              }}
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block">
                <span className="text-xs font-bold text-slate-600">主标题</span>
                <input value={form.headline} onChange={event => setForm({ ...form, headline: event.target.value })}
                  className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
              </label>
              <label className="block">
                <span className="text-xs font-bold text-slate-600">价格</span>
                <input value={form.price} onChange={event => setForm({ ...form, price: event.target.value })}
                  className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
              </label>
            </div>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">卖点（每行一条）</span>
              <textarea
                value={pointsText}
                onChange={event => {
                  setPointsText(event.target.value);
                  setForm(prev => ({ ...prev, points: event.target.value.split('\n').map(line => line.trim()).filter(Boolean) }));
                }}
                rows={3}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400"
              />
            </label>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">补充要求</span>
              <input value={form.extra} onChange={event => setForm({ ...form, extra: event.target.value })}
                placeholder="例如：价格用红色，保留右下角角标"
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
            <label className="block">
              <span className="flex items-center justify-between text-xs font-bold text-slate-600">
                完整文案
                <button
                  type="button"
                  onClick={handlePolish}
                  disabled={polishing}
                  className="inline-flex items-center gap-1 rounded-lg border border-slate-200 px-2 py-1 text-[11px] font-bold text-slate-600 hover:bg-slate-50 disabled:opacity-50"
                >
                  {polishing ? <Loader2 className="h-3 w-3 animate-spin" /> : <Sparkles className="h-3 w-3" />}
                  润色成闲鱼口吻
                </button>
              </span>
              <textarea value={form.copy} onChange={event => setForm({ ...form, copy: event.target.value })} rows={3}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">一键发货文字</span>
              <textarea value={form.shipping} onChange={event => setForm({ ...form, shipping: event.target.value })} rows={3}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
            <div className="flex flex-wrap items-center gap-2">
              <button
                type="button"
                onClick={handleGenerate}
                disabled={busy}
                className="inline-flex items-center gap-2 rounded-xl bg-sky-600 px-4 py-2 text-sm font-bold text-white hover:bg-sky-700 disabled:opacity-60"
              >
                {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Wand2 className="h-4 w-4" />}
                生成主图
              </button>
              <button type="button" onClick={handlePreview}
                className="inline-flex items-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-xs font-bold text-slate-700 hover:bg-slate-50">
                <Sparkles className="h-3.5 w-3.5" />预览提示词
              </button>
              {busy && <span className="text-xs font-semibold text-slate-500">上游出图通常 20–40 秒，请勿关闭页面</span>}
            </div>
            {promptPreview && (
              <pre className="max-h-52 overflow-auto rounded-xl bg-slate-900 p-3 text-[11px] leading-relaxed text-slate-100">{promptPreview}</pre>
            )}
          </section>

          <section className="space-y-3 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
            <h2 className="text-base font-black text-slate-900">结果与上架</h2>
            {result ? (
              <>
                <img src={result.image} alt="结果图" className="w-full rounded-xl border border-slate-200 object-contain" />
                <div className="flex flex-wrap items-center gap-2 text-xs font-semibold text-slate-500">
                  <span>模型 {result.model}</span>
                  <span>·</span>
                  <span>{result.seconds.toFixed(0)} 秒</span>
                  <a href={downloadHref} download="studio-cover.png"
                    className="ml-auto inline-flex items-center gap-1 rounded-lg border border-slate-200 px-2 py-1 text-slate-700 hover:bg-slate-50">
                    <Download className="h-3.5 w-3.5" />下载
                  </a>
                </div>
                <div className="rounded-xl border border-slate-200 bg-slate-50 p-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="text-xs font-bold text-slate-600">
                      发布账号：已登录 {accounts.length} 个，已勾选 {publish.cookieIds.length} 个
                    </span>
                    <div className="flex items-center gap-1">
                      <button type="button" onClick={() => setPublish(prev => ({ ...prev, cookieIds: accounts.map(item => item.id) }))}
                        className="rounded-lg border border-slate-200 bg-white px-2 py-1 text-[11px] font-bold text-slate-600 hover:bg-slate-100">全选</button>
                      <button type="button" onClick={() => setPublish(prev => ({ ...prev, cookieIds: [] }))}
                        className="rounded-lg border border-slate-200 bg-white px-2 py-1 text-[11px] font-bold text-slate-600 hover:bg-slate-100">清空</button>
                    </div>
                  </div>
                  {accounts.length === 0 ? (
                    <p className="mt-2 text-xs font-medium text-slate-500">没有识别到已登录账号，请先在「账号管理」扫码登录闲鱼账号。</p>
                  ) : (
                    <div className="mt-2 grid gap-2 sm:grid-cols-2">
                      {accounts.map(item => {
                        // checked 表示该账号已被勾选发布。
                        const checked = publish.cookieIds.includes(item.id);
                        return (
                          <label key={item.id}
                            className={`flex cursor-pointer items-center gap-2 rounded-lg border px-2.5 py-2 text-sm transition ${checked ? 'border-emerald-300 bg-emerald-50' : 'border-slate-200 bg-white hover:border-slate-300'}`}>
                            <input type="checkbox" checked={checked} onChange={() => toggleAccount(item.id)}
                              className="h-4 w-4 shrink-0 accent-emerald-600" />
                            <span className="truncate font-semibold text-slate-700">{accountLabel(item)}</span>
                            <span className="ml-auto shrink-0 text-[11px] font-medium text-slate-400">{item.id}</span>
                            {item.enabled === false && <span className="shrink-0 rounded bg-amber-100 px-1 text-[10px] font-bold text-amber-700">自动化停用</span>}
                          </label>
                        );
                      })}
                    </div>
                  )}
                  <p className="mt-2 text-[11px] font-medium text-slate-500">勾选 1 个账号即单账号发布；勾选多个账号会用同一张图、同一份文案依次发布到每个账号（账号之间间隔 1.5 秒）。</p>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <label className="block">
                    <span className="text-xs font-bold text-slate-600">售价</span>
                    <input value={publish.price} onChange={event => setPublish({ ...publish, price: event.target.value })}
                      className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
                  </label>
                  <label className="block">
                    <span className="text-xs font-bold text-slate-600">库存件数</span>
                    <input value={publish.quantity} inputMode="numeric" onChange={event => setPublish({ ...publish, quantity: event.target.value })}
                      className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
                  </label>
                </div>
                <label className="block">
                  <span className="text-xs font-bold text-slate-600">标题</span>
                  <input value={publish.title} onChange={event => setPublish({ ...publish, title: event.target.value })}
                    className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
                </label>
                <label className="block">
                  <span className="text-xs font-bold text-slate-600">描述（第一行会被当作标题展示）</span>
                  <textarea value={publish.description} onChange={event => setPublish({ ...publish, description: event.target.value })} rows={6}
                    className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
                </label>
                <button
                  type="button"
                  onClick={handlePublish}
                  disabled={publishing}
                  className="inline-flex items-center gap-2 rounded-xl bg-emerald-600 px-4 py-2 text-sm font-bold text-white hover:bg-emerald-700 disabled:opacity-60"
                >
                  {publishing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                  {publishing ? (publishProgress || '发布中…') : `用这张图发布${publish.cookieIds.length > 1 ? `到 ${publish.cookieIds.length} 个账号` : ''}`}
                </button>
                {publishResults.length > 0 && (
                  <ul className="space-y-1">
                    {publishResults.map(item => (
                      <li key={item.accountId}
                        className={`rounded-lg px-3 py-2 text-xs font-semibold ${item.ok ? 'bg-emerald-50 text-emerald-700' : 'bg-rose-50 text-rose-700'}`}>
                        {item.ok ? '✓' : '✕'} {item.label}：{item.detail}
                      </li>
                    ))}
                  </ul>
                )}
                <p className="text-xs font-medium text-slate-500">发布走闲鱼助手既有通道，类目由服务端按电子资料兜底，运费自动包邮。库存默认 1 件：平台对「新建多件库存商品」有账号与类目限制，填多件若被拒绝会明确提示，改成 1 件即可；商品上架后由自动补货巡检把售罄商品补回库存。</p>
              </>
            ) : (
              <div className="flex min-h-[240px] flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-slate-200 bg-slate-50 text-center">
                <Sparkles className="h-6 w-6 text-slate-400" />
                <div className="text-sm font-bold text-slate-600">还没有生成结果</div>
                <div className="text-xs text-slate-500">左侧填好文案并生成后，这里会显示主图与上架表单</div>
              </div>
            )}
          </section>
        </div>
      )}
    </div>
  );
};

export default StudioPage;
