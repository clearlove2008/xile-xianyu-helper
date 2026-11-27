import React, { useMemo, useState } from 'react';
import { Check, Search } from 'lucide-react';
import type { SearchOptions, SearchQueryConfig } from '../types';
import { DEFAULT_QUERY_CONFIG } from '../localStore';

interface ConfigPanelProps {
  /** config 是当前检索偏好。 */ config: SearchQueryConfig;
  /** options 是服务端可用的插件与网盘类型。 */ options: SearchOptions | null;
  /** onChange 更新偏好。 */ onChange: (next: SearchQueryConfig) => void;
  /** onReset 恢复默认偏好。 */ onReset: () => void;
}

// ConfigPanel 提供检索范围与过滤条件的配置，偏好保存在浏览器本地。
export const ConfigPanel: React.FC<ConfigPanelProps> = ({ config, options, onChange, onReset }) => {
  const [pluginFilter, setPluginFilter] = useState('');

  // plugins 是服务端已启用插件，按名称过滤后用于渲染。
  const plugins = options?.plugins ?? [];
  const visiblePlugins = useMemo(
    () => (pluginFilter.trim() ? plugins.filter(name => name.toLowerCase().includes(pluginFilter.trim().toLowerCase())) : plugins),
    [plugins, pluginFilter],
  );
  const cloudTypes = options?.cloud_types ?? [];

  /** togglePlugin 切换单个插件的启用状态。 */
  const togglePlugin = (name: string): void => {
    const selected = config.selectedPlugins.includes(name)
      ? config.selectedPlugins.filter(item => item !== name)
      : [...config.selectedPlugins, name];
    onChange({ ...config, selectedPlugins: selected });
  };

  /** toggleCloudType 切换单个网盘类型的展示状态。 */
  const toggleCloudType = (key: string): void => {
    const selected = config.selectedCloudTypes.includes(key)
      ? config.selectedCloudTypes.filter(item => item !== key)
      : [...config.selectedCloudTypes, key];
    // 至少保留一个类型，否则服务端会返回全部类型而让筛选失效。
    onChange({ ...config, selectedCloudTypes: selected.length > 0 ? selected : [key] });
  };

  return (
    <div className="space-y-4">
      <section className="rounded-lg border border-gray-200 bg-white p-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-sm font-bold text-gray-900">网盘类型</h2>
          <button
            type="button"
            onClick={() => onChange({ ...config, selectedCloudTypes: cloudTypes.map(item => item.key) })}
            className="text-xs font-medium text-gray-500 hover:text-gray-900"
          >
            全选
          </button>
        </div>
        <div className="mt-2 flex flex-wrap gap-1.5">
          {cloudTypes.map(item => {
            const selected = config.selectedCloudTypes.includes(item.key);
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => toggleCloudType(item.key)}
                aria-pressed={selected}
                className={`inline-flex items-center gap-1 rounded-md px-2.5 py-1.5 text-xs font-bold transition ${
                  selected ? 'bg-gray-900 text-white' : 'bg-gray-50 text-gray-600 hover:bg-gray-100'
                }`}
              >
                {selected && <Check className="h-3 w-3" />}
                {item.label}
              </button>
            );
          })}
        </div>
      </section>

      <section className="rounded-lg border border-gray-200 bg-white p-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-sm font-bold text-gray-900">
            搜索源插件
            <span className="ml-1.5 text-xs font-medium text-gray-400">
              已选 {config.selectedPlugins.length} / {plugins.length}
            </span>
          </h2>
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => onChange({ ...config, selectedPlugins: [...plugins] })}
              className="text-xs font-medium text-gray-500 hover:text-gray-900"
            >
              全选
            </button>
            <button
              type="button"
              onClick={() => onChange({ ...config, selectedPlugins: [] })}
              className="text-xs font-medium text-gray-500 hover:text-gray-900"
            >
              清空
            </button>
          </div>
        </div>
        <div className="relative mt-2">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-gray-400" />
          <input
            value={pluginFilter}
            onChange={event => setPluginFilter(event.target.value)}
            placeholder="过滤插件名称"
            aria-label="过滤插件"
            className="w-full rounded-md border border-gray-300 py-1.5 pl-8 pr-2 text-xs text-gray-900 placeholder:text-gray-400 focus:border-gray-900 focus:outline-none"
          />
        </div>
        <div className="mt-2 flex max-h-56 flex-wrap gap-1.5 overflow-y-auto">
          {visiblePlugins.length === 0 ? (
            <p className="text-xs text-gray-400">没有匹配的插件。</p>
          ) : (
            visiblePlugins.map(name => {
              const selected = config.selectedPlugins.includes(name);
              return (
                <button
                  key={name}
                  type="button"
                  onClick={() => togglePlugin(name)}
                  aria-pressed={selected}
                  className={`rounded px-2 py-1 text-[11px] font-bold transition ${
                    selected ? 'bg-gray-900 text-white' : 'bg-gray-50 text-gray-600 hover:bg-gray-100'
                  }`}
                >
                  {name}
                </button>
              );
            })
          )}
        </div>
        {plugins.length === 0 && (
          <p className="mt-2 rounded-md bg-amber-50 px-3 py-2 text-xs font-medium text-amber-700">服务端尚未启用任何插件，请先在系统设置中配置。</p>
        )}
      </section>

      <section className="rounded-lg border border-gray-200 bg-white p-4">
        <h2 className="text-sm font-bold text-gray-900">结果过滤</h2>
        <p className="mt-1 text-xs text-gray-500">包含关键词之间是「任一命中」，排除关键词命中即剔除。</p>
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          <label className="block">
            <span className="text-xs font-bold text-gray-700">包含关键词</span>
            <input
              value={config.includeText}
              onChange={event => onChange({ ...config, includeText: event.target.value })}
              placeholder="合集, 全集"
              className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-xs text-gray-900 placeholder:text-gray-400 focus:border-gray-900 focus:outline-none"
            />
          </label>
          <label className="block">
            <span className="text-xs font-bold text-gray-700">排除关键词</span>
            <input
              value={config.excludeText}
              onChange={event => onChange({ ...config, excludeText: event.target.value })}
              placeholder="预告, 花絮"
              className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-xs text-gray-900 placeholder:text-gray-400 focus:border-gray-900 focus:outline-none"
            />
          </label>
        </div>
      </section>

      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => {
            onReset();
            onChange({ ...DEFAULT_QUERY_CONFIG, selectedPlugins: [...plugins] });
          }}
          className="rounded-md border border-gray-300 bg-white px-4 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-gray-50"
        >
          恢复默认
        </button>
      </div>
    </div>
  );
};
