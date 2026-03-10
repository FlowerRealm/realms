import { api } from './client';
import type { APIResponse } from './types';
import { browserTimeZone } from './timezone';

export type UsageWindow = {
  window: string;
  since: string;
  until: string;
  requests: number;
  tokens: number;
  rpm: number;
  tpm: number;
  input_tokens: number;
  output_tokens: number;
  cached_input_tokens: number;
  cached_output_tokens: number;
  cache_ratio: number;
  used_usd: string;
  committed_usd: string;
  reserved_usd: string;
  limit_usd: string;
  remaining_usd: string;
};

type UsageWindowsResponse = {
  time_zone?: string;
  now: string;
  subscription?: {
    active: boolean;
    plan_name?: string;
    start_at?: string;
    end_at?: string;
  };
  windows: UsageWindow[];
};

export type UsageEvent = {
  id: number;
  time: string;
  request_id: string;
  endpoint?: string | null;
  method?: string | null;
  token_id: number;
  upstream_endpoint_id?: number | null;
  upstream_credential_id?: number | null;
  state: string;
  model?: string | null;
  service_tier?: string | null;
  input_tokens?: number | null;
  cached_input_tokens?: number | null;
  output_tokens?: number | null;
  cached_output_tokens?: number | null;
  reserved_usd: string;
  committed_usd: string;
  reserve_expires_at: string;
  status_code: number;
  latency_ms: number;
  error_class?: string | null;
  error_message?: string | null;
  is_stream: boolean;
  request_bytes: number;
  response_bytes: number;
  created_at: string;
  updated_at: string;
};

type UsageEventsResponse = {
  events: UsageEvent[];
  next_before_id?: number | null;
};

export type UsageTimeSeriesPoint = {
  bucket: string;
  requests: number;
  tokens: number;
  committed_usd: number;
  cache_ratio: number;
  avg_first_token_latency: number;
  tokens_per_second: number;
};

type UsageTimeSeriesResponse = {
  time_zone?: string;
  start: string;
  end: string;
  granularity: 'hour' | 'day';
  points: UsageTimeSeriesPoint[];
};

export async function getUsageWindows(start?: string, end?: string, tokenID?: number, allTime?: boolean) {
  const res = await api.get<APIResponse<UsageWindowsResponse>>('/api/usage/windows', {
    params: {
      start: start || undefined,
      end: end || undefined,
      token_id: tokenID || undefined,
      all_time: allTime ? true : undefined,
      tz: browserTimeZone(),
    },
  });
  return res.data;
}

export async function getUsageEvents(limit = 100, beforeID?: number, start?: string, end?: string, tokenID?: number) {
  const res = await api.get<APIResponse<UsageEventsResponse>>('/api/usage/events', {
    params: {
      limit,
      before_id: beforeID || undefined,
      start: start || undefined,
      end: end || undefined,
      token_id: tokenID || undefined,
      tz: browserTimeZone(),
    },
  });
  return res.data;
}

export async function getUsageEventsV2(params: {
  limit?: number;
  before_id?: number;
  start?: string;
  end?: string;
  token_id?: number;
  index?: string;
  q?: string;
  q_key?: string;
  q_model?: string;
}) {
  const res = await api.get<APIResponse<UsageEventsResponse>>('/api/usage/events', {
    params: {
      ...params,
      tz: browserTimeZone(),
    },
  });
  return res.data;
}

export async function getUsageTimeSeries(start?: string, end?: string, granularity?: 'hour' | 'day', tokenID?: number, allTime?: boolean) {
  const res = await api.get<APIResponse<UsageTimeSeriesResponse>>('/api/usage/timeseries', {
    params: {
      start: start || undefined,
      end: end || undefined,
      granularity: granularity || undefined,
      token_id: tokenID || undefined,
      all_time: allTime ? true : undefined,
      tz: browserTimeZone(),
    },
  });
  return res.data;
}

export type UsageEventDetail = {
  event_id: number;
  pricing_breakdown?: UsageEventPricingBreakdown;
};

export type UsageEventPricingBreakdown = {
  cost_source: 'committed' | 'reserved' | 'none' | string;
  cost_source_usd: string;

  model_public_id?: string;
  model_found: boolean;
  service_tier?: string;
  pricing_kind?: string;
  high_context_applied: boolean;
  high_context_threshold_tokens: number;
  high_context_trigger_input_tokens: number;
  effective_service_tier?: string;

  input_tokens_total: number;
  input_tokens_cached: number;
  input_tokens_billable: number;
  output_tokens_total: number;
  output_tokens_cached: number;
  output_tokens_billable: number;

  input_usd_per_1m: string;
  output_usd_per_1m: string;
  cache_input_usd_per_1m: string;
  cache_output_usd_per_1m: string;

  input_cost_usd: string;
  output_cost_usd: string;
  cache_input_cost_usd: string;
  cache_output_cost_usd: string;
  base_cost_usd: string;

  payment_multiplier: string;
  group_name: string;
  group_multiplier: string;
  effective_multiplier: string;

  final_cost_usd: string;
  diff_from_source_usd: string;
};

export async function getUsageEventDetail(eventID: number, tokenID?: number) {
  const res = await api.get<APIResponse<UsageEventDetail>>(`/api/usage/events/${eventID}/detail`, {
    params: {
      token_id: tokenID || undefined,
    },
  });
  return res.data;
}
