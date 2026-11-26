import React from 'react';
import { Loader2, RotateCw, Search } from 'lucide-react';

interface SearchInputProps {
  /** value 是受控输入值。 */ value: string;
  /** onChange 同步用户输入。 */ onChange: (value: string) => void;
  /** onSubmit 触发检索。 */ onSubmit: () => void;
  /** loading 表示检索进行中，用于禁用重复提交。 */ loading: boolean;
  /** refresh 表示本次是否强制刷新缓存。 */ refresh: boolean;
  /** onToggleRefresh 切换强制刷新。 */ onToggleRefresh: () => void;
}

// SearchInput 渲染关键词输入与主要操作按钮，回车即可检索。
export const SearchInput: React.FC<SearchInputProps> = ({ value, onChange, onSubmit, loading, refresh, onToggleRefresh }) => (
  <form
    className="flex flex-col gap-3 sm:flex-row sm:items-center"
    onSubmit={event => {
      event.preventDefault();
      onSubmit();
    }}
  >
    <div className="relative flex-1">
      <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
      <input
        type="search"
        value={value}
        onChange={event => onChange(event.target.value)}
        placeholder="输入片名、剧名或软件名，例如：沙丘"
        aria-label="搜索关键词"
        className="w-full rounded-lg border border-gray-300 bg-white py-2.5 pl-9 pr-3 text-sm text-gray-900 placeholder:text-gray-400 focus:border-gray-900 focus:outline-none focus:ring-2 focus:ring-gray-900/10"
      />
    </div>
    <label className="flex select-none items-center gap-2 whitespace-nowrap text-sm font-medium text-gray-600">
      <input type="checkbox" checked={refresh} onChange={onToggleRefresh} className="h-4 w-4 rounded border-gray-300 text-gray-900 focus:ring-gray-900" />
      强制刷新
    </label>
    <button
      type="submit"
      disabled={loading || value.trim() === ''}
      className="inline-flex items-center justify-center gap-2 rounded-lg bg-gray-900 px-5 py-2.5 text-sm font-bold text-white transition hover:bg-gray-800 disabled:cursor-not-allowed disabled:bg-gray-300"
    >
      {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RotateCw className="h-4 w-4" />}
      {loading ? '检索中' : '检索资源'}
    </button>
  </form>
);
