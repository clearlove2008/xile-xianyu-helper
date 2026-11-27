import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, Inbox, SlidersHorizontal, Sparkles } from 'lucide-react';
import { PageHeader } from '../../../../shared/ui/PageHeader';
import { useSession } from '../../../providers/SessionProvider';
import { fetchSearchOptions, normalizeMergedByType, searchErrorMessage, searchSearch } from '../api';
import { ConfigPanel } from '../components/ConfigPanel';
import { ResultList, sourceLabel } from '../components/ResultList';
import { SearchInput } from '../components/SearchInput';
import ServerSettingsPanel from '../components/ServerSettingsPanel';
import { FollowPanel, HistoryPanel } from '../components/SidePanels';
import {
  addFollowItem,
  appendHistory,
  clearFollowList,
  clearHistory,
  readFollowList,
  readHistory,
  readQueryConfig,
  refreshFollowSnapshot,
  removeFollowItem,
  removeHistory,
  sanitizeQueryConfig,
  writeQueryConfig,
  DEFAULT_QUERY_CONFIG,
} from '../localStore';
import type { SearchFollowItem, SearchHistoryItem, SearchMergedByType, SearchMergedLink, SearchOptions, SearchQueryConfig } from '../types';

/** REFILL_INTERVAL_MS 是补齐请求的间隔；异步插件会在后台继续产出结果。 */
const REFILL_INTERVAL_MS = 2_500;

/** MAX_REFILL_ROUNDS 是最多补齐轮次，避免无限轮询。 */
const MAX_REFILL_ROUNDS = 3;

/** wait 返回一个可被页面卸载打断的延时。 */
const wait = (ms: number): Promise<void> => new Promise(resolve => window.setTimeout(resolve, ms));

// ResourceSearch 是网盘资源搜索页，聚合多源结果并按网盘类型分组展示。
const ResourceSearch: React.FC = () => {
  const { isAdmin } = useSession();
  const [view, setView] = useState<'search' | 'config'>('search');
  const [keyword, setKeyword] = useState('');
  const [searchKeyword, setSearchKeyword] = useState('');
  const [refresh, setRefresh] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [merged, setMerged] = useState<SearchMergedByType>({});
  const [options, setOptions] = useState<SearchOptions | null>(null);
  const [config, setConfig] = useState<SearchQueryConfig>(readQueryConfig);
  const [history, setHistory] = useState<SearchHistoryItem[]>(readHistory);
  const [follows, setFollows] = useState<SearchFollowItem[]>(readFollowList);
  const [refreshingKey, setRefreshingKey] = useState<string | null>(null);

  // aliveRef 标记组件是否仍然挂载，用于中断补齐轮询。
  const aliveRef = useRef(true);
  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
    };
  }, []);

  useEffect(() => {
    writeQueryConfig(config);
  }, [config]);

  // 首次进入读取服务端能力，并按实际插件/类型修正本地偏好。
  useEffect(() => {
    let cancelled = false;
    const load = async (): Promise<void> => {
      try {
        const next = await fetchSearchOptions();
        if (cancelled) return;
        setOptions(next);
        setConfig(previous =>
          sanitizeQueryConfig(previous, {
            plugins: next.plugins ?? [],
            cloudTypes: (next.cloud_types ?? []).map(item => item.key),
          }),
        );
      } catch {
        if (!cancelled) setOptions(null);
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  /** executeSearch 执行检索，并在后台补齐后续轮次结果。 */
  const executeSearch = useCallback(
    async (target: string, forceRefresh: boolean): Promise<void> => {
      const word = target.trim();
      if (!word) return;

      setLoading(true);
      setError('');
      setSearchKeyword(word);

      try {
        const first = await searchSearch({ keyword: word, config, refresh: forceRefresh });
        if (!aliveRef.current) return;
        setMerged(normalizeMergedByType(first));
        setHistory(appendHistory(word));
        setLoading(false);

        // 首轮结果稳定后继续补齐，异步插件在后台产出的结果会在后续轮次出现。
        let latest = first;
        for (let round = 1; round < MAX_REFILL_ROUNDS; round += 1) {
          await wait(REFILL_INTERVAL_MS);
          if (!aliveRef.current) return;
          try {
            const next = await searchSearch({ keyword: word, config, refresh: false });
            if (!aliveRef.current) return;
            if ((next.total ?? 0) > (latest.total ?? 0)) {
              latest = next;
              setMerged(normalizeMergedByType(next));
            }
          } catch {
            // 补齐失败不影响已展示的结果。
            return;
          }
        }
      } catch (searchError) {
        if (!aliveRef.current) return;
        setError(searchErrorMessage(searchError, '检索失败，请稍后重试'));
        setMerged({});
        setLoading(false);
      }
    },
    [config],
  );

  /** handleFollow 把当前资源加入关注列表。 */
  const handleFollow = (item: SearchMergedLink, sourceType: string): void => {
    if (!searchKeyword.trim()) return;
    setFollows(addFollowItem({ keyword: searchKeyword, sourceType, item }));
  };

  /** handleRefreshFollow 重新检索关注项并刷新快照。 */
  const handleRefreshFollow = async (target: SearchFollowItem): Promise<void> => {
    setRefreshingKey(target.key);
    try {
      const result = await searchSearch({ keyword: target.keyword, config, refresh: false });
      const list = normalizeMergedByType(result)[target.sourceType] ?? [];
      const snapshot = list[0];
      if (snapshot) setFollows(refreshFollowSnapshot(target, snapshot));
    } catch {
      // 刷新失败保留原快照，由用户择机重试。
    } finally {
      if (aliveRef.current) setRefreshingKey(null);
    }
  };

  // total 是当前结果的资源总数。
  const total = useMemo(() => Object.values(merged).reduce((sum, list) => sum + list.length, 0), [merged]);
  // sourceCount 是有结果的网盘类型数量。
  const sourceCount = Object.keys(merged).length;
  const hasSearched = searchKeyword !== '';

  return (
    <div className="space-y-5">
      <PageHeader title="网盘资源搜索" description="聚合多个资源站的网盘分享链接，按网盘类型分组，支持一键复制与关注追更。">
        <div className="flex items-center gap-1 rounded-lg border border-gray-200 bg-white p-1">
          <button
            type="button"
            onClick={() => setView('search')}
            className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-bold transition ${
              view === 'search' ? 'bg-gray-900 text-white' : 'text-gray-600 hover:bg-gray-50'
            }`}
          >
            <Sparkles className="h-4 w-4" />
            检索
          </button>
          <button
            type="button"
            onClick={() => setView('config')}
            className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-bold transition ${
              view === 'config' ? 'bg-gray-900 text-white' : 'text-gray-600 hover:bg-gray-50'
            }`}
          >
            <SlidersHorizontal className="h-4 w-4" />
            配置
          </button>
        </div>
      </PageHeader>

      <section className="rounded-lg border border-gray-200 bg-white p-4">
        <SearchInput
          value={keyword}
          onChange={setKeyword}
          onSubmit={() => {
            setView('search');
            void executeSearch(keyword, refresh);
          }}
          loading={loading}
          refresh={refresh}
          onToggleRefresh={() => setRefresh(value => !value)}
        />
        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500">
          <span>
            已选 {config.selectedPlugins.length} 个插件 · {config.selectedCloudTypes.length} 种网盘类型
          </span>
          {options && !options.enable_tg && <span className="rounded bg-amber-50 px-1.5 py-0.5 font-medium text-amber-700">Telegram 频道未启用</span>}
          {options && !options.proxy_configured && <span className="rounded bg-amber-50 px-1.5 py-0.5 font-medium text-amber-700">未配置代理，境外源可能失败</span>}
        </div>
      </section>

      {view === 'config' ? (
        <div className="space-y-4">
          <ConfigPanel
            config={config}
            options={options}
            onChange={setConfig}
            onReset={() => setConfig({ ...DEFAULT_QUERY_CONFIG, selectedPlugins: options?.plugins ?? [] })}
          />
          {isAdmin && <ServerSettingsPanel />}
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-[300px_minmax(0,1fr)]">
          <aside className="space-y-4 lg:sticky lg:top-6 lg:self-start">
            <HistoryPanel
              items={history}
              onSearch={word => {
                setKeyword(word);
                void executeSearch(word, false);
              }}
              onRemove={word => setHistory(removeHistory(word))}
              onClear={() => {
                clearHistory();
                setHistory([]);
              }}
            />
            <FollowPanel
              items={follows}
              refreshingKey={refreshingKey}
              onRefresh={item => void handleRefreshFollow(item)}
              onRemove={key => setFollows(removeFollowItem(key))}
              onClear={() => {
                clearFollowList();
                setFollows([]);
              }}
            />
          </aside>

          <section className="space-y-4" aria-live="polite">
            <div className="rounded-lg border border-gray-200 bg-white px-4 py-3">
              {hasSearched ? (
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="text-sm text-gray-700">
                    关键词：<span className="font-bold text-gray-900">{searchKeyword}</span>
                  </p>
                  <p className="text-xs font-medium text-gray-500">
                    共 {total} 条，覆盖 {sourceCount} 种网盘类型
                  </p>
                </div>
              ) : (
                <p className="text-sm text-gray-500">输入关键词后开始检索；可先在「配置」中调整搜索源与网盘类型。</p>
              )}
            </div>

            {loading && (
              <div className="rounded-lg border border-gray-200 bg-white px-4 py-12 text-center text-sm text-gray-500">正在并发检索各资源站，请稍候…</div>
            )}

            {!loading && error !== '' && (
              <div className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-4 py-3">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-600" />
                <div className="min-w-0">
                  <p className="text-sm font-bold text-red-800">检索失败</p>
                  <p className="mt-0.5 break-words text-xs text-red-700">{error}</p>
                  <button
                    type="button"
                    onClick={() => void executeSearch(searchKeyword, true)}
                    className="mt-2 rounded border border-red-300 bg-white px-2.5 py-1 text-xs font-bold text-red-700 hover:bg-red-100"
                  >
                    重新检索
                  </button>
                </div>
              </div>
            )}

            {!loading && error === '' && hasSearched && total === 0 && (
              <div className="rounded-lg border border-gray-200 bg-white px-4 py-12 text-center">
                <Inbox className="mx-auto h-8 w-8 text-gray-300" />
                <p className="mt-2 text-sm font-bold text-gray-700">没有找到「{searchKeyword}」的资源</p>
                <p className="mt-1 text-xs text-gray-500">换个关键词，或在配置中启用更多搜索源。</p>
              </div>
            )}

            {!loading && error === '' && total > 0 && <ResultList mergedByType={merged} onFollow={handleFollow} />}

            {!loading && error === '' && total > 0 && (
              <p className="text-center text-xs text-gray-400">
                结果来自 {sourceCount} 种网盘类型，当前展示 {sourceLabel(Object.keys(merged)[0] ?? '')} 等分组；点击上方页签切换。
              </p>
            )}
          </section>
        </div>
      )}
    </div>
  );
};

export default ResourceSearch;
