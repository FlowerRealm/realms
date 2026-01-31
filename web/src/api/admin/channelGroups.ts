import { api } from '../client';
import type { APIResponse } from '../types';

export type AdminChannelGroup = {
  id: number;
  name: string;
  description?: string | null;
  price_multiplier: string;
  max_attempts: number;
  status: number;
  created_at: string;
  updated_at: string;
};

export type AdminChannelGroupMember = {
  member_id: number;
  parent_group_id: number;

  member_group_id?: number | null;
  member_group_name?: string | null;
  member_group_status?: number | null;
  member_group_max_attempts?: number | null;

  member_channel_id?: number | null;
  member_channel_name?: string | null;
  member_channel_type?: string | null;
  member_channel_groups?: string | null;
  member_channel_status?: number | null;

  priority: number;
  promotion: boolean;
};

export type AdminChannelRef = {
  id: number;
  name: string;
  type: string;
};

export type AdminChannelGroupDetail = {
  group: AdminChannelGroup;
  breadcrumb: AdminChannelGroup[];
  members: AdminChannelGroupMember[];
  channels: AdminChannelRef[];
};

export async function listAdminChannelGroups() {
  const res = await api.get<APIResponse<AdminChannelGroup[]>>('/api/admin/channel-groups');
  return res.data;
}

export type CreateAdminChannelGroupRequest = {
  name: string;
  description?: string | null;
  price_multiplier?: string;
  max_attempts?: number;
  status?: number;
};

export async function createAdminChannelGroup(req: CreateAdminChannelGroupRequest) {
  const res = await api.post<APIResponse<{ id: number }>>('/api/admin/channel-groups', req);
  return res.data;
}

export async function getAdminChannelGroup(groupID: number) {
  const res = await api.get<APIResponse<AdminChannelGroup>>(`/api/admin/channel-groups/${groupID}`);
  return res.data;
}

export async function getAdminChannelGroupDetail(groupID: number) {
  const res = await api.get<APIResponse<AdminChannelGroupDetail>>(`/api/admin/channel-groups/${groupID}/detail`);
  return res.data;
}

export type UpdateAdminChannelGroupRequest = {
  description?: string | null;
  price_multiplier?: string;
  max_attempts?: number;
  status?: number;
};

export async function updateAdminChannelGroup(groupID: number, req: UpdateAdminChannelGroupRequest) {
  const res = await api.put<APIResponse<void>>(`/api/admin/channel-groups/${groupID}`, req);
  return res.data;
}

export async function deleteAdminChannelGroup(groupID: number) {
  const res = await api.delete<APIResponse<void>>(`/api/admin/channel-groups/${groupID}`);
  return res.data;
}

export async function createAdminChildChannelGroup(parentGroupID: number, req: CreateAdminChannelGroupRequest) {
  const res = await api.post<APIResponse<{ id: number }>>(`/api/admin/channel-groups/${parentGroupID}/children/groups`, req);
  return res.data;
}

export async function addAdminChannelGroupChannelMember(parentGroupID: number, channelID: number) {
  const res = await api.post<APIResponse<void>>(`/api/admin/channel-groups/${parentGroupID}/children/channels`, { channel_id: channelID });
  return res.data;
}

export async function deleteAdminChannelGroupGroupMember(parentGroupID: number, childGroupID: number) {
  const res = await api.delete<APIResponse<void>>(`/api/admin/channel-groups/${parentGroupID}/children/groups/${childGroupID}`);
  return res.data;
}

export async function deleteAdminChannelGroupChannelMember(parentGroupID: number, channelID: number) {
  const res = await api.delete<APIResponse<void>>(`/api/admin/channel-groups/${parentGroupID}/children/channels/${channelID}`);
  return res.data;
}

export async function reorderAdminChannelGroupMembers(parentGroupID: number, orderedMemberIDs: number[]) {
  const res = await api.post<APIResponse<void>>(`/api/admin/channel-groups/${parentGroupID}/children/reorder`, orderedMemberIDs);
  return res.data;
}

