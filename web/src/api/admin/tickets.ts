import { api } from '../client';
import type { APIResponse } from '../types';

export type AdminTicketListItem = {
  id: number;
  user_email: string;
  subject: string;
  status_text: string;
  status_badge: string;
  last_message_at: string;
  created_at: string;
};

export type AdminTicketAttachment = {
  id: number;
  name: string;
  size: string;
  expires_at: string;
  url: string;
};

export type AdminTicketMessage = {
  id: number;
  actor: string;
  actor_meta: string;
  body: string;
  created_at: string;
  attachments?: AdminTicketAttachment[];
};

export type AdminTicketDetail = {
  id: number;
  user_email: string;
  subject: string;
  status_text: string;
  status_badge: string;
  last_message_at: string;
  created_at: string;
  closed_at: string;
  can_reply: boolean;
  closed: boolean;
};

export type AdminTicketDetailResponse = {
  ticket: AdminTicketDetail;
  messages: AdminTicketMessage[];
};

export async function listAdminTickets(status: 'all' | 'open' | 'closed') {
  const params: Record<string, string> = {};
  if (status === 'open' || status === 'closed') {
    params.status = status;
  }
  const res = await api.get<APIResponse<AdminTicketListItem[]>>('/api/admin/tickets', { params });
  return res.data;
}

export async function getAdminTicketDetail(ticketId: number) {
  const res = await api.get<APIResponse<AdminTicketDetailResponse>>(`/api/admin/tickets/${ticketId}`);
  return res.data;
}

export async function replyAdminTicket(ticketId: number, body: string, attachments: File[] = []) {
  const form = new FormData();
  form.set('body', body);
  for (const f of attachments) {
    form.append('attachments', f);
  }
  const res = await api.post<APIResponse<void>>(`/api/admin/tickets/${ticketId}/reply`, form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
  return res.data;
}

export async function closeAdminTicket(ticketId: number) {
  const res = await api.post<APIResponse<void>>(`/api/admin/tickets/${ticketId}/close`);
  return res.data;
}

export async function reopenAdminTicket(ticketId: number) {
  const res = await api.post<APIResponse<void>>(`/api/admin/tickets/${ticketId}/reopen`);
  return res.data;
}

