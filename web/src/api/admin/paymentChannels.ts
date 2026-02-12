import { api } from '../client';
import type { APIResponse } from '../types';

export type AdminPaymentChannel = {
  id: number;
  type: string;
  type_label: string;
  name: string;
  status: number;
  usable: boolean;

  stripe_currency?: string;
  stripe_secret_key_set: boolean;
  stripe_webhook_secret_set: boolean;

  epay_gateway?: string;
  epay_partner_id?: string;
  epay_key_set: boolean;

  webhook_url?: string;

  created_at: string;
  updated_at: string;
};

type CreateAdminPaymentChannelRequest = {
  type: string;
  name: string;
  enabled: boolean;

  stripe_currency?: string;
  stripe_secret_key?: string;
  stripe_webhook_secret?: string;

  epay_gateway?: string;
  epay_partner_id?: string;
  epay_key?: string;
};

type UpdateAdminPaymentChannelRequest = Partial<CreateAdminPaymentChannelRequest> & {
  name?: string;
  enabled?: boolean;
};

export async function listAdminPaymentChannels() {
  const res = await api.get<APIResponse<AdminPaymentChannel[]>>('/api/admin/payment-channels');
  return res.data;
}

export async function createAdminPaymentChannel(req: CreateAdminPaymentChannelRequest) {
  const res = await api.post<APIResponse<{ id: number }>>('/api/admin/payment-channels', req);
  return res.data;
}

export async function updateAdminPaymentChannel(paymentChannelID: number, req: UpdateAdminPaymentChannelRequest) {
  const res = await api.put<APIResponse<void>>(`/api/admin/payment-channels/${paymentChannelID}`, req);
  return res.data;
}

export async function deleteAdminPaymentChannel(paymentChannelID: number) {
  const res = await api.delete<APIResponse<void>>(`/api/admin/payment-channels/${paymentChannelID}`);
  return res.data;
}
