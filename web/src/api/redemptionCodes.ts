import { api } from './client';
import { asRecord, pickString } from './redemptionSupport';
import type { APIResponse } from './types';

export type RedemptionKind = 'subscription' | 'topup';
export type RedemptionApplyMode = 'parallel' | 'sequential';

export type UserRedemptionRequest = {
  kind: RedemptionKind;
  code: string;
  applyMode?: RedemptionApplyMode;
};

export type UserRedemptionData = {
  reward_type?: 'subscription' | 'balance';
  balance_usd?: string;
  new_balance_usd?: string;
  plan_name?: string;
  subscription_start_at?: string;
  subscription_end_at?: string;
  subscription_activation_mode?: 'immediate' | 'deferred';
  error_code?: string;
};

export type UserRedemptionModePrompt = {
  title: string;
  description: string;
  options: RedemptionApplyMode[];
  defaultMode: RedemptionApplyMode;
  reason?: string;
};

function toBackendActivationMode(mode?: RedemptionApplyMode) {
  if (mode === 'parallel') return 'immediate';
  if (mode === 'sequential') return 'deferred';
  return undefined;
}

export async function redeemUserCode(req: UserRedemptionRequest) {
  const payload = {
    kind: req.kind === 'topup' ? 'balance' : 'subscription',
    code: req.code.trim(),
    subscription_activation_mode: toBackendActivationMode(req.applyMode),
  };
  const res = await api.post<APIResponse<UserRedemptionData>>('/api/billing/redeem', payload);
  return res.data;
}

export function getUserRedemptionModePrompt(resp: APIResponse<UserRedemptionData>): UserRedemptionModePrompt | null {
  if (resp.success) return null;
  const data = asRecord(resp.data);
  if (pickString(data.error_code) !== 'subscription_activation_mode_required') {
    return null;
  }
  return {
    title: '选择生效方式',
    description: '当前账号已有同套餐订阅，请选择本次兑换是立即并行生效，还是顺延到当前套餐结束后再生效。',
    options: ['parallel', 'sequential'],
    defaultMode: 'sequential',
    reason: pickString(resp.message),
  };
}
