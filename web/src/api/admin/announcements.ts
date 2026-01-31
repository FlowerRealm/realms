import { api } from '../client';
import type { APIResponse } from '../types';

export type AdminAnnouncement = {
  id: number;
  title: string;
  body: string;
  status: number;
  created_at: string;
  updated_at: string;
};

export async function listAdminAnnouncements() {
  const res = await api.get<APIResponse<AdminAnnouncement[]>>('/api/admin/announcements');
  return res.data;
}

export type CreateAdminAnnouncementRequest = {
  title: string;
  body: string;
  status?: number;
};

export async function createAdminAnnouncement(req: CreateAdminAnnouncementRequest) {
  const res = await api.post<APIResponse<{ id: number }>>('/api/admin/announcements', req);
  return res.data;
}

export async function updateAdminAnnouncementStatus(id: number, status: number) {
  const res = await api.put<APIResponse<void>>(`/api/admin/announcements/${id}`, { status });
  return res.data;
}

export async function deleteAdminAnnouncement(id: number) {
  const res = await api.delete<APIResponse<void>>(`/api/admin/announcements/${id}`);
  return res.data;
}

