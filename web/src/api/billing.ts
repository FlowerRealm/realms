import { api } from './client';
import type { APIResponse } from './types';

export type BillingSubscriptionOrderView = {
  id: number;
  plan_name: string;
  amount_cny: string;
  status: string;
  created_at: string;
  paid_at?: string;
  approved_at?: string;
};

type BillingSubscriptionWindow = {
  window: string;
  used_usd: string;
  limit_usd: string;
  used_percent: number;
};

export type BillingSubscriptionView = {
  active: boolean;
  plan_name: string;
  price_cny: string;
  group_name: string;
  start_at: string;
  end_at: string;
  usage_windows?: BillingSubscriptionWindow[];
};

export type BillingPlanView = {
  id: number;
  name: string;
  price_cny: string;
  group_name: string;
  limit_5h: string;
  limit_1d: string;
  limit_7d: string;
  limit_30d: string;
  duration_days: number;
};

export type BillingSubscriptionPageResponse = {
  subscription?: BillingSubscriptionView;
  subscriptions: BillingSubscriptionView[];
  plans: BillingPlanView[];
  subscription_orders: BillingSubscriptionOrderView[];
};

export type BillingTopupOrderView = {
  id: number;
  amount_cny: string;
  credit_usd: string;
  status: string;
  created_at: string;
  paid_at?: string;
};

export type BillingTopupPageResponse = {
  balance_usd: string;
  pay_as_you_go_enabled: boolean;
  topup_min_cny: string;
  topup_orders: BillingTopupOrderView[];
  payment_channels: BillingPaymentChannelView[];
};

export type BillingPayOrderView = {
  kind: string;
  id: number;
  title: string;
  amount_cny: string;
  credit_usd?: string;
  status: string;
  created_at: string;
};

export type BillingPaymentChannelView = {
  id: number;
  type: string;
  type_label: string;
  name: string;
};

export type BillingPayPageResponse = {
  base_url: string;
  pay_order: BillingPayOrderView;
  payment_channels: BillingPaymentChannelView[];
};

export async function getSubscriptionPage() {
  const res = await api.get<APIResponse<BillingSubscriptionPageResponse>>('/api/billing/subscription');
  return res.data;
}

export async function purchaseSubscription(planId: number) {
  const res = await api.post<APIResponse<{ order_id: number }>>('/api/billing/subscription/purchase', { plan_id: planId });
  return res.data;
}

export async function getTopupPage() {
  const res = await api.get<APIResponse<BillingTopupPageResponse>>('/api/billing/topup');
  return res.data;
}

export async function createTopupOrder(amountCNY: string) {
  const res = await api.post<APIResponse<{ order_id: number }>>('/api/billing/topup/create', { amount_cny: amountCNY });
  return res.data;
}

export async function getPayPage(kind: string, orderId: number) {
  const res = await api.get<APIResponse<BillingPayPageResponse>>(`/api/billing/pay/${encodeURIComponent(kind)}/${orderId}`);
  return res.data;
}

export async function cancelPayOrder(kind: string, orderId: number) {
  const res = await api.post<APIResponse<void>>(`/api/billing/pay/${encodeURIComponent(kind)}/${orderId}/cancel`);
  return res.data;
}

type StartPaymentRequest = {
  payment_channel_id: number;
  epay_type?: string;
};

export async function startPayment(kind: string, orderId: number, req: StartPaymentRequest) {
  const res = await api.post<APIResponse<{ redirect_url: string }>>(`/api/billing/pay/${encodeURIComponent(kind)}/${orderId}/start`, req);
  return res.data;
}
