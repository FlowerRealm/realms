import { api } from '../client';
import { asRecord, pickNumber, pickString } from '../redemptionSupport';
import type { APIResponse } from '../types';

export type AdminRedemptionRewardType = 'subscription' | 'balance';
export type AdminRedemptionDistributionMode = 'single' | 'shared';

export type AdminRedemptionCode = {
  id: number;
  batch_name: string;
  code: string;
  distribution_mode: AdminRedemptionDistributionMode;
  reward_type: AdminRedemptionRewardType;
  plan_id?: number;
  plan_name?: string;
  balance_usd?: string;
  max_redemptions: number;
  redeemed_count: number;
  expires_at?: string;
  status: number;
  created_at: string;
  updated_at: string;
};

export type CreateAdminRedemptionCodesRequest = {
  batch_name: string;
  codes?: string[];
  count?: number;
  distribution_mode: AdminRedemptionDistributionMode;
  reward_type: AdminRedemptionRewardType;
  subscription_plan_id?: number;
  balance_usd?: string;
  max_redemptions?: number;
  expires_at?: string;
  status?: number;
};

export type UpdateAdminRedemptionCodeRequest = {
  max_redemptions: number;
  expires_at?: string;
  status: number;
};

export type CreateAdminRedemptionCodesResponse = {
  ids: number[];
  codes: string[];
};

function normalizeAdminRedemptionCode(item: unknown): AdminRedemptionCode {
  const rec = asRecord(item);
  return {
    id: pickNumber(rec.id) || 0,
    batch_name: pickString(rec.batch_name) || '',
    code: pickString(rec.code) || '',
    distribution_mode: (pickString(rec.distribution_mode) === 'shared' ? 'shared' : 'single') as AdminRedemptionDistributionMode,
    reward_type: (pickString(rec.reward_type) === 'balance' ? 'balance' : 'subscription') as AdminRedemptionRewardType,
    plan_id: pickNumber(rec.plan_id),
    plan_name: pickString(rec.plan_name),
    balance_usd: pickString(rec.balance_usd),
    max_redemptions: pickNumber(rec.max_redemptions) || 1,
    redeemed_count: pickNumber(rec.redeemed_count) || 0,
    expires_at: pickString(rec.expires_at),
    status: pickNumber(rec.status) || 0,
    created_at: pickString(rec.created_at) || '',
    updated_at: pickString(rec.updated_at) || '',
  };
}

export async function listAdminRedemptionCodes() {
  const res = await api.get<APIResponse<unknown[]>>('/api/admin/redemption-codes');
  return {
    ...res.data,
    data: Array.isArray(res.data.data) ? res.data.data.map(normalizeAdminRedemptionCode) : [],
  } satisfies APIResponse<AdminRedemptionCode[]>;
}

export async function createAdminRedemptionCodes(req: CreateAdminRedemptionCodesRequest) {
  const payload = {
    batch_name: req.batch_name.trim(),
    codes: req.codes?.map((item) => item.trim()).filter(Boolean),
    count: req.count,
    distribution_mode: req.distribution_mode,
    reward_type: req.reward_type,
    subscription_plan_id: req.subscription_plan_id,
    balance_usd: req.balance_usd?.trim() || undefined,
    max_redemptions: req.max_redemptions,
    expires_at: req.expires_at?.trim() || undefined,
    status: req.status,
  };
  const res = await api.post<APIResponse<CreateAdminRedemptionCodesResponse>>('/api/admin/redemption-codes', payload);
  return res.data;
}

export async function updateAdminRedemptionCode(id: number, req: UpdateAdminRedemptionCodeRequest) {
  const res = await api.patch<APIResponse<void>>(`/api/admin/redemption-codes/${encodeURIComponent(String(id))}`, {
    max_redemptions: req.max_redemptions,
    expires_at: req.expires_at?.trim() || '',
    status: req.status,
  });
  return res.data;
}

export async function disableAdminRedemptionCode(id: number) {
  const res = await api.post<APIResponse<void>>(`/api/admin/redemption-codes/${encodeURIComponent(String(id))}/disable`);
  return res.data;
}

function fileNameFromHeaders(contentDisposition?: string | null) {
  const header = (contentDisposition || '').trim();
  if (!header) return 'redemption-codes.csv';
  const utf8 = header.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8?.[1]) return decodeURIComponent(utf8[1]);
  const plain = header.match(/filename="?([^"]+)"?/i);
  if (plain?.[1]) return plain[1];
  return 'redemption-codes.csv';
}

export async function exportAdminRedemptionCodes() {
  const res = await api.get<Blob>('/api/admin/redemption-codes/export', { responseType: 'blob' });
  return {
    blob: res.data,
    fileName: fileNameFromHeaders(res.headers['content-disposition']),
  };
}
