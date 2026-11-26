import React, { useCallback, useEffect, useState } from 'react';
import { Loader2, Play, Plus, Save, Settings2, Trash2, Workflow as WorkflowIcon } from 'lucide-react';
import { PageHeader } from '../../../../shared/ui/PageHeader';
import { listAccounts } from '../../studio/api';
import type { StudioAccount } from '../../studio/types';
import {
  createTask, deleteTask, getModels, listLogs, listTasks, runTask, saveModels, toggleTask, updateTask,
} from '../api';
import type { WorkflowLog, WorkflowModelConfig, WorkflowTask, WorkflowTaskInput } from '../api';

/** accountLabel 返回账号展示名。 */
const accountLabel = (account: StudioAccount): string =>
  account.nickname || account.remark || account.username || account.id;

/** emptyInput 返回新建任务的默认表单。 */
const emptyInput = (): WorkflowTaskInput & { id?: number } => ({
  id: 0, keyword: '', searchCookieId: '', publishCookieIds: [],
  priceMinCents: 0, priceMaxCents: 0, targetCount: 2, intervalMinutes: 120, publishGapSeconds: 30,
  useSourceImage: false, priceCents: 0, quantity: 1, enabled: false,
});

/** centsToYuan 分转元文本。 */
const centsToYuan = (cents: number): string => (cents > 0 ? String(cents / 100) : '');

/** yuanToCents 元文本转分。 */
const yuanToCents = (yuan: string): number => {
  const value = Number(yuan);
  if (!Number.isFinite(value) || value <= 0) return 0;
  return Math.round(value * 100);
};

/** ModelPanel 是一组模型配置输入。 */
const ModelPanel: React.FC<{
  title: string;
  hint: string;
  value: WorkflowModelConfig;
  onChange: (next: WorkflowModelConfig) => void;
}> = ({ title, hint, value, onChange }) => (
  <div className="grid gap-3 md:grid-cols-2">
    <label className="block">
      <span className="text-xs font-bold text-slate-600">{title} Base URL</span>
      <input value={value.baseUrl} onChange={event => onChange({ ...value, baseUrl: event.target.value })}
        placeholder="https://api.example.com/v1"
        className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
    </label>
    <label className="block">
      <span className="text-xs font-bold text-slate-600">{title} 密钥</span>
      <input type="password" value={value.apiKey} onChange={event => onChange({ ...value, apiKey: event.target.value })}
        placeholder="sk-..."
        className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
    </label>
    <label className="block">
      <span className="text-xs font-bold text-slate-600">{title} 模型名</span>
      <input value={value.model} onChange={event => onChange({ ...value, model: event.target.value })}
        placeholder={hint}
        className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
    </label>
    <label className="block">
      <span className="text-xs font-bold text-slate-600">{title} 备用模型</span>
      <input value={value.fallback} onChange={event => onChange({ ...value, fallback: event.target.value })}
        className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
    </label>
  </div>
);

/** WorkflowPage 是商品工作流：关键词选品 + 定时 + 多账号自动铺货。 */
const WorkflowPage: React.FC = () => {
  /** accounts 是已登录账号。 */
  const [accounts, setAccounts] = useState<StudioAccount[]>([]);
  /** tasks 是任务列表。 */
  const [tasks, setTasks] = useState<WorkflowTask[]>([]);
  /** loading 表示任务列表加载中。 */
  const [loading, setLoading] = useState(true);
  /** form 是新建/编辑表单（0 表示新建）。 */
  const [form, setForm] = useState<(WorkflowTaskInput & { id?: number }) | null>(null);
  /** saving 表示表单保存中。 */
  const [saving, setSaving] = useState(false);
  /** error 是错误提示。 */
  const [error, setError] = useState('');
  /** notice 是成功提示。 */
  const [notice, setNotice] = useState('');
  /** showModels 表示模型配置面板展开。 */
  const [showModels, setShowModels] = useState(false);
  /** textModel、imageModel 是服务端模型配置。 */
  const [textModel, setTextModel] = useState<WorkflowModelConfig>({ baseUrl: '', apiKey: '', model: '', fallback: '' });
  const [imageModel, setImageModel] = useState<WorkflowModelConfig>({ baseUrl: '', apiKey: '', model: '', fallback: '' });
  /** modelsSaving 表示模型保存中。 */
  const [modelsSaving, setModelsSaving] = useState(false);
  /** logsTaskId 表示当前查看日志的任务。 */
  const [logsTaskId, setLogsTaskId] = useState(0);
  /** logs 是运行日志。 */
  const [logs, setLogs] = useState<WorkflowLog[]>([]);
  /** busyTaskIds 记录触发中的任务，避免重复点击。 */
  const [busyTaskIds, setBusyTaskIds] = useState<number[]>([]);

  /** refresh 刷新任务列表。 */
  const refresh = useCallback(async () => {
    try {
      setTasks(await listTasks());
    } catch {
      // 列表加载失败保持现状，错误由具体操作提示。
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    listAccounts().then(setAccounts).catch(() => undefined);
    refresh();
    getModels().then(data => {
      setTextModel(data.text ?? { baseUrl: '', apiKey: '', model: '', fallback: '' });
      setImageModel(data.image ?? { baseUrl: '', apiKey: '', model: '', fallback: '' });
    }).catch(() => undefined);
  }, [refresh]);

  /** togglePublishAccount 在表单里勾选/取消发布账号。 */
  const togglePublishAccount = (accountId: string) => {
    setForm(prev => prev ? {
      ...prev,
      publishCookieIds: prev.publishCookieIds.includes(accountId)
        ? prev.publishCookieIds.filter(id => id !== accountId)
        : [...prev.publishCookieIds, accountId],
    } : prev);
  };

  /** saveForm 保存新建或编辑。 */
  const saveForm = useCallback(async () => {
    if (!form) return;
    if (!form.keyword.trim()) { setError('请填写关键词'); return; }
    if (!form.searchCookieId) { setError('请选择搜索账号'); return; }
    if (form.publishCookieIds.length === 0) { setError('请至少勾选一个发布账号'); return; }
    setSaving(true);
    setError('');
    try {
      if (form.id && form.id > 0) {
        await updateTask(form.id, form);
        setNotice('任务已更新');
      } else {
        await createTask(form);
        setNotice('任务已创建');
      }
      setForm(null);
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败');
    } finally {
      setSaving(false);
    }
  }, [form, refresh]);

  /** editTask 打开编辑表单。 */
  const editTask = (task: WorkflowTask) => {
    setForm({
      id: task.id, keyword: task.keyword, searchCookieId: task.searchCookieId,
      publishCookieIds: [...task.publishCookieIds], priceMinCents: task.priceMinCents, priceMaxCents: task.priceMaxCents,
      targetCount: task.targetCount, intervalMinutes: task.intervalMinutes, publishGapSeconds: task.publishGapSeconds,
      useSourceImage: task.useSourceImage, priceCents: task.priceCents, quantity: task.quantity, enabled: task.enabled,
    });
    setNotice('');
    setError('');
  };

  /** onToggle 启停任务。 */
  const onToggle = async (task: WorkflowTask) => {
    try {
      await toggleTask(task.id, !task.enabled);
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败');
    }
  };

  /** onRun 手动触发一次。 */
  const onRun = async (task: WorkflowTask) => {
    setBusyTaskIds(prev => [...prev, task.id]);
    setError('');
    setNotice('');
    try {
      await runTask(task.id);
      setNotice(`已触发「${task.keyword}」执行，一轮约需 1-3 分钟，稍后刷新日志查看结果`);
    } catch (err) {
      setError(err instanceof Error ? err.message : '触发失败');
    } finally {
      setBusyTaskIds(prev => prev.filter(id => id !== task.id));
    }
  };

  /** onDelete 删除任务。 */
  const onDelete = async (task: WorkflowTask) => {
    if (!window.confirm(`删除任务「${task.keyword}」？已发布的商品不受影响。`)) return;
    try {
      await deleteTask(task.id);
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败');
    }
  };

  /** loadLogs 查看任务日志。 */
  const loadLogs = async (taskId: number) => {
    setLogsTaskId(taskId);
    try {
      setLogs(await listLogs(taskId));
    } catch {
      setLogs([]);
    }
  };

  /** saveModelsConfig 保存模型配置。 */
  const saveModelsConfig = async () => {
    setModelsSaving(true);
    setError('');
    try {
      await saveModels(textModel, imageModel);
      setNotice('模型配置已保存（服务端存储，工作流无人值守运行时使用）');
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败');
    } finally {
      setModelsSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader title="商品工作流" description="按关键词定时选品，AI 改写 + 生图后自动发布到多个账号；同一账号不会重复发同一源商品。">
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => { setForm(emptyInput()); setError(''); setNotice(''); }}
            className="inline-flex items-center gap-2 rounded-xl bg-sky-600 px-3 py-2 text-xs font-bold text-white hover:bg-sky-700">
            <Plus className="h-3.5 w-3.5" />新建任务
          </button>
          <button type="button" onClick={() => setShowModels(prev => !prev)}
            className="inline-flex items-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-xs font-bold text-slate-700 hover:bg-slate-50">
            <Settings2 className="h-3.5 w-3.5" />模型配置
          </button>
        </div>
      </PageHeader>

      {error && <div className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm font-semibold text-rose-700">{error}</div>}
      {notice && <div className="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm font-semibold text-emerald-700">{notice}</div>}

      {showModels && (
        <section className="space-y-3 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
          <div>
            <h2 className="text-base font-black text-slate-900">AI 模型配置（服务端）</h2>
            <p className="mt-1 text-xs font-medium text-slate-500">工作流无人值守运行，模型配置保存在服务端。文字模型用于改写文案（可不配，沿用源文案）；图片模型用于生成封面（不配且任务未勾选源图时会跳过商品）。</p>
          </div>
          <ModelPanel title="文字模型" hint="如 gpt-4o-mini" value={textModel} onChange={setTextModel} />
          <ModelPanel title="图片模型" hint="如 gpt-image-2" value={imageModel} onChange={setImageModel} />
          <button type="button" onClick={saveModelsConfig} disabled={modelsSaving}
            className="inline-flex items-center gap-2 rounded-xl bg-emerald-600 px-4 py-2 text-sm font-bold text-white hover:bg-emerald-700 disabled:opacity-60">
            {modelsSaving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            保存模型配置
          </button>
        </section>
      )}

      {form && (
        <section className="space-y-4 rounded-2xl border border-sky-200 bg-white p-5 shadow-sm">
          <div className="flex items-center justify-between">
            <h2 className="text-base font-black text-slate-900">{form.id && form.id > 0 ? '编辑任务' : '新建任务'}</h2>
            <button type="button" onClick={() => setForm(null)}
              className="text-xs font-bold text-slate-400 hover:text-slate-600">取消</button>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <label className="block">
              <span className="text-xs font-bold text-slate-600">选品关键词</span>
              <input value={form.keyword} onChange={event => setForm({ ...form, keyword: event.target.value })}
                placeholder="例如：露营车、考研资料"
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">搜索账号（一号选品）</span>
              <select value={form.searchCookieId} onChange={event => setForm({ ...form, searchCookieId: event.target.value })}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400">
                <option value="">选择账号</option>
                {accounts.map(item => <option key={item.id} value={item.id}>{accountLabel(item)}</option>)}
              </select>
            </label>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">价格下限（元，0 不限）</span>
              <input value={centsToYuan(form.priceMinCents)} inputMode="decimal"
                onChange={event => setForm({ ...form, priceMinCents: yuanToCents(event.target.value) })}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">价格上限（元，0 不限）</span>
              <input value={centsToYuan(form.priceMaxCents)} inputMode="decimal"
                onChange={event => setForm({ ...form, priceMaxCents: yuanToCents(event.target.value) })}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">每轮上架数量（1-10 件）</span>
              <input value={form.targetCount} inputMode="numeric"
                onChange={event => setForm({ ...form, targetCount: Number(event.target.value) || 1 })}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">运行间隔（分钟，≥10）</span>
              <input value={form.intervalMinutes} inputMode="numeric"
                onChange={event => setForm({ ...form, intervalMinutes: Number(event.target.value) || 120 })}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">发布间隔（秒，同账号错峰）</span>
              <input value={form.publishGapSeconds} inputMode="numeric"
                onChange={event => setForm({ ...form, publishGapSeconds: Number(event.target.value) || 30 })}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
            <label className="block">
              <span className="text-xs font-bold text-slate-600">固定售价（元，0 用源价）</span>
              <input value={centsToYuan(form.priceCents)} inputMode="decimal"
                onChange={event => setForm({ ...form, priceCents: yuanToCents(event.target.value) })}
                className="mt-1 w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none focus:border-sky-400" />
            </label>
          </div>
          <label className="flex cursor-pointer items-center gap-2 rounded-xl border border-slate-200 px-3 py-2">
            <input type="checkbox" checked={form.useSourceImage} onChange={event => setForm({ ...form, useSourceImage: event.target.checked })}
              className="h-4 w-4 accent-emerald-600" />
            <span className="text-xs font-bold text-slate-700">用源商品图直接发布（不 AI 生图，更快但需自行把控图片版权）</span>
          </label>
          <div className="rounded-xl border border-slate-200 bg-slate-50 p-3">
            <span className="text-xs font-bold text-slate-600">发布账号（多账号铺货，勾选 {form.publishCookieIds.length} 个）</span>
            <div className="mt-2 grid gap-2 sm:grid-cols-2">
              {accounts.map(item => {
                const checked = form.publishCookieIds.includes(item.id);
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
          <button type="button" onClick={saveForm} disabled={saving}
            className="inline-flex items-center gap-2 rounded-xl bg-sky-600 px-4 py-2 text-sm font-bold text-white hover:bg-sky-700 disabled:opacity-60">
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            {form.id && form.id > 0 ? '保存修改' : '创建任务'}
          </button>
        </section>
      )}

      <section className="space-y-3">
        {loading ? (
          <div className="flex min-h-[160px] items-center justify-center rounded-2xl border border-slate-200 bg-white">
            <Loader2 className="h-5 w-5 animate-spin text-slate-400" />
          </div>
        ) : tasks.length === 0 ? (
          <div className="flex min-h-[240px] flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-slate-200 bg-slate-50 text-center">
            <WorkflowIcon className="h-6 w-6 text-slate-400" />
            <div className="text-sm font-bold text-slate-600">还没有工作流任务</div>
            <div className="text-xs text-slate-500">新建任务：选关键词 → 勾发布账号 → 开启后按间隔自动选品上架</div>
          </div>
        ) : (
          tasks.map(task => (
            <div key={task.id} className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
              <div className="flex flex-wrap items-center gap-3">
                <span className={`rounded-lg px-2 py-1 text-[11px] font-black ${task.enabled ? 'bg-emerald-100 text-emerald-700' : 'bg-slate-100 text-slate-500'}`}>
                  {task.enabled ? '运行中' : '已停用'}
                </span>
                <span className="text-sm font-black text-slate-800">{task.keyword}</span>
                <span className="text-xs font-semibold text-slate-500">
                  每轮 {task.targetCount} 件 · 每 {task.intervalMinutes} 分钟 · {task.publishCookieIds.length} 个账号{task.useSourceImage ? ' · 源图直发' : ' · AI 生图'}
                </span>
                <div className="ml-auto flex items-center gap-1">
                  <button type="button" onClick={() => onToggle(task)}
                    className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs font-bold text-slate-700 hover:bg-slate-50">
                    {task.enabled ? '停用' : '启用'}
                  </button>
                  <button type="button" onClick={() => onRun(task)} disabled={busyTaskIds.includes(task.id)}
                    className="inline-flex items-center gap-1 rounded-lg bg-sky-600 px-2.5 py-1.5 text-xs font-bold text-white hover:bg-sky-700 disabled:opacity-50">
                    {busyTaskIds.includes(task.id) ? <Loader2 className="h-3 w-3 animate-spin" /> : <Play className="h-3 w-3" />}
                    立即运行
                  </button>
                  <button type="button" onClick={() => editTask(task)}
                    className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs font-bold text-slate-700 hover:bg-slate-50">编辑</button>
                  <button type="button" onClick={() => loadLogs(task.id)}
                    className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs font-bold text-slate-700 hover:bg-slate-50">日志</button>
                  <button type="button" onClick={() => onDelete(task)}
                    className="rounded-lg border border-rose-200 px-2.5 py-1.5 text-xs font-bold text-rose-600 hover:bg-rose-50">
                    <Trash2 className="h-3 w-3" />
                  </button>
                </div>
              </div>
              <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[11px] font-medium text-slate-500">
                {task.priceMinCents > 0 && <span>价格 ≥ ¥{task.priceMinCents / 100}</span>}
                {task.priceMaxCents > 0 && <span>价格 ≤ ¥{task.priceMaxCents / 100}</span>}
                {task.priceCents > 0 && <span>固定售价 ¥{task.priceCents / 100}</span>}
                {task.lastRunAt && <span>上次运行 {task.lastRunAt}</span>}
                {task.nextRunAt && task.enabled && <span>下次运行 {task.nextRunAt}</span>}
              </div>
            </div>
          ))
        )}
      </section>

      {logsTaskId > 0 && (
        <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
          <div className="flex items-center justify-between">
            <h2 className="text-base font-black text-slate-900">运行日志（任务 #{logsTaskId}）</h2>
            <div className="flex items-center gap-2">
              <button type="button" onClick={() => loadLogs(logsTaskId)}
                className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs font-bold text-slate-700 hover:bg-slate-50">刷新</button>
              <button type="button" onClick={() => { setLogsTaskId(0); setLogs([]); }}
                className="text-xs font-bold text-slate-400 hover:text-slate-600">关闭</button>
            </div>
          </div>
          {logs.length === 0 ? (
            <p className="mt-3 text-xs font-medium text-slate-500">暂无日志；触发运行后约 1-3 分钟产生记录。</p>
          ) : (
            <ul className="mt-3 space-y-1">
              {logs.map(entry => (
                <li key={entry.id}
                  className={`rounded-lg px-3 py-2 text-xs font-medium ${entry.level === 'error' ? 'bg-rose-50 text-rose-700' : entry.level === 'warn' ? 'bg-amber-50 text-amber-700' : 'bg-slate-50 text-slate-600'}`}>
                  <span className="mr-2 font-bold text-slate-400">{entry.createdAt}</span>{entry.message}
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </div>
  );
};

export default WorkflowPage;
