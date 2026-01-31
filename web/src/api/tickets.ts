import { api } from './client';
import type { APIResponse } from './types';

export type TicketListItem = {
  id: number;
  subject: string;
  status_text: string;
  status_badge: string;
  last_message_at: string;
  created_at: string;
};

export type TicketAttachment = {
  id: number;
  name: string;
  size: string;
  expires_at: string;
  url: string;
};

export type TicketMessage = {
  id: number;
  actor: string;
  actor_meta: string;
  body: string;
  created_at: string;
  attachments?: TicketAttachment[];
};

export type TicketDetail = {
  id: number;
  subject: string;
  status_text: string;
  status_badge: string;
  last_message_at: string;
  created_at: string;
  closed_at: string;
  can_reply: boolean;
};

export type TicketDetailResponse = {
  ticket: TicketDetail;
  messages: TicketMessage[];
};

export async function listTickets(status: 'all' | 'open' | 'closed') {
  const params: Record<string, string> = {};
  if (status === 'open' || status === 'closed') {
    params.status = status;
  }
  const res = await api.get<APIResponse<TicketListItem[]>>('/api/tickets', { params });
  return res.data;
}

export async function createTicket(subject: string, body: string, attachments: File[] = []) {
  const form = new FormData();
  form.set('subject', subject);
  form.set('body', body);
  for (const f of attachments) {
    form.append('attachments', f);
  }
  const res = await api.post<APIResponse<{ ticket_id: number }>>('/api/tickets', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
  return res.data;
}

export async function getTicketDetail(ticketId: number) {
  const res = await api.get<APIResponse<TicketDetailResponse>>(`/api/tickets/${ticketId}`);
  return res.data;
}

export async function replyTicket(ticketId: number, body: string, attachments: File[] = []) {
  const form = new FormData();
  form.set('body', body);
  for (const f of attachments) {
    form.append('attachments', f);
  }
  const res = await api.post<APIResponse<void>>(`/api/tickets/${ticketId}/reply`, form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
  return res.data;
}

