/** NetdiskPlatform 是支持的网盘平台。 */
export type NetdiskPlatform = 'quark' | 'baidu';

/** NetdiskAccount 是网盘账号的非敏感视图，绝不包含 cookie。 */
export interface NetdiskAccount {
  id: string;
  platform: NetdiskPlatform;
  remark: string;
  status: 'active' | 'disabled';
  createdAt: string;
  updatedAt: string;
  lastUsedAt?: string;
}

/** NetdiskAccountInput 是新增/更新账号的输入。 */
export interface NetdiskAccountInput {
  platform?: NetdiskPlatform;
  cookie?: string;
  remark?: string;
  status?: 'active' | 'disabled';
}

/** NetdiskQRStatus 是扫码登录会话状态。 */
export interface NetdiskQRStatus {
  sessionId: string;
  platform: NetdiskPlatform;
  state: 'waiting' | 'scanned' | 'success' | 'failed' | 'expired';
  qrDataUrl?: string;
  cookie?: string;
  error?: string;
}

/** NetdiskShareResult 是转存并生成分享链接的结果。 */
export interface NetdiskShareResult {
  shareUrl: string;
  passcode: string;
  title: string;
  fileCount: number;
  platform: NetdiskPlatform;
}

/** netdiskPlatformLabel 返回平台的中文名称。 */
export const netdiskPlatformLabel = (platform: string): string =>
  platform === 'quark' ? '夸克网盘' : platform === 'baidu' ? '百度网盘' : platform;
