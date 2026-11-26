/** WorkflowTask 是一条商品工作流任务。 */
export interface WorkflowTask {
  id: number;
  keyword: string;
  searchCookieId: string;
  publishCookieIds: string[];
  priceMinCents: number;
  priceMaxCents: number;
  targetCount: number;
  intervalMinutes: number;
  publishGapSeconds: number;
  useSourceImage: boolean;
  priceCents: number;
  quantity: number;
  enabled: boolean;
  lastRunAt?: string;
  nextRunAt?: string;
}

/** WorkflowTaskInput 是创建/更新任务的请求体。 */
export interface WorkflowTaskInput {
  keyword: string;
  searchCookieId: string;
  publishCookieIds: string[];
  priceMinCents: number;
  priceMaxCents: number;
  targetCount: number;
  intervalMinutes: number;
  publishGapSeconds: number;
  useSourceImage: boolean;
  priceCents: number;
  quantity: number;
  enabled: boolean;
}

/** WorkflowLog 是一条运行日志。 */
export interface WorkflowLog {
  id: number;
  taskId: number;
  level: string;
  message: string;
  createdAt: string;
}

/** WorkflowModelConfig 是一组模型接入参数（服务端存储）。 */
export interface WorkflowModelConfig {
  baseUrl: string;
  apiKey: string;
  model: string;
  fallback: string;
}

import { get, post, put, del } from '../../../shared/http/client';

/** listTasks 读取任务列表。 */
export const listTasks = async (): Promise<WorkflowTask[]> => {
  const data = await get<{ tasks: WorkflowTask[] }>('/api/v1/workflow/tasks');
  return data.tasks ?? [];
};

/** createTask 新建任务。 */
export const createTask = async (input: WorkflowTaskInput): Promise<number> => {
  const data = await post<{ id: number }>('/api/v1/workflow/tasks', input);
  return data.id ?? 0;
};

/** updateTask 更新任务。 */
export const updateTask = async (id: number, input: WorkflowTaskInput): Promise<void> => {
  await put(`/api/v1/workflow/tasks/${id}`, input);
};

/** deleteTask 删除任务。 */
export const deleteTask = async (id: number): Promise<void> => {
  await del(`/api/v1/workflow/tasks/${id}`);
};

/** toggleTask 启停任务。 */
export const toggleTask = async (id: number, enabled: boolean): Promise<void> => {
  await post(`/api/v1/workflow/tasks/${id}/toggle`, { enabled });
};

/** runTask 手动触发一次执行。 */
export const runTask = async (id: number): Promise<void> => {
  await post(`/api/v1/workflow/tasks/${id}/run`, {});
};

/** listLogs 读取任务运行日志。 */
export const listLogs = async (id: number, limit = 50): Promise<WorkflowLog[]> => {
  const data = await get<{ logs: WorkflowLog[] }>(`/api/v1/workflow/tasks/${id}/logs`, { limit: String(limit) });
  return data.logs ?? [];
};

/** getModels 读取模型配置（密钥打码）。 */
export const getModels = async (): Promise<{ text: WorkflowModelConfig; image: WorkflowModelConfig }> => {
  return get<{ text: WorkflowModelConfig; image: WorkflowModelConfig }>('/api/v1/workflow/models');
};

/** saveModels 保存模型配置；密钥为掩码时服务端保留原值。 */
export const saveModels = async (text: WorkflowModelConfig, image: WorkflowModelConfig): Promise<void> => {
  await put('/api/v1/workflow/models', { text, image });
};
