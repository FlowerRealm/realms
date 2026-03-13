import { api } from "./client";
import type { APIResponse } from "./types";

export type ChannelModelRuntime = {
  available: boolean;
  fail_score: number;
  banned_until?: string;
  banned_remaining?: string;
  ban_streak: number;
  banned_active: boolean;
};

export type ChannelModelBinding = {
  id: number;
  channel_id: number;
  public_id: string;
  upstream_model: string;
  status: number;
  runtime: ChannelModelRuntime;
};

export async function listChannelModels(channelID: number) {
  const res = await api.get<APIResponse<ChannelModelBinding[]>>(
    `/api/channel/${channelID}/models`,
  );
  return res.data;
}

export async function createChannelModel(
  channelID: number,
  publicID: string,
  upstreamModel?: string,
  status = 1,
) {
  const res = await api.post<APIResponse<{ id: number }>>(
    `/api/channel/${channelID}/models`,
    {
      public_id: publicID,
      upstream_model: upstreamModel || undefined,
      status,
    },
  );
  return res.data;
}

export async function updateChannelModel(
  channelID: number,
  binding: {
    id: number;
    public_id: string;
    upstream_model: string;
    status: number;
  },
) {
  const res = await api.put<APIResponse<void>>(
    `/api/channel/${channelID}/models`,
    binding,
  );
  return res.data;
}
