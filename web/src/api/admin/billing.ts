import { api } from '../client';
import type { APIResponse } from '../types';

export type AdminSubscriptionPlan = {
  id: number;
  code: string;
  name: string;
  group_name: string;
  price_multiplier: string;
  price_cny: string;
  duration_days: number;
  status: number;

  limit_5h?: string;
  limit_1d?: string;
  limit_7d?: string;
  limit_30d?: string;

  created_at: string;
  updated_at: string;
};

export type CreateAdminSubscriptionPlanRequest = {
  code?: string;
  name: string;
  group_name?: string;
  price_multiplier?: string;
  price_cny: string;
  duration_days: number;
  status?: number;
  limit_5h?: string;
  limit_1d?: string;
  limit_7d?: string;
  limit_30d?: string;
};

export type UpdateAdminSubscriptionPlanRequest = CreateAdminSubscriptionPlanRequest;

export async function listAdminSubscriptionPlans() {
  const res = await api.get<APIResponse<AdminSubscriptionPlan[]>>('/api/admin/subscriptions');
  return res.data;
}

export async function getAdminSubscriptionPlan(planID: number) {
  const res = await api.get<APIResponse<AdminSubscriptionPlan>>(`/api/admin/subscriptions/${planID}`);
  return res.data;
}

export async function createAdminSubscriptionPlan(req: CreateAdminSubscriptionPlanRequest) {
  const res = await api.post<APIResponse<{ id: number }>>('/api/admin/subscriptions', req);
  return res.data;
}

export async function updateAdminSubscriptionPlan(planID: number, req: UpdateAdminSubscriptionPlanRequest) {
  const res = await api.put<APIResponse<void>>(`/api/admin/subscriptions/${planID}`, req);
  return res.data;
}

export async function deleteAdminSubscriptionPlan(planID: number) {
  const res = await api.delete<APIResponse<void>>(`/api/admin/subscriptions/${planID}`);
  return res.data;
}

export type AdminSubscriptionOrder = {
  id: number;
  user_email: string;
  plan_name: string;
  group_name?: string;
  amount_cny: string;
  status: number;
  status_text: string;
  created_at: string;
  paid_at?: string;
  approved_at?: string;
};

export async function listAdminSubscriptionOrders() {
  const res = await api.get<APIResponse<AdminSubscriptionOrder[]>>('/api/admin/orders');
  return res.data;
}

export async function approveAdminSubscriptionOrder(orderID: number) {
  const res = await api.post<APIResponse<void>>(`/api/admin/orders/${orderID}/approve`);
  return res.data;
}

export async function rejectAdminSubscriptionOrder(orderID: number) {
  const res = await api.post<APIResponse<void>>(`/api/admin/orders/${orderID}/reject`);
  return res.data;
}
