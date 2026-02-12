import { api } from './client';
import type { APIResponse } from './types';

type ForceLogoutResponse = {
  force_logout?: boolean;
};

export async function updateEmail(email: string, verificationCode: string) {
  const res = await api.post<APIResponse<ForceLogoutResponse>>('/api/account/email', {
    email,
    verification_code: verificationCode,
  });
  return res.data;
}

export async function updatePassword(oldPassword: string, newPassword: string) {
  const res = await api.post<APIResponse<ForceLogoutResponse>>('/api/account/password', {
    old_password: oldPassword,
    new_password: newPassword,
  });
  return res.data;
}
