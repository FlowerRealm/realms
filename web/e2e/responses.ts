import { type APIRequestContext, type APIResponse } from '@playwright/test';

type ResponsesRequest = {
  token: string;
  model: string;
  input: string;
};

const RETRYABLE_STATUSES = new Set([408, 409, 425, 429, 500, 502, 503, 504]);

function envBool(key: string): boolean {
  const value = (process.env[key] || '').trim().toLowerCase();
  return value === '1' || value === 'true' || value === 'yes' || value === 'on';
}

function firstNonEmptyEnv(keys: string[]): string {
  for (const key of keys) {
    const value = (process.env[key] || '').trim();
    if (value) return value;
  }
  return '';
}

export function isRealUpstreamEnabledForE2E(): boolean {
  if (envBool('REALMS_E2E_ENFORCE_REAL_UPSTREAM')) {
    return true;
  }

  const upstreamBaseURL = firstNonEmptyEnv(['REALMS_E2E_UPSTREAM_BASE_URL', 'REALMS_CI_UPSTREAM_BASE_URL']);
  const upstreamAPIKey = firstNonEmptyEnv(['REALMS_E2E_UPSTREAM_API_KEY', 'REALMS_CI_UPSTREAM_API_KEY']);
  return upstreamBaseURL !== '' && upstreamAPIKey !== '';
}

export async function postResponsesWithRetry(request: APIRequestContext, req: ResponsesRequest): Promise<APIResponse> {
  const useRetry = isRealUpstreamEnabledForE2E();
  const maxAttempts = useRetry ? 3 : 1;
  let lastResponse: APIResponse | null = null;

  for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
    const response = await request.post('/v1/responses', {
      headers: { Authorization: `Bearer ${req.token}` },
      data: { model: req.model, input: req.input, stream: false },
    });
    lastResponse = response;

    const status = response.status();
    if (!useRetry || status === 200 || !RETRYABLE_STATUSES.has(status) || attempt === maxAttempts) {
      return response;
    }

    await new Promise((resolve) => setTimeout(resolve, attempt * 800));
  }

  if (!lastResponse) {
    throw new Error('调用 /v1/responses 未获得响应');
  }
  return lastResponse;
}
