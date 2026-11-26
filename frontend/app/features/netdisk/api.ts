import { get, post, put, del } from '../../../shared/http/client';
import type {
  NetdiskAccount, NetdiskAccountInput, NetdiskQRStatus, NetdiskShareResult, NetdiskPlatform,
} from './types';

/** TRANSFER_TIMEOUT_MS 是转存的等待上限，覆盖异步任务轮询。 */
const TRANSFER_TIMEOUT_MS = 60_000;

/** fetchNetdiskAccounts 读取当前用户的网盘账号列表。 */
export const fetchNetdiskAccounts = async (platform?: NetdiskPlatform): Promise<NetdiskAccount[]> => {
  const data = await get<{ accounts: NetdiskAccount[] }>('/api/v1/netdisk/accounts', platform ? { platform } : undefined);
  return data.accounts ?? [];
};

/** createNetdiskAccount 新增网盘账号。 */
export const createNetdiskAccount = async (input: NetdiskAccountInput): Promise<NetdiskAccount> =>
  post<NetdiskAccount>('/api/v1/netdisk/accounts', input);

/** updateNetdiskAccount 更新账号 cookie 或备注/状态。 */
export const updateNetdiskAccount = async (id: string, input: NetdiskAccountInput): Promise<void> => {
  await put(`/api/v1/netdisk/accounts/${encodeURIComponent(id)}`, input);
};

/** deleteNetdiskAccount 删除网盘账号。 */
export const deleteNetdiskAccount = async (id: string): Promise<void> => {
  await del(`/api/v1/netdisk/accounts/${encodeURIComponent(id)}`);
};

/** checkNetdiskAccount 校验账号登录态是否有效。 */
export const checkNetdiskAccount = async (id: string): Promise<boolean> => {
  const data = await post<{ valid: boolean }>(`/api/v1/netdisk/accounts/${encodeURIComponent(id)}/check`, {});
  return Boolean(data.valid);
};

/** beginNetdiskQR 发起扫码登录，返回初始会话（含二维码图片）。 */
export const beginNetdiskQR = async (platform: NetdiskPlatform): Promise<NetdiskQRStatus> =>
  post<NetdiskQRStatus>('/api/v1/netdisk/qr/begin', { platform });

/** fetchNetdiskQRStatus 查询扫码登录进度。 */
export const fetchNetdiskQRStatus = async (sessionId: string): Promise<NetdiskQRStatus> =>
  get<NetdiskQRStatus>(`/api/v1/netdisk/qr/status/${encodeURIComponent(sessionId)}`);

/** cancelNetdiskQR 取消扫码登录会话。 */
export const cancelNetdiskQR = async (sessionId: string): Promise<void> => {
  await post(`/api/v1/netdisk/qr/cancel/${encodeURIComponent(sessionId)}`, {});
};

/** netdiskTransfer 一键转存并生成自己的分享链接。 */
export interface NetdiskTransferInput {
  /** accountId 是目标网盘账号 ID。 */
  accountId: string;
  /** shareUrl 是他人的分享链接。 */
  shareUrl: string;
  /** passcode 是源分享提取码。 */
  passcode?: string;
  /** newPasscode 是新分享的提取码，留空由平台随机生成。 */
  newPasscode?: string;
  /** periodDays 是新分享有效期（0 永久 / 1 / 7 / 30）。 */
  periodDays?: number;
}

/** netdiskTransfer 提交转存任务并等待结果。 */
export const netdiskTransfer = async (input: NetdiskTransferInput): Promise<NetdiskShareResult> =>
  post<NetdiskShareResult>('/api/v1/netdisk/transfer', input, { timeoutMs: TRANSFER_TIMEOUT_MS });
