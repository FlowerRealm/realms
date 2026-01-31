import { api } from './client';
import type { APIResponse } from './types';

export type DashboardSubscriptionWindow = {
  window: string;
  used_usd: string;
  limit_usd: string;
  used_percent: number;
};

export type DashboardSubscription = {
  active: boolean;
  plan_name?: string;
  end_at?: string;
  usage_windows?: DashboardSubscriptionWindow[];
};

export type DashboardModelUsage = {
  model: string;
  icon_url?: string;
  color: string;
  requests: number;
  tokens: number;
  committed_usd: string;
};

export type DashboardTimeSeriesUsage = {
  label: string;
  requests: number;
  tokens: number;
  committed_usd: number;
};

export type DashboardCharts = {
  model_stats: DashboardModelUsage[];
  time_series_stats: DashboardTimeSeriesUsage[];
};

export type DashboardData = {
  today_usage_usd: string;
  today_requests: number;
  today_tokens: number;
  today_rpm: string;
  today_tpm: string;
  unread_announcements_count: number;
  subscription?: DashboardSubscription;
  charts: DashboardCharts;
};

export async function getDashboard() {
  const res = await api.get<APIResponse<DashboardData>>('/api/dashboard');
  return res.data;
}
