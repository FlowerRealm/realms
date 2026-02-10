import { api } from '../client';
import type { APIResponse } from '../types';

export type FeatureBanItem = {
  key: string;
  label: string;
  hint: string;
  disabled: boolean;
  override: boolean;
  editable: boolean;
  forced_by_self_mode: boolean;
  forced_by_build: boolean;
};

export type FeatureBanGroup = {
  title: string;
  items: FeatureBanItem[];
};

export type AdminSettings = {
  self_mode: boolean;
  features: Record<string, unknown>;
  feature_ban_groups: FeatureBanGroup[];
  startup_config_keys: string[];

  site_base_url: string;
  site_base_url_override: boolean;
  site_base_url_effective: string;
  site_base_url_invalid: boolean;

  admin_time_zone: string;
  admin_time_zone_override: boolean;
  admin_time_zone_effective: string;
  admin_time_zone_invalid: boolean;

  email_verification_enabled: boolean;
  email_verification_override: boolean;

  smtp_server: string;
  smtp_server_override: boolean;
  smtp_port: number;
  smtp_port_override: boolean;
  smtp_ssl_enabled: boolean;
  smtp_ssl_enabled_override: boolean;
  smtp_account: string;
  smtp_account_override: boolean;
  smtp_from: string;
  smtp_from_override: boolean;
  smtp_token_set: boolean;
  smtp_token_override: boolean;

  billing_enable_pay_as_you_go: boolean;
  billing_enable_pay_as_you_go_override: boolean;
  billing_min_topup_cny: string;
  billing_min_topup_cny_override: boolean;
  billing_credit_usd_per_cny: string;
  billing_credit_usd_per_cny_override: boolean;
  billing_paygo_price_multiplier: string;
  billing_paygo_price_multiplier_override: boolean;
};

export type UpdateAdminSettingsRequest = {
  site_base_url: string;
  admin_time_zone: string;

  email_verification_enable: boolean;

  smtp_server: string;
  smtp_port: number;
  smtp_ssl_enabled: boolean;
  smtp_account: string;
  smtp_from: string;
  smtp_token: string;

  billing_enable_pay_as_you_go: boolean;
  billing_min_topup_cny: string;
  billing_credit_usd_per_cny: string;
  billing_paygo_price_multiplier: string;

  feature_enabled: Record<string, boolean>;
};

export async function getAdminSettings() {
  const res = await api.get<APIResponse<AdminSettings>>('/api/admin/settings');
  return res.data;
}

export async function updateAdminSettings(req: UpdateAdminSettingsRequest) {
  const res = await api.put<APIResponse<void>>('/api/admin/settings', req);
  return res.data;
}

export async function resetAdminSettings() {
  const res = await api.post<APIResponse<void>>('/api/admin/settings/reset');
  return res.data;
}
