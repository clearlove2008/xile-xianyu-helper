import React, { useMemo, useState } from 'react';
import { Check, Copy, ExternalLink, Star } from 'lucide-react';
import type { PanSouMergedByType, PanSouMergedLink } from '../types';

/** PAGE_SIZE 是单个网盘类型页签下首次渲染的条数。 */
const PAGE_SIZE = 20;

/** 网盘类型展示顺序：优先展示使用频率高的类型。 */
const SOURCE_ORDER = ['quark', 'baidu', 'aliyun', 'uc', 'xunlei', '123', '115', 'tianyi', 'mobile', 'pikpak', 'magnet', 'ed2k', 'others'];

/** SOURCE_LABEL 是网盘类型的中文名称。 */
const SOURCE_LABEL: Record<string, string> = {
  baidu: '百度网盘',
  aliyun: '阿里云盘',
  quark: '夸克网盘',
  tianyi: '天翼云盘',
  uc: 'UC网盘',
  mobile: '移动云盘',
  115: '115网盘',
  pikpak: 'PikPak',
  xunlei: '迅雷网盘',
  123: '123网盘',
  magnet: '磁力链接',
  ed2k: '电驴链接',
  others: '其他网盘',
};

/** sourceLabel 返回网盘类型的中文名称，未知类型回退为原始键。 */
export const sourceLabel = (key: string): string => SOURCE_LABEL[key] ?? key;

/** formatTime 把 ISO 时间渲染为本地日期，无法解析时返回空串。 */
const formatTime = (value: string): string => {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
};

/** sortSources 按固定优先级排序网盘类型页签。 */
const sortSources = (keys: string[]): string[] =>
  [...keys].sort((a, b) => {
    const indexA = SOURCE_ORDER.indexOf(a);
    const indexB = SOURCE_ORDER.indexOf(b);
    if (indexA >= 0 && indexB >= 0) return indexA - indexB;
    if (indexA >= 0) return -1;
    if (indexB >= 0) return 1;
    return a.localeCompare(b);
  });

interface ResultListProps {
  /** mergedByType 是按网盘类型分组的结果。 */ mergedByType: PanSouMergedByType;
  /** onFollow 把单条资源加入关注。 */ onFollow: (item: PanSouMergedLink, sourceType: string) => void;
}

// ResultList 渲染按网盘类型分组的结果，支持分页与复制链接。
export const ResultList: React.FC<ResultListProps> = ({ mergedByType, onFollow }) => {
  const sources = useMemo(() => sortSources(Object.keys(mergedByType)), [mergedByType]);
  const [activeSource, setActiveSource] = useState<string | null>(null);
  const [visibleCount, setVisibleCount] = useState(PAGE_SIZE);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  // active 是当前选中的网盘类型；未选择或已失效时回退到第一个有内容的类型。
  const active = activeSource && sources.includes(activeSource) ? activeSource : sources[0] ?? null;
  const activeList = active ? mergedByType[active] ?? [] : [];
  const visibleList = activeList.slice(0, visibleCount);

  /** handleSelect 切换页签并重置分页。 */
  const handleSelect = (source: string): void => {
    setActiveSource(source);
    setVisibleCount(PAGE_SIZE);
  };

  /** handleCopy 复制「链接 + 提取码」，并短暂提示成功。 */
  const handleCopy = async (item: PanSouMergedLink, key: string): Promise<void> => {
    const text = item.password ? `${item.url} 提取码：${item.password}` : item.url;
    try {
      await navigator.clipboard.writeText(text);
      setCopiedKey(key);
      window.setTimeout(() => setCopiedKey(current => (current === key ? null : current)), 1800);
    } catch {
      // 剪贴板不可用时保持静默，用户仍可手动选中复制。
    }
  };

  if (sources.length === 0) return null;

  return (
    <section className="rounded-lg border border-gray-200 bg-white">
      <div className="flex flex-wrap gap-1.5 border-b border-gray-200 px-4 py-3">
        {sources.map(source => {
          const count = mergedByType[source]?.length ?? 0;
          const selected = source === active;
          return (
            <button
              key={source}
              type="button"
              onClick={() => handleSelect(source)}
              className={`rounded-md px-3 py-1.5 text-xs font-bold transition ${
                selected ? 'bg-gray-900 text-white' : 'bg-gray-50 text-gray-600 hover:bg-gray-100'
              }`}
            >
              {sourceLabel(source)} ({count})
            </button>
          );
        })}
      </div>

      <ul className="divide-y divide-gray-100">
        {visibleList.map(item => {
          const key = `${active}-${item.url}`;
          const copied = copiedKey === key;
          return (
            <li key={key} className="px-4 py-3 transition hover:bg-gray-50/70">
              <div className="flex flex-col gap-2 md:flex-row md:items-start md:justify-between md:gap-4">
                <div className="min-w-0 flex-1">
                  <p className="line-clamp-2 text-sm font-semibold text-gray-900">{item.note || '未命名资源'}</p>
                  <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500">
                    <span className="truncate">{item.source || '未知来源'}</span>
                    {formatTime(item.datetime) && <span>{formatTime(item.datetime)}</span>}
                    {item.password && (
                      <span className="rounded bg-amber-50 px-1.5 py-0.5 font-mono font-bold text-amber-700">提取码 {item.password}</span>
                    )}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-1.5">
                  <a
                    href={item.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-bold text-gray-700 transition hover:bg-gray-50"
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                    打开
                  </a>
                  <button
                    type="button"
                    onClick={() => void handleCopy(item, key)}
                    className="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-bold text-gray-700 transition hover:bg-gray-50"
                  >
                    {copied ? <Check className="h-3.5 w-3.5 text-emerald-600" /> : <Copy className="h-3.5 w-3.5" />}
                    {copied ? '已复制' : '复制'}
                  </button>
                  <button
                    type="button"
                    onClick={() => onFollow(item, active ?? '')}
                    title="加入关注，便于后续追更"
                    className="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-bold text-gray-700 transition hover:bg-gray-50"
                  >
                    <Star className="h-3.5 w-3.5" />
                    关注
                  </button>
                </div>
              </div>
            </li>
          );
        })}
      </ul>

      {activeList.length > visibleCount && (
        <div className="border-t border-gray-200 px-4 py-3 text-center">
          <button
            type="button"
            onClick={() => setVisibleCount(count => count + PAGE_SIZE)}
            className="rounded-md border border-gray-300 bg-white px-4 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-gray-50"
          >
            加载更多（已显示 {visibleList.length}/{activeList.length}）
          </button>
        </div>
      )}
    </section>
  );
};
