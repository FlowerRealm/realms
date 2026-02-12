import { api } from './client';
import type { APIResponse } from './types';

export type AnnouncementListItem = {
  id: number;
  title: string;
  created_at: string;
  read: boolean;
};

type AnnouncementsListResponse = {
  unread_count: number;
  items: AnnouncementListItem[];
};

export type AnnouncementDetail = {
  id: number;
  title: string;
  body: string;
  created_at: string;
};

export async function listAnnouncements(limit = 200) {
  const res = await api.get<APIResponse<AnnouncementsListResponse>>('/api/announcements', {
    params: { limit },
  });
  return res.data;
}

export async function getAnnouncement(id: number) {
  const res = await api.get<APIResponse<AnnouncementDetail>>(`/api/announcements/${id}`);
  return res.data;
}
