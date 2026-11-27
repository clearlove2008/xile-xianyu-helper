import { ApiError, get, post, postForm } from '../../../shared/http/client';
import type {
  StudioAccount, StudioGenerateForm, StudioGenerateResult, StudioModelConfig, StudioPublishForm, StudioSettings,
} from './types';

/** SETTINGS_KEY 是本地设置的存储键；密钥只留在本机浏览器，不上送服务端也不入库。 */
const SETTINGS_KEY = 'xile.studio.settings.v1';

/** GENERATE_TIMEOUT_MS 是单次出图的等待上限，上游排队时可能超过两分钟。 */
const GENERATE_TIMEOUT_MS = 5 * 60_000;

/** PUBLISH_TIMEOUT_MS 是发布商品的等待上限，平台上传图片较慢。 */
const PUBLISH_TIMEOUT_MS = 10 * 60_000;

/** emptyModelConfig 返回一组空配置。 */
export const emptyModelConfig = (): StudioModelConfig => ({ baseUrl: '', apiKey: '', model: '', fallback: '' });

/** defaultSettings 是首次进入页面时的默认设置。 */
export const defaultSettings = (): StudioSettings => ({
  image: { baseUrl: '', apiKey: '', model: 'gpt-image-2', fallback: 'gpt-image-2.5-sunburst' },
  text: emptyModelConfig(),
  systemPrompt: '',
});

/** loadSettings 读取本机保存的设置，缺失时返回默认值。 */
export const loadSettings = (): StudioSettings => {
  if (typeof window === 'undefined') return defaultSettings();
  try {
    const raw = window.localStorage.getItem(SETTINGS_KEY);
    if (!raw) return defaultSettings();
    const parsed = JSON.parse(raw) as Partial<StudioSettings>;
    const base = defaultSettings();
    return {
      image: { ...base.image, ...(parsed.image ?? {}) },
      text: { ...base.text, ...(parsed.text ?? {}) },
      systemPrompt: typeof parsed.systemPrompt === 'string' ? parsed.systemPrompt : '',
    };
  } catch {
    return defaultSettings();
  }
};

/** saveSettings 把设置写入本机浏览器。 */
export const saveSettings = (settings: StudioSettings): void => {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
};

/** fetchSystemPrompt 读取服务端内置的默认系统提示词，避免前后端各存一份文案。 */
export const fetchSystemPrompt = async (): Promise<string> => {
  const data = await get<{ systemPrompt: string }>('/api/v1/studio/defaults');
  return data.systemPrompt ?? '';
};

/** testConnection 用给定配置拉取模型清单，验证地址与密钥是否可用。 */
export const testConnection = async (config: StudioModelConfig): Promise<string[]> => {
  const data = await post<{ models: string[]; count: number }>(
    '/api/v1/studio/models',
    { baseUrl: config.baseUrl.trim(), apiKey: config.apiKey.trim() },
    { timeoutMs: 30_000 },
  );
  return data.models ?? [];
};

/** previewPrompt 预览组装后的提示词，不调用模型。 */
export const previewPrompt = async (form: StudioGenerateForm, systemPrompt: string): Promise<string> => {
  const data = await post<{ prompt: string }>('/api/v1/studio/prompt', {
    systemPrompt: systemPrompt.trim(),
    headline: form.headline,
    price: form.price,
    points: form.points,
    extra: form.extra,
    copy: form.copy,
    shipping: form.shipping,
    hasImage: form.image !== '',
  });
  return data.prompt ?? '';
};

/** generateImage 调用中转站出图，返回 data URL 结果。 */
export const generateImage = async (settings: StudioSettings, form: StudioGenerateForm): Promise<StudioGenerateResult> => {
  const image = settings.image;
  return post<StudioGenerateResult>(
    '/api/v1/studio/generate',
    {
      baseUrl: image.baseUrl.trim(),
      apiKey: image.apiKey.trim(),
      model: image.model.trim(),
      fallback: image.fallback.trim(),
      image: form.image,
      systemPrompt: settings.systemPrompt.trim(),
      headline: form.headline,
      price: form.price,
      points: form.points,
      extra: form.extra,
      copy: form.copy,
      shipping: form.shipping,
    },
    { timeoutMs: GENERATE_TIMEOUT_MS },
  );
};

/** polishCopy 用文字模型把文案改写成闲鱼口吻；未配置文字模型时调用方应先拦截。 */
export const polishCopy = async (settings: StudioSettings, copy: string, shipping: string): Promise<string> => {
  const text = settings.text;
  const data = await post<{ copy: string; model: string }>(
    '/api/v1/studio/polish',
    {
      baseUrl: text.baseUrl.trim(),
      apiKey: text.apiKey.trim(),
      model: text.model.trim(),
      copy,
      shipping,
    },
    { timeoutMs: 120_000 },
  );
  return data.copy ?? '';
};

/** listAccounts 读取可用于发布的账号摘要；/accounts/details 只返回已登录账号。 */
export const listAccounts = async (): Promise<StudioAccount[]> => {
  const data = await get<Array<{ id: string; remark?: string; nickname?: string; username?: string; enabled?: boolean }>>('/api/v1/accounts/details');
  return (data ?? []).map(item => ({
    id: item.id,
    remark: item.remark,
    nickname: item.nickname,
    username: item.username,
    enabled: item.enabled,
  }));
};

/** dataUrlToFile 把 data URL 转成发布接口需要的 File。 */
export const dataUrlToFile = (dataUrl: string, filename: string): File => {
  const comma = dataUrl.indexOf(',');
  const header = comma >= 0 ? dataUrl.slice(0, comma) : 'data:image/png;base64';
  const body = comma >= 0 ? dataUrl.slice(comma + 1) : dataUrl;
  const mime = /:(.*?);/.exec(header)?.[1] ?? 'image/png';
  const binary = atob(body);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
  return new File([bytes], filename, { type: mime });
};

/** imageToFile 区分网络图片与 Base64，避免把商品原图网址交给 atob。 */
export const imageToFile = async (image: string, filename: string): Promise<File> => {
  // source 去除配置或接口返回的首尾空白。
  const source = image.trim();
  if (!source) throw new Error('没有可发布的图片，请重新选择商品或生成图片');
  if (/^(https?:)?\/\//i.test(source)) {
    // response 只下载图片，不向第三方图片站发送本机登录凭证。
    let response: Response;
    try {
      response = await fetch(source.startsWith('//') ? `https:${source}` : source, { credentials: 'omit' });
    } catch {
      throw new Error('源图片下载失败或不允许跨域访问，请改用 AI 生图后重试');
    }
    if (!response.ok) throw new Error(`源图片下载失败（HTTP ${response.status}），请重新选择图片`);
    // blob 必须为非空图片，拒绝将错误页面作为商品主图提交。
    const blob = await response.blob();
    if (!blob.size || !blob.type.startsWith('image/')) throw new Error('源地址未返回有效图片，请改用 AI 生图');
    return new File([blob], filename, { type: blob.type });
  }
  try {
    return dataUrlToFile(source, filename);
  } catch {
    throw new Error('图片数据不是有效的 Base64，请重新生成或选择图片');
  }
};

/** publishItem 把一张或多张图片发布到指定账号，描述第一行由服务端固化为标题。 */
export const publishItem = async (
  form: StudioPublishForm,
  image: string | string[],
  cookieId: string,
): Promise<{ item_id?: string; quantity?: number; stock_warning?: string }> => {
  const body = new FormData();
  body.set('cookie_id', cookieId);
  body.set('title', form.title.trim());
  body.set('description', form.description);
  body.set('price', form.price.trim());
  // 库存按用户输入提交：平台对「新建多件库存」有账号/类目限制，
  // 命中时后端返回 multi_inventory_not_allowed，由页面提示改为 1 件。
  body.set('quantity', (form.quantity || '').trim() || '1');
  body.set('postage_mode', 'free');
  const images = (Array.isArray(image) ? image : [image]).slice(0, 9);
  if (!images.length || images.some(source => !source.trim())) throw new Error('没有可发布的图片，请重新选择商品或生成图片');
  for (let index = 0; index < images.length; index += 1) {
    body.append('images', await imageToFile(images[index], images.length === 1 ? 'studio-cover.png' : `studio-cover-${index + 1}.png`));
  }
  return postForm<{ item_id?: string; quantity?: number; stock_warning?: string }>('/api/v1/items/publish', body, { timeoutMs: PUBLISH_TIMEOUT_MS });
};

/** studioErrorMessage 把请求异常转成可直接展示的中文提示。 */
export const studioErrorMessage = (error: unknown, fallback: string): string => {
  if (error instanceof ApiError) return error.message || fallback;
  if (error instanceof Error) return error.message || fallback;
  return fallback;
};
