/** StudioModelConfig 描述一组模型接入参数，图片与文字模型各持一份。 */
export interface StudioModelConfig {
  /** baseUrl 是 OpenAI 兼容接口根地址。 */
  baseUrl: string;
  /** apiKey 是访问密钥。 */
  apiKey: string;
  /** model 是首选模型标识。 */
  model: string;
  /** fallback 是首选失败时的备用模型，留空表示不回退。 */
  fallback: string;
}

/** StudioSettings 是出图工作台的全部本地设置。 */
export interface StudioSettings {
  /** image 是图片模型配置。 */
  image: StudioModelConfig;
  /** text 是文字模型配置，留空时由内置模板组装指令。 */
  text: StudioModelConfig;
  /** systemPrompt 是发给图片模型的角色约束。 */
  systemPrompt: string;
}

/** StudioGenerateForm 是一次出图的输入。 */
export interface StudioGenerateForm {
  /** image 是原图 data URL，留空表示文生图。 */
  image: string;
  /** headline 是主标题。 */
  headline: string;
  /** price 是价格。 */
  price: string;
  /** points 是卖点清单。 */
  points: string[];
  /** extra 是补充要求。 */
  extra: string;
  /** copy 是完整商品文案。 */
  copy: string;
  /** shipping 是一键发货文字。 */
  shipping: string;
}

/** StudioGenerateResult 是一次出图的结果。 */
export interface StudioGenerateResult {
  /** model 是实际产出图片的模型。 */
  model: string;
  /** seconds 是耗时秒数。 */
  seconds: number;
  /** image 是 data URL 结果图。 */
  image: string;
  /** fallback 表示是否发生了模型回退。 */
  fallback: boolean;
  /** prompt 是本次实际发出的提示词。 */
  prompt: string;
}

/** StudioAccount 是发布商品时可选的账号摘要。 */
export interface StudioAccount {
  /** id 是账号稳定标识，发布时作为 cookie_id 传入。 */
  id: string;
  /** remark 是用户备注。 */
  remark?: string;
  /** nickname 是闲鱼昵称，作为账号展示名优先于备注。 */
  nickname?: string;
  /** username 是登录用户名，昵称与备注都缺失时用于展示。 */
  username?: string;
  /** enabled 表示账号的自动化任务是否开启；关闭的账号仍可手动发布。 */
  enabled?: boolean;
}

/** StudioPublishTarget 是单个账号的发布结果。 */
export interface StudioPublishTarget {
  /** accountId 是对应账号标识。 */
  accountId: string;
  /** label 是账号展示名。 */
  label: string;
  /** ok 表示该账号是否发布成功。 */
  ok: boolean;
  /** detail 是成功时的商品 ID 或失败原因。 */
  detail: string;
}

/** StudioPublishForm 是把生成图发布到闲鱼所需的字段。 */
export interface StudioPublishForm {
  /** cookieIds 是本次勾选的发布账号，可多选，按勾选顺序依次发布。 */
  cookieIds: string[];
  /** title 是商品标题。 */
  title: string;
  /** description 是商品描述，第一行会被平台当作标题展示。 */
  description: string;
  /** price 是售价。 */
  price: string;
  /** quantity 是库存件数文本；平台若拒绝多件新建，会返回明确提示。 */
  quantity: string;
}
