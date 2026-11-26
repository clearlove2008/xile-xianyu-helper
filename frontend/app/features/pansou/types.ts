/** 网盘搜索的数据契约，字段与服务端 model 保持一致。 */

/** PanSouSourceType 是合并后的网盘类型键，例如 quark、baidu。 */
export type PanSouSourceType = string;

/** PanSouMergedLink 是按网盘类型分组后的单条资源。 */
export interface PanSouMergedLink {
  /** url 是网盘分享地址。 */
  url: string;
  /** password 是提取码，可能为空。 */
  password: string;
  /** note 是资源标题或说明。 */
  note: string;
  /** datetime 是资源发布时间，ISO 字符串。 */
  datetime: string;
  /** source 是数据来源标识，形如 plugin:xxx。 */
  source?: string;
  /** images 是 Telegram 消息附带的图片。 */
  images?: string[];
}

/** PanSouMergedByType 以网盘类型为键聚合资源列表。 */
export type PanSouMergedByType = Record<string, PanSouMergedLink[]>;

/** PanSouSearchData 是一次搜索的完整结果。 */
export interface PanSouSearchData {
  /** total 是结果总数。 */
  total: number;
  /** results 是原始结果，仅在 res=results 或 all 时返回。 */
  results?: Array<{
    /** message_id 是来源消息标识。 */
    message_id: string;
    /** unique_id 是全局唯一标识。 */
    unique_id: string;
    /** channel 是来源频道名称。 */
    channel: string;
    /** datetime 是发布时间。 */
    datetime: string;
    /** title 是标题。 */
    title: string;
    /** content 是正文。 */
    content: string;
    /** links 是该消息包含的网盘链接。 */
    links: Array<{ type: string; url: string; password: string }>;
  }>;
  /** merged_by_type 是按网盘类型分组的结果。 */
  merged_by_type?: PanSouMergedByType;
}

/** PanSouOptions 描述服务端当前可用的搜索能力，用于渲染配置面板。 */
export interface PanSouOptions {
  /** enabled 表示功能是否开放。 */
  enabled: boolean;
  /** enable_tg 表示是否启用 Telegram 频道来源。 */
  enable_tg: boolean;
  /** channels 是已配置的频道列表。 */
  channels: string[] | null;
  /** plugins 是已启用的插件名称。 */
  plugins: string[];
  /** cloud_types 是网盘类型字典。 */
  cloud_types: Array<{ key: string; label: string }>;
  /** proxy_configured 表示服务端是否已配置代理。 */
  proxy_configured: boolean;
}

/** PanSouQueryConfig 是用户在配置面板中维护的检索偏好，持久化在浏览器本地。 */
export interface PanSouQueryConfig {
  /** selectedPlugins 是本次检索启用的插件。 */
  selectedPlugins: string[];
  /** selectedCloudTypes 是本次检索要展示的网盘类型。 */
  selectedCloudTypes: string[];
  /** includeText 是包含关键词，逗号分隔。 */
  includeText: string;
  /** excludeText 是排除关键词，逗号分隔。 */
  excludeText: string;
}

/** PanSouHistoryItem 是一条本地搜索历史。 */
export interface PanSouHistoryItem {
  /** keyword 是搜索词。 */
  keyword: string;
  /** ts 是记录时间戳。 */
  ts: number;
}

/** PanSouFollowSnapshot 是关注项最近一次命中的资源快照。 */
export interface PanSouFollowSnapshot {
  /** url 是网盘地址。 */
  url: string;
  /** password 是提取码。 */
  password: string;
  /** datetime 是资源时间。 */
  datetime: string;
  /** source 是来源标识。 */
  source?: string;
  /** note 是资源标题。 */
  note: string;
}

/** PanSouFollowItem 是一条关注（追更）记录。 */
export interface PanSouFollowItem {
  /** key 是关键词与网盘类型的组合键。 */
  key: string;
  /** keyword 是关注的关键词。 */
  keyword: string;
  /** sourceType 是关注的网盘类型。 */
  sourceType: PanSouSourceType;
  /** createdAt 是创建时间。 */
  createdAt: number;
  /** updatedAt 是最近更新时间。 */
  updatedAt: number;
  /** lastSnapshot 是最近一次命中的资源。 */
  lastSnapshot: PanSouFollowSnapshot;
  /** linkUnchanged 表示本次刷新与上次链接一致。 */
  linkUnchanged: boolean;
}
