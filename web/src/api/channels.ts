import { api } from './client';
import type { APIResponse } from './types';

export type UpstreamChannelSetting = {
  force_format?: boolean;
  thinking_to_content?: boolean;
  proxy?: string;
  pass_through_body_enabled?: boolean;
  system_prompt?: string;
  system_prompt_override?: boolean;
};

export type Channel = {
  id: number;
  type: string;
  name: string;
  groups: string;
  status: number;
  priority: number;
  promotion: boolean;
  base_url?: string;

  allow_service_tier: boolean;
  disable_store: boolean;
  allow_safety_identifier: boolean;

  openai_organization?: string | null;
  test_model?: string | null;
  remark?: string | null;
  auto_ban?: boolean;
  setting?: UpstreamChannelSetting;
  param_override?: string;
  header_override?: string;
  status_code_mapping?: string;
  model_suffix_preserve?: string;
  request_body_blacklist?: string;
  request_body_whitelist?: string;

  tag?: string | null;
  weight: number;
  key_hint?: string | null;

  last_test_at?: string | null;
  last_test_latency_ms?: number | null;
  last_test_ok?: boolean | null;
};

export type ChannelUsage = {
  committed_usd: string;
  tokens: number;
  cache_ratio: string;
  avg_first_token_latency: string;
  tokens_per_second: string;
};

type ChannelUsageOverview = {
  requests: number;
  tokens: number;
  committed_usd: string;
  cache_ratio: string;
  avg_first_token_latency: string;
  tokens_per_second: string;
};

export type ChannelRuntime = {
  available: boolean;
  fail_score: number;
  banned_until?: string;
  banned_remaining?: string;
  ban_streak: number;
  banned_active: boolean;
};

export type ChannelAdminItem = Channel & {
  usage: ChannelUsage;
  runtime: ChannelRuntime;
};

type ChannelsPageResponse = {
  admin_time_zone: string;
  start: string;
  end: string;
  overview: ChannelUsageOverview;
  channels: ChannelAdminItem[];
};

export type ChannelTimeSeriesPoint = {
  bucket: string;
  committed_usd: number;
  tokens: number;
  cache_ratio: number;
  avg_first_token_latency: number;
  tokens_per_second: number;
};

type ChannelTimeSeriesResponse = {
  admin_time_zone: string;
  channel_id: number;
  start: string;
  end: string;
  granularity: 'hour' | 'day';
  points: ChannelTimeSeriesPoint[];
};

type CreateChannelRequest = {
  type: string;
  name: string;
  groups?: string;
  base_url: string;
  key?: string;
  priority?: number;
  promotion?: boolean;
  allow_service_tier?: boolean;
  disable_store?: boolean;
  allow_safety_identifier?: boolean;
};

type UpdateChannelRequest = {
  id: number;
  name?: string;
  groups?: string;
  base_url?: string;
  key?: string;
  status?: number;
  priority?: number;
  promotion?: boolean;
  allow_service_tier?: boolean;
  disable_store?: boolean;
  allow_safety_identifier?: boolean;
};

export async function getChannelsPage(params?: { start?: string; end?: string }) {
  const res = await api.get<APIResponse<ChannelsPageResponse>>('/api/channel/page', { params });
  return res.data;
}

export async function getChannelTimeSeries(channelID: number, params?: { start?: string; end?: string; granularity?: 'hour' | 'day' }) {
  const res = await api.get<APIResponse<ChannelTimeSeriesResponse>>(`/api/channel/${channelID}/timeseries`, { params });
  return res.data;
}

export async function createChannel(req: CreateChannelRequest) {
  const res = await api.post<APIResponse<{ id: number }>>('/api/channel', req);
  return res.data;
}

export async function updateChannel(req: UpdateChannelRequest) {
  const res = await api.put<APIResponse<void>>('/api/channel', req);
  return res.data;
}

export async function getChannel(channelID: number) {
  const res = await api.get<APIResponse<Channel>>(`/api/channel/${channelID}`);
  return res.data;
}

export type ChannelCredential = {
  id: number;
  name?: string | null;
  api_key_hint?: string | null;
  masked_key: string;
  status: number;
};

export type CodexOAuthAccount = {
  id: number;
  account_id: string;
  email?: string | null;
  status: number;
  expires_at?: string | null;
  last_refresh_at?: string | null;
  cooldown_until?: string | null;
  last_used_at?: string | null;
  balance_total_granted_usd?: string | null;
  balance_total_used_usd?: string | null;
  balance_total_available_usd?: string | null;
  balance_updated_at?: string | null;
  balance_error?: string | null;
  quota_credits_has_credits?: boolean | null;
  quota_credits_unlimited?: boolean | null;
  quota_credits_balance?: string | null;
  quota_primary_used_percent?: number | null;
  quota_primary_reset_at?: string | null;
  quota_secondary_used_percent?: number | null;
  quota_secondary_reset_at?: string | null;
  quota_updated_at?: string | null;
  quota_error?: string | null;
  created_at: string;
  updated_at: string;
};

export async function listChannelCredentials(channelID: number) {
  const res = await api.get<APIResponse<ChannelCredential[]>>(`/api/channel/${channelID}/credentials`);
  return res.data;
}

export async function createChannelCredential(channelID: number, apiKey: string, name?: string) {
  const res = await api.post<APIResponse<{ id: number; api_key_hint?: string | null }>>(`/api/channel/${channelID}/credentials`, {
    api_key: apiKey,
    name: name || undefined,
  });
  return res.data;
}

export async function deleteChannelCredential(channelID: number, credentialID: number) {
  const res = await api.delete<APIResponse<void>>(`/api/channel/${channelID}/credentials/${credentialID}`);
  return res.data;
}

export async function listChannelCodexAccounts(channelID: number) {
  const res = await api.get<APIResponse<CodexOAuthAccount[]>>(`/api/channel/${channelID}/codex-accounts`);
  return res.data;
}

export async function startChannelCodexOAuth(channelID: number) {
  const res = await api.post<APIResponse<{ auth_url: string }>>(`/api/channel/${channelID}/codex-oauth/start`);
  return res.data;
}

export async function completeChannelCodexOAuth(channelID: number, callbackURL: string) {
  const res = await api.post<APIResponse<void>>(`/api/channel/${channelID}/codex-oauth/complete`, {
    callback_url: callbackURL,
  });
  return res.data;
}

export async function createChannelCodexAccount(
  channelID: number,
  req: {
    account_id?: string;
    email?: string;
    access_token: string;
    refresh_token: string;
    id_token?: string;
    expires_at?: string;
  },
) {
  const res = await api.post<APIResponse<{ id: number }>>(`/api/channel/${channelID}/codex-accounts`, req);
  return res.data;
}

export async function refreshChannelCodexAccount(channelID: number, accountID: number) {
  const res = await api.post<APIResponse<void>>(`/api/channel/${channelID}/codex-accounts/${accountID}/refresh`);
  return res.data;
}

export async function deleteChannelCodexAccount(channelID: number, accountID: number) {
  const res = await api.delete<APIResponse<void>>(`/api/channel/${channelID}/codex-accounts/${accountID}`);
  return res.data;
}

export async function deleteChannel(channelID: number) {
  const res = await api.delete<APIResponse<void>>(`/api/channel/${channelID}`);
  return res.data;
}

export type ChannelModelProbeResult = {
  model: string;
  ok: boolean;
  message: string;
  success_path?: string;
  used_fallback?: boolean;
  ttft_ms?: number;
  sample?: string;
};

export type ChannelProbeSummary = {
  ok: boolean;
  message: string;
  source?: string;
  total: number;
  success: number;
  responses_ok: number;
  chat_ok: number;
  fallback_count: number;
  avg_ttft_ms?: number;
  sample?: string;
  latency_ms?: number;
  results: ChannelModelProbeResult[];
};

export type ChannelTestProgressEvent =
  | {
      type: 'start';
      source?: string;
      total?: number;
      models?: string[];
    }
  | {
      type: 'model_start';
      source?: string;
      index?: number;
      total?: number;
      model?: string;
    }
  | {
      type: 'model_done';
      source?: string;
      index?: number;
      total?: number;
      model?: string;
      result?: ChannelModelProbeResult;
    };

type SSEFrame = {
  event: string;
  data: string;
};

function parseSSEFrames(chunk: string): { frames: SSEFrame[]; rest: string } {
  const normalized = chunk.replace(/\r\n/g, '\n');
  const parts = normalized.split('\n\n');
  const rest = parts.pop() ?? '';
  const frames: SSEFrame[] = [];
  for (const part of parts) {
    let event = 'message';
    const dataLines: string[] = [];
    for (const line of part.split('\n')) {
      if (line.startsWith('event:')) {
        event = line.slice('event:'.length).trim() || 'message';
        continue;
      }
      if (line.startsWith('data:')) {
        dataLines.push(line.slice('data:'.length).trimStart());
      }
    }
    if (dataLines.length > 0) {
      frames.push({ event, data: dataLines.join('\n') });
    }
  }
  return { frames, rest };
}

function resolveTestStreamURL(channelID: number): string {
  return api.getUri({ url: `/api/channel/test/${channelID}`, params: { stream: 1 } });
}

function readRealmsUserHeader(): string {
	try {
		const raw = localStorage.getItem('user');
		if (!raw) return '';
    const parsed = JSON.parse(raw) as { id?: number };
    const id = parsed?.id;
    if (typeof id === 'number' && id > 0) {
      return String(id);
    }
  } catch {
    // ignore
  }
	return '';
}

export async function testChannelStream(
	channelID: number,
	onProgress?: (evt: ChannelTestProgressEvent) => void,
): Promise<APIResponse<{ latency_ms: number; probe?: ChannelProbeSummary }>> {
  const headers: Record<string, string> = {
    Accept: 'text/event-stream',
    'Cache-Control': 'no-store',
  };
  const realmsUser = readRealmsUserHeader();
  if (realmsUser) headers['Realms-User'] = realmsUser;

  const resp = await fetch(resolveTestStreamURL(channelID), {
    method: 'GET',
    credentials: 'include',
    headers,
  });
  if (!resp.ok) {
    const text = (await resp.text()).trim();
    throw new Error(text || `测试失败（HTTP ${resp.status}）`);
  }

  const contentType = resp.headers.get('content-type') || '';
  if (contentType.includes('application/json')) {
    return (await resp.json()) as APIResponse<{ latency_ms: number; probe?: ChannelProbeSummary }>;
  }
  if (!contentType.includes('text/event-stream') || !resp.body) {
    throw new Error(`测试接口返回非流式内容（${contentType || 'unknown'}）`);
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let finalResult: APIResponse<{ latency_ms: number; probe?: ChannelProbeSummary }> | null = null;

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parsed = parseSSEFrames(buffer);
    buffer = parsed.rest;
    for (const frame of parsed.frames) {
      if (frame.event === 'summary') {
        finalResult = JSON.parse(frame.data) as APIResponse<{ latency_ms: number; probe?: ChannelProbeSummary }>;
        continue;
      }
      if (frame.event === 'start' || frame.event === 'model_start' || frame.event === 'model_done') {
        onProgress?.(JSON.parse(frame.data) as ChannelTestProgressEvent);
      }
    }
  }
  buffer += decoder.decode();
  const tail = parseSSEFrames(buffer).frames;
  for (const frame of tail) {
    if (frame.event === 'summary') {
      finalResult = JSON.parse(frame.data) as APIResponse<{ latency_ms: number; probe?: ChannelProbeSummary }>;
      continue;
    }
    if (frame.event === 'start' || frame.event === 'model_start' || frame.event === 'model_done') {
      onProgress?.(JSON.parse(frame.data) as ChannelTestProgressEvent);
    }
  }

  if (!finalResult) {
    throw new Error('测试流未返回总结结果');
  }
  return finalResult;
}

export async function getChannelKey(channelID: number) {
  const res = await api.post<APIResponse<{ key: string }>>(`/api/channel/${channelID}/key`);
  return res.data;
}

export async function reorderChannels(ids: number[]) {
  const res = await api.post<APIResponse<void>>('/api/channel/reorder', ids);
  return res.data;
}

export async function updateChannelMeta(
  channelID: number,
  req: {
    openai_organization?: string | null;
    test_model?: string | null;
    tag?: string | null;
    remark?: string | null;
    weight?: number;
    auto_ban?: boolean;
  },
) {
  const res = await api.put<APIResponse<void>>(`/api/channel/${channelID}/meta`, req);
  return res.data;
}

export async function updateChannelSetting(
  channelID: number,
  req: {
    force_format?: boolean;
    thinking_to_content?: boolean;
    proxy?: string;
    pass_through_body_enabled?: boolean;
    system_prompt?: string;
    system_prompt_override?: boolean;
  },
) {
  const res = await api.put<APIResponse<void>>(`/api/channel/${channelID}/setting`, req);
  return res.data;
}

export async function updateChannelParamOverride(channelID: number, paramOverride: string) {
  const res = await api.put<APIResponse<void>>(`/api/channel/${channelID}/param_override`, { param_override: paramOverride });
  return res.data;
}

export async function updateChannelHeaderOverride(channelID: number, headerOverride: string) {
  const res = await api.put<APIResponse<void>>(`/api/channel/${channelID}/header_override`, { header_override: headerOverride });
  return res.data;
}

export async function updateChannelModelSuffixPreserve(channelID: number, modelSuffixPreserve: string) {
  const res = await api.put<APIResponse<void>>(`/api/channel/${channelID}/model_suffix_preserve`, { model_suffix_preserve: modelSuffixPreserve });
  return res.data;
}

export async function updateChannelRequestBodyWhitelist(channelID: number, requestBodyWhitelist: string) {
  const res = await api.put<APIResponse<void>>(`/api/channel/${channelID}/request_body_whitelist`, { request_body_whitelist: requestBodyWhitelist });
  return res.data;
}

export async function updateChannelRequestBodyBlacklist(channelID: number, requestBodyBlacklist: string) {
  const res = await api.put<APIResponse<void>>(`/api/channel/${channelID}/request_body_blacklist`, { request_body_blacklist: requestBodyBlacklist });
  return res.data;
}

export async function updateChannelStatusCodeMapping(channelID: number, statusCodeMapping: string) {
  const res = await api.put<APIResponse<void>>(`/api/channel/${channelID}/status_code_mapping`, { status_code_mapping: statusCodeMapping });
  return res.data;
}
