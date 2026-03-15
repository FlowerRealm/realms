import { isAxiosError } from 'axios';

import type { APIResponse } from './types';

export type RawRecord = Record<string, unknown>;

export type CandidateJSONResponse<T> = {
  status: number;
  data: APIResponse<T>;
};

export function asRecord(value: unknown): RawRecord {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as RawRecord;
  }
  return {};
}

export function pickString(...values: unknown[]): string | undefined {
  for (const value of values) {
    if (typeof value === 'string') {
      const trimmed = value.trim();
      if (trimmed) return trimmed;
    }
  }
  return undefined;
}

export function pickNumber(...values: unknown[]): number | undefined {
  for (const value of values) {
    if (typeof value === 'number' && Number.isFinite(value)) return value;
    if (typeof value === 'string') {
      const trimmed = value.trim();
      if (!trimmed) continue;
      const parsed = Number(trimmed);
      if (Number.isFinite(parsed)) return parsed;
    }
  }
  return undefined;
}

export function pickBoolean(...values: unknown[]): boolean | undefined {
  for (const value of values) {
    if (typeof value === 'boolean') return value;
    if (typeof value === 'number') return value !== 0;
    if (typeof value === 'string') {
      const normalized = value.trim().toLowerCase();
      if (!normalized) continue;
      if (['1', 'true', 'yes', 'enabled', 'active'].includes(normalized)) return true;
      if (['0', 'false', 'no', 'disabled', 'inactive'].includes(normalized)) return false;
    }
  }
  return undefined;
}

export function pickStringList(...values: unknown[]): string[] | undefined {
  for (const value of values) {
    if (Array.isArray(value)) {
      const out = value
        .map((item) => (typeof item === 'string' ? item.trim() : ''))
        .filter(Boolean);
      if (out.length > 0) return out;
    }
  }
  return undefined;
}

function isMissingEndpointMessage(message?: string): boolean {
  const normalized = (message || '').trim().toLowerCase();
  if (!normalized) return false;
  return normalized === 'not found' || normalized.includes('404 not found') || normalized.includes('no route');
}

function isMissingEndpointResponse<T>(response: CandidateJSONResponse<T>): boolean {
  if (response.status === 404 || response.status === 405) return true;
  return !response.data.success && isMissingEndpointMessage(response.data.message);
}

export async function requestJSONCandidates<T>(attempts: Array<() => Promise<CandidateJSONResponse<T>>>) {
  let lastResult: APIResponse<T> | null = null;
  let lastError: unknown = null;

  for (const attempt of attempts) {
    try {
      const response = await attempt();
      if (isMissingEndpointResponse(response)) {
        lastResult = response.data;
        continue;
      }
      return response.data;
    } catch (error) {
      if (isAxiosError(error) && [404, 405].includes(error.response?.status || 0)) {
        lastError = error;
        continue;
      }
      throw error;
    }
  }

  if (lastResult) return lastResult;
  if (lastError) throw lastError;
  throw new Error('兑换码接口不可用');
}

export function toErrorMessage(error: unknown, fallback: string) {
  if (error instanceof Error && error.message.trim()) return error.message;
  return fallback;
}
