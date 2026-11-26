import React, { useState } from 'react';
import { Clock, History, RefreshCw, Star, Trash2, X } from 'lucide-react';
import type { PanSouFollowItem, PanSouHistoryItem } from '../types';
import { sourceLabel } from './ResultList';

/** COLLAPSED_COUNT 是历史面板折叠时展示的条数。 */
const COLLAPSED_COUNT = 8;

interface HistoryPanelProps {
  /** items 是搜索历史列表。 */ items: PanSouHistoryItem[];
  /** onSearch 用历史关键词重新检索。 */ onSearch: (keyword: string) => void;
  /** onRemove 删除单条历史。 */ onRemove: (keyword: string) => void;
  /** onClear 清空历史。 */ onClear: () => void;
}

// HistoryPanel 展示本地搜索历史，支持一键重搜与清理。
export const HistoryPanel: React.FC<HistoryPanelProps> = ({ items, onSearch, onRemove, onClear }) => {
  const [expanded, setExpanded] = useState(false);
  const visible = expanded ? items : items.slice(0, COLLAPSED_COUNT);

  return (
    <section className="rounded-lg border border-gray-200 bg-white">
      <header className="flex items-center justify-between px-4 py-3">
        <h2 className="flex items-center gap-1.5 text-sm font-bold text-gray-900">
          <History className="h-4 w-4 text-gray-400" />
          搜索历史
          <span className="text-xs font-medium text-gray-400">({items.length})</span>
        </h2>
        {items.length > 0 && (
          <button type="button" onClick={onClear} className="text-xs font-medium text-gray-400 transition hover:text-red-600">
            清空
          </button>
        )}
      </header>
      {items.length === 0 ? (
        <p className="px-4 pb-4 text-xs text-gray-400">还没有搜索记录。</p>
      ) : (
        <ul className="space-y-0.5 px-2 pb-2">
          {visible.map(item => (
            <li key={item.keyword} className="group flex items-center gap-1">
              <button
                type="button"
                onClick={() => onSearch(item.keyword)}
                className="min-w-0 flex-1 truncate rounded px-2 py-1.5 text-left text-xs font-medium text-gray-700 transition hover:bg-gray-50"
                title={item.keyword}
              >
                {item.keyword}
              </button>
              <button
                type="button"
                onClick={() => onRemove(item.keyword)}
                aria-label={`删除历史 ${item.keyword}`}
                className="shrink-0 rounded p-1 text-gray-300 opacity-0 transition hover:text-red-600 group-hover:opacity-100"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </li>
          ))}
        </ul>
      )}
      {items.length > COLLAPSED_COUNT && (
        <div className="border-t border-gray-100 px-4 py-2">
          <button type="button" onClick={() => setExpanded(value => !value)} className="text-xs font-medium text-gray-500 hover:text-gray-900">
            {expanded ? '收起' : `展开全部 ${items.length} 条`}
          </button>
        </div>
      )}
    </section>
  );
};

interface FollowPanelProps {
  /** items 是关注列表。 */ items: PanSouFollowItem[];
  /** refreshingKey 是正在刷新的关注项键。 */ refreshingKey: string | null;
  /** onRefresh 刷新单条关注的最新资源。 */ onRefresh: (item: PanSouFollowItem) => void;
  /** onRemove 取消关注。 */ onRemove: (key: string) => void;
  /** onClear 清空关注。 */ onClear: () => void;
}

// FollowPanel 展示已关注的资源，并对比链接是否更新。
export const FollowPanel: React.FC<FollowPanelProps> = ({ items, refreshingKey, onRefresh, onRemove, onClear }) => (
  <section className="rounded-lg border border-gray-200 bg-white">
    <header className="flex items-center justify-between px-4 py-3">
      <h2 className="flex items-center gap-1.5 text-sm font-bold text-gray-900">
        <Star className="h-4 w-4 text-gray-400" />
        资源关注
        <span className="text-xs font-medium text-gray-400">({items.length})</span>
      </h2>
      {items.length > 0 && (
        <button type="button" onClick={onClear} className="text-xs font-medium text-gray-400 transition hover:text-red-600">
          清空
        </button>
      )}
    </header>
    {items.length === 0 ? (
      <p className="px-4 pb-4 text-xs text-gray-400">在结果中点击「关注」，后续可一键检查资源是否更新。</p>
    ) : (
      <ul className="divide-y divide-gray-100">
        {items.map(item => (
          <li key={item.key} className="px-4 py-2.5">
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <p className="truncate text-xs font-bold text-gray-900" title={item.keyword}>
                  {item.keyword}
                </p>
                <p className="mt-0.5 flex items-center gap-1.5 text-[11px] text-gray-500">
                  <span className="rounded bg-gray-100 px-1.5 py-0.5 font-medium">{sourceLabel(item.sourceType)}</span>
                  {item.linkUnchanged ? '链接未变' : '有更新'}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-0.5">
                <button
                  type="button"
                  onClick={() => onRefresh(item)}
                  disabled={refreshingKey === item.key}
                  title="检查更新"
                  className="rounded p-1 text-gray-400 transition hover:bg-gray-50 hover:text-gray-900 disabled:opacity-40"
                >
                  <RefreshCw className={`h-3.5 w-3.5 ${refreshingKey === item.key ? 'animate-spin' : ''}`} />
                </button>
                <a
                  href={item.lastSnapshot.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  title="打开最近一次命中的资源"
                  className="rounded p-1 text-gray-400 transition hover:bg-gray-50 hover:text-gray-900"
                >
                  <Clock className="h-3.5 w-3.5" />
                </a>
                <button
                  type="button"
                  onClick={() => onRemove(item.key)}
                  title="取消关注"
                  className="rounded p-1 text-gray-400 transition hover:bg-gray-50 hover:text-red-600"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
          </li>
        ))}
      </ul>
    )}
  </section>
);
