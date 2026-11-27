import type { SearchFollowItem, SearchFollowSnapshot, SearchHistoryItem, SearchQueryConfig } from './types';

/** 本地存储键统一加前缀，避免与闲鱼助手其它模块的键冲突。 */
const HISTORY_KEY = 'xile_search_history_v1';
const FOLLOW_KEY = 'xile_search_follow_v1';
const CONFIG_KEY = 'xile_search_config_v1';

/** 历史条数上限，超出后淘汰最旧记录。 */
const HISTORY_LIMIT = 50;

/** 默认网盘类型顺序，用于首次进入时预选。 */
const DEFAULT_CLOUD_TYPES = [
  'baidu',
  'aliyun',
  'quark',
  'tianyi',
  'uc',
  'mobile',
  '115',
  'pikpak',
  'xunlei',
  '123',
  'magnet',
  'ed2k',
  'others',
];

/** 默认检索配置。 */
export const DEFAULT_QUERY_CONFIG: SearchQueryConfig = {
  selectedPlugins: [],
  selectedCloudTypes: [...DEFAULT_CLOUD_TYPES],
  includeText: '',
  excludeText: '',
};

/** canUseStorage 判断当前环境是否可用 localStorage。 */
const canUseStorage = (): boolean => typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';

/** readJson 安全读取并解析 JSON，失败返回 null。 */
const readJson = <T>(key: string): T | null => {
  if (!canUseStorage()) return null;
  try {
    const raw = window.localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
};

/** writeJson 安全写入 JSON，配额不足时静默失败。 */
const writeJson = (key: string, value: unknown): void => {
  if (!canUseStorage()) return;
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // 本地存储不可写时仅影响偏好持久化，不阻断搜索主流程。
  }
};

/** readHistory 读取本地搜索历史。 */
export const readHistory = (): SearchHistoryItem[] => {
  const parsed = readJson<SearchHistoryItem[]>(HISTORY_KEY);
  if (!Array.isArray(parsed)) return [];
  return parsed.filter(item => typeof item?.keyword === 'string' && item.keyword !== '');
};

/** appendHistory 把关键词插到最前并去重，返回最新列表。 */
export const appendHistory = (keyword: string): SearchHistoryItem[] => {
  const target = keyword.trim();
  if (!target) return readHistory();
  const rest = readHistory().filter(item => item.keyword !== target);
  const next = [{ keyword: target, ts: Date.now() }, ...rest].slice(0, HISTORY_LIMIT);
  writeJson(HISTORY_KEY, next);
  return next;
};

/** removeHistory 删除单条历史。 */
export const removeHistory = (keyword: string): SearchHistoryItem[] => {
  const next = readHistory().filter(item => item.keyword !== keyword);
  writeJson(HISTORY_KEY, next);
  return next;
};

/** clearHistory 清空历史。 */
export const clearHistory = (): void => writeJson(HISTORY_KEY, []);

/** readFollowList 读取关注列表。 */
export const readFollowList = (): SearchFollowItem[] => {
  const parsed = readJson<SearchFollowItem[]>(FOLLOW_KEY);
  if (!Array.isArray(parsed)) return [];
  return parsed.filter(item => typeof item?.key === 'string');
};

/** followKey 生成关键词与网盘类型的组合键。 */
const followKey = (keyword: string, sourceType: string): string => `${keyword.trim()}::${sourceType}`;

/** addFollowItem 新增或覆盖一条关注记录。 */
export const addFollowItem = (input: { keyword: string; sourceType: string; item: SearchFollowSnapshot }): SearchFollowItem[] => {
  const key = followKey(input.keyword, input.sourceType);
  const now = Date.now();
  const existing = readFollowList().find(item => item.key === key);
  const record: SearchFollowItem = {
    key,
    keyword: input.keyword.trim(),
    sourceType: input.sourceType,
    createdAt: existing?.createdAt ?? now,
    updatedAt: now,
    lastSnapshot: input.item,
    linkUnchanged: existing ? existing.lastSnapshot.url === input.item.url : false,
  };
  const next = [record, ...readFollowList().filter(item => item.key !== key)];
  writeJson(FOLLOW_KEY, next);
  return next;
};

/** removeFollowItem 删除一条关注。 */
export const removeFollowItem = (key: string): SearchFollowItem[] => {
  const next = readFollowList().filter(item => item.key !== key);
  writeJson(FOLLOW_KEY, next);
  return next;
};

/** clearFollowList 清空关注。 */
export const clearFollowList = (): void => writeJson(FOLLOW_KEY, []);

/** refreshFollowSnapshot 用最新命中的资源刷新关注项，记录链接是否变化。 */
export const refreshFollowSnapshot = (target: SearchFollowItem, snapshot: SearchFollowSnapshot): SearchFollowItem[] => {
  const next = readFollowList().map(item =>
    item.key === target.key
      ? { ...item, updatedAt: Date.now(), lastSnapshot: snapshot, linkUnchanged: item.lastSnapshot.url === snapshot.url }
      : item,
  );
  writeJson(FOLLOW_KEY, next);
  return next;
};

/** readQueryConfig 读取检索偏好并与默认值合并。 */
export const readQueryConfig = (): SearchQueryConfig => {
  const parsed = readJson<Partial<SearchQueryConfig>>(CONFIG_KEY);
  if (!parsed) return { ...DEFAULT_QUERY_CONFIG };
  const unique = (values: unknown): string[] =>
    Array.isArray(values) ? Array.from(new Set(values.filter((v): v is string => typeof v === 'string'))) : [];
  const cloudTypes = unique(parsed.selectedCloudTypes);
  return {
    selectedPlugins: unique(parsed.selectedPlugins),
    selectedCloudTypes: cloudTypes.length > 0 ? cloudTypes : [...DEFAULT_CLOUD_TYPES],
    includeText: typeof parsed.includeText === 'string' ? parsed.includeText : '',
    excludeText: typeof parsed.excludeText === 'string' ? parsed.excludeText : '',
  };
};

/** writeQueryConfig 持久化检索偏好。 */
export const writeQueryConfig = (config: SearchQueryConfig): void => writeJson(CONFIG_KEY, config);

/** sanitizeQueryConfig 丢弃服务端已不存在的插件与网盘类型，避免无效筛选。 */
export const sanitizeQueryConfig = (
  config: SearchQueryConfig,
  options: { plugins: string[]; cloudTypes: string[] },
): SearchQueryConfig => {
  const pluginSet = new Set(options.plugins);
  const cloudSet = new Set(options.cloudTypes);
  const keptPlugins = config.selectedPlugins.filter(item => pluginSet.has(item));
  const keptCloud = config.selectedCloudTypes.filter(item => cloudSet.has(item));
  return {
    ...config,
    selectedPlugins: keptPlugins.length > 0 ? keptPlugins : [...options.plugins],
    selectedCloudTypes: keptCloud.length > 0 ? keptCloud : [...options.cloudTypes],
  };
};
