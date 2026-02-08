import { test, expect, type Page } from '@playwright/test';

import { E2E_SEED } from './seed';

type APIResponse<T> = {
  success: boolean;
  message?: string;
  data: T;
};

type UsageEventsData = {
  events: Array<Record<string, unknown>>;
};

type UsageEventDetailData = {
  available: boolean;
} & Record<string, unknown>;

async function loginAsBillingUser(page: Page) {
  await page.locator('input[name="login"]').fill(E2E_SEED.billing.user.username);
  await page.locator('input[name="password"]').fill(E2E_SEED.billing.user.password);
  await page.getByRole('button', { name: '立即登录' }).click();
  await page.waitForURL('**/dashboard', { timeout: 30_000 });
}

test.describe('usage', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('user usage detail hides upstream channel', async ({ page, request }) => {
    const marker = `pw-usage-ok-${Date.now()}`;
    const resp = await request.post('/v1/responses', {
      headers: { Authorization: `Bearer ${E2E_SEED.billing.user.token}` },
      data: { model: E2E_SEED.billing.model, input: marker, stream: false },
    });
    expect(resp.status()).toBe(200);

    await page.goto('/login', { waitUntil: 'commit' });
    await loginAsBillingUser(page);

    const tokensRespPromise = page.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      return url.pathname.endsWith('/api/token');
    });
    const modelsDetailRespPromise = page.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      return url.pathname.endsWith('/api/user/models/detail');
    });
    await page.goto('/usage', { waitUntil: 'commit' });
    await tokensRespPromise;
    await modelsDetailRespPromise;
    await expect(page.getByRole('heading', { name: /用量统计/ }).first()).toBeVisible();

    const now = new Date();
    const utcToday = `${now.getUTCFullYear()}-${String(now.getUTCMonth() + 1).padStart(2, '0')}-${String(now.getUTCDate()).padStart(2, '0')}`;
    const dates = page.locator('input[type="date"]');
    await dates.nth(0).fill(utcToday);
    await dates.nth(1).fill(utcToday);

    const eventsRespPromise = page.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      if (!url.pathname.endsWith('/api/usage/events')) return false;
      return url.searchParams.get('start') === utcToday && url.searchParams.get('end') === utcToday;
    });
    await page.getByRole('button', { name: '更新统计' }).click();

    const eventsResp = await eventsRespPromise;
    const payload = (await eventsResp.json()) as APIResponse<UsageEventsData>;
    expect(payload.success).toBeTruthy();
    const events = payload.data.events;
    expect(events.length).toBeGreaterThan(0);
    for (const ev of events) {
      expect('upstream_channel_id' in ev).toBe(false);
      expect('upstream_channel_name' in ev).toBe(false);
      expect('upstream_endpoint_id' in ev).toBe(false);
      expect('upstream_credential_id' in ev).toBe(false);
    }

    const row = page.locator('tr.rlm-usage-row').first();
    await expect(row).toBeVisible();

    const detailRespPromise = page.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      return /\/api\/usage\/events\/\d+\/detail$/.test(url.pathname);
    });
    await row.click();
    const detailResp = await detailRespPromise;
    const detailPayload = (await detailResp.json()) as APIResponse<UsageEventDetailData>;
    expect(detailPayload.success).toBeTruthy();
    expect('upstream_request_body' in detailPayload.data).toBe(false);
    expect('upstream_response_body' in detailPayload.data).toBe(false);

    const detail = page.locator('tr.rlm-usage-detail-row').first();
    await expect(detail).toBeVisible();
    await expect(detail.getByText('上游渠道')).toHaveCount(0);
    await expect(detail.getByText('pw-e2e-upstream')).toHaveCount(0);
  });
});
