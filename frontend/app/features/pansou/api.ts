import { ApiError, get, post, put } from '../../../shared/http/client';
import type { PanSouMergedByType, PanSouOptions, PanSouQueryConfig, PanSouSearchData } from './types';

/** SEARCH_TIMEOUT_MS 是单次搜索的等待上限；异步插件会在后台持续补齐结果。 */
const SEARCH_TIMEOUT_MS = 45_000;

/** splitCsv 把逗号分隔的输入拆成去重后的关键词列表。 */
const splitCsv = (value: string, separator = ','): string[] =>
  Array.from(new Set(value.split(separator).map(item => item.trim()).filter(Boolean)));

/** cloudTypesParam 把网盘类型列表转为服务端可接受的字符串数组。 */
const toList = (values: string[]): string[] => (values.length > 0 ? values : []);

/** pansouErrorMessage 把请求异常转换为可直接展示的中文提示。 */
export const pansouErrorMessage = (error: unknown, fallback: string): string => {
  if (error instanceof ApiError) return error.message || fallback;
  if (error instanceof Error) return error.message || fallback;
  return fallback;
};

/** fetchPanSouOptions 读取服务端当前可用的插件与网盘类型。 */
export const fetchPanSouOptions = async (): Promise<PanSouOptions> => get<PanSouOptions>('/api/v1/pansou/options');

/** searchPanSou 执行一次网盘搜索。 */
export interface PanSouSearchInput {
  /** keyword 是搜索关键词。 */
  keyword: string;
  /** config 是本次检索的偏好配置。 */
  config: PanSouQueryConfig;
  /** refresh 为 true 时跳过服务端缓存。 */
  refresh?: boolean;
}

/** searchPanSou 以 POST JSON 提交检索，避免数组参数被序列化为字符串。 */
export const searchPanSou = async ({ keyword, config, refresh = false }: PanSouSearchInput): Promise<PanSouSearchData> => {
  const include = splitCsv(config.includeText);
  const exclude = splitCsv(config.excludeText);
  return post<PanSouSearchData>(
    '/api/v1/pansou/search',
    {
      kw: keyword.trim(),
      plugins: toList(config.selectedPlugins),
      cloud_types: toList(config.selectedCloudTypes),
      res: 'merge',
      src: 'all',
      refresh,
      ...(include.length > 0 || exclude.length > 0 ? { filter: { include, exclude } } : {}),
    },
    { timeoutMs: SEARCH_TIMEOUT_MS },
  );
};

/** normalizeMergedByType 过滤空分组，保证渲染逻辑只处理有内容的网盘类型。 */
export const normalizeMergedByType = (data: PanSouSearchData | null): PanSouMergedByType => {
  const merged = data?.merged_by_type ?? {};
  return Object.fromEntries(Object.entries(merged).filter(([, list]) => list.length > 0));
};

/** PanSouServerSettings 是网盘搜索的服务端可配置项，仅管理员可修改。 */
export interface PanSouServerSettings {
  /** enabled 表示功能是否对用户开放。 */
  enabled: boolean;
  /** enableTg 表示是否启用 Telegram 频道抓取。 */
  enableTg: boolean;
  /** channels 是以逗号分隔的频道列表。 */
  channels: string;
  /** proxy 是访问境外源使用的代理地址。 */
  proxy: string;
  /** cacheTtlMinutes 是结果缓存有效期。 */
  cacheTtlMinutes: string;
}

/** 服务端设置键，与后端 versioned_pansou_routes.go 中的常量保持一致。 */
const SERVER_SETTING_KEYS = {
  enabled: 'pansou_enabled',
  enableTg: 'pansou_enable_tg',
  channels: 'pansou_channels',
  proxy: 'pansou_proxy',
  cacheTtlMinutes: 'pansou_cache_ttl_minutes',
} as const;

/** fetchServerSettings 读取网盘搜索的服务端设置，复用闲鱼助手的系统设置存储。 */
export const fetchServerSettings = async (): Promise<PanSouServerSettings> => {
  const raw = await get<Record<string, string>>('/api/v1/settings/system');
  return {
    enabled: readBool(raw[SERVER_SETTING_KEYS.enabled], true),
    enableTg: readBool(raw[SERVER_SETTING_KEYS.enableTg], false),
    channels: raw[SERVER_SETTING_KEYS.channels] ?? '',
    proxy: raw[SERVER_SETTING_KEYS.proxy] ?? '',
    cacheTtlMinutes: raw[SERVER_SETTING_KEYS.cacheTtlMinutes] ?? '60',
  };
};

/** updateServerSettings 保存网盘搜索的服务端设置。 */
export const updateServerSettings = async (settings: PanSouServerSettings): Promise<void> => {
  await put('/api/v1/settings/system', {
    values: {
      [SERVER_SETTING_KEYS.enabled]: String(settings.enabled),
      [SERVER_SETTING_KEYS.enableTg]: String(settings.enableTg),
      [SERVER_SETTING_KEYS.channels]: settings.channels.trim(),
      [SERVER_SETTING_KEYS.proxy]: settings.proxy.trim(),
      [SERVER_SETTING_KEYS.cacheTtlMinutes]: settings.cacheTtlMinutes.trim() || '60',
    },
  });
};

/** readBool 解析设置中的布尔值，缺失时返回回退值。 */
const readBool = (value: string | undefined, fallback: boolean): boolean => {
  if (value === undefined || value === '') return fallback;
  const normalized = value.trim().toLowerCase();
  if (['true', '1', 'yes', 'on'].includes(normalized)) return true;
  if (['false', '0', 'no', 'off'].includes(normalized)) return false;
  return fallback;
};
