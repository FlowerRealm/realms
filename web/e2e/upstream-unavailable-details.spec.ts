import fs from 'node:fs/promises';
import path from 'node:path';

import { test, expect, type Page } from '@playwright/test';

import { E2E_SEED } from './seed';

type APIResponse<T> = {
  success: boolean;
  message?: string;
  data: T;
};

type AdminUsageEvent = {
  id: number;
  request_id: string;
  error_class: string;
  error_message: string;
};

type UsageEvent = {
  id: number;
  request_id: string;
  error_class?: string | null;
  error_message?: string | null;
};

async function loginAsBillingUser(page: Page) {
  await page.goto('/login', { waitUntil: 'commit' });
  await page.locator('input[name="login"]').fill(E2E_SEED.billing.user.username);
  await page.locator('input[name="password"]').fill(E2E_SEED.billing.user.password);
  await page.getByRole('button', { name: '立即登录' }).click();
  await page.waitForURL('**/dashboard', { timeout: 30_000, waitUntil: 'commit' });
}

function utcTodayStr(): string {
  const now = new Date();
  return `${now.getUTCFullYear()}-${String(now.getUTCMonth() + 1).padStart(2, '0')}-${String(now.getUTCDate()).padStart(2, '0')}`;
}

test.describe('upstream_unavailable error details', () => {
  test('admin shows upstream detail; user stays generic', async ({ page, browser, request }, testInfo) => {
    await page.setViewportSize({ width: 1600, height: 900 });
    const requestID = `pw-unavail-${Date.now()}`;

    const marker = `__pw_unavailable__ ${requestID}`;
    const input = [
      {
        role: 'user',
        content: [
          {
            type: 'input_text',
            text: marker,
          },
        ],
      },
    ];
    const resp = await request.post('/v1/responses', {
      headers: { Authorization: `Bearer ${E2E_SEED.billing.user.token}`, 'X-Request-Id': requestID },
      data: { model: E2E_SEED.billing.model, input, stream: false },
    });
    expect(resp.status()).toBe(502);
    await expect(resp.text()).resolves.toContain('上游不可用');

    const today = utcTodayStr();

    // Admin UI should show detailed upstream failure in Error Message.
    await page.goto('/admin/usage', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /全站用量统计/ }).first()).toBeVisible({ timeout: 30_000 });

    const dates = page.locator('input[type="date"]');
    await dates.nth(0).fill(today);
    await dates.nth(1).fill(today);

    const adminUsageRespPromise = page.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      if (!url.pathname.endsWith('/api/admin/usage')) return false;
      return url.searchParams.get('start') === today && url.searchParams.get('end') === today;
    });
    await page.getByRole('button', { name: '更新统计' }).click();
    const adminUsageResp = await adminUsageRespPromise;
    const adminPayload = (await adminUsageResp.json()) as APIResponse<{ events: AdminUsageEvent[] }>;
    expect(adminPayload.success).toBeTruthy();

    const adminEvent = (adminPayload.data?.events || []).find((e) => (e.request_id || '').trim() === requestID);
    expect(adminEvent).toBeTruthy();
    expect(adminEvent?.error_class).toBe('upstream_unavailable');
    expect(adminEvent?.error_message).toContain('最后一次失败');
    expect(adminEvent?.error_message).toContain('rate limited for e2e');

    const adminRow = page.locator('tbody tr[role="button"]').filter({ hasText: requestID }).first();
    await expect(adminRow).toBeVisible({ timeout: 30_000 });
    const adminDetailRespPromise = page.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      return url.pathname.endsWith(`/api/admin/usage/events/${adminEvent!.id}/detail`);
    });
    await adminRow.click();
    await adminDetailRespPromise;
    await expect(page.getByText(adminEvent!.error_message)).toBeVisible({ timeout: 30_000 });

    const outDir = path.join(process.cwd(), '..', 'output');
    await fs.mkdir(outDir, { recursive: true });
    await page.screenshot({ path: path.join(outDir, `pw-upstream-unavailable-admin-${requestID}.png`), fullPage: true });

    const adminTable = page.locator('.table-responsive').first();
    await adminTable.evaluate((el) => {
      el.scrollLeft = el.scrollWidth;
    });
    await page.waitForTimeout(150);
    await page.screenshot({ path: path.join(outDir, `pw-upstream-unavailable-admin-right-${requestID}.png`), fullPage: true });

    // User UI should mask Error Message as "上游不可用" for upstream_unavailable.
    const baseURL = (testInfo.project.use as { baseURL?: string }).baseURL || 'http://127.0.0.1:18181';
    const userContext = await browser.newContext({ baseURL, storageState: { cookies: [], origins: [] } });
    const userPage = await userContext.newPage();
    await userPage.setViewportSize({ width: 1600, height: 900 });
    await loginAsBillingUser(userPage);

    const tokensRespPromise = userPage.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      return url.pathname.endsWith('/api/token');
    });
    const usageWindowsRespPromise = userPage.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      return url.pathname.endsWith('/api/usage/windows');
    });
    await userPage.goto('/usage', { waitUntil: 'commit' });
    await tokensRespPromise;
    await usageWindowsRespPromise;
    await expect(userPage.getByRole('heading', { name: /用量统计/ }).first()).toBeVisible({ timeout: 30_000 });

    const userDates = userPage.locator('input[type="date"]');
    await userDates.nth(0).fill(today);
    await userDates.nth(1).fill(today);

    const usageEventsRespPromise = userPage.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      if (!url.pathname.endsWith('/api/usage/events')) return false;
      return url.searchParams.get('start') === today && url.searchParams.get('end') === today;
    });
    await userPage.getByRole('button', { name: '更新统计' }).click();
    const usageEventsResp = await usageEventsRespPromise;
    const usagePayload = (await usageEventsResp.json()) as APIResponse<{ events: UsageEvent[] }>;
    expect(usagePayload.success).toBeTruthy();

    const userEvent = (usagePayload.data?.events || []).find((e) => (e.request_id || '').trim() === requestID);
    expect(userEvent).toBeTruthy();
    expect(userEvent?.error_class).toBe('upstream_unavailable');
    expect(userEvent?.error_message).toBe('上游不可用');

    const userRow = userPage.locator('tr.rlm-usage-row').filter({ hasText: requestID }).first();
    await expect(userRow).toBeVisible({ timeout: 30_000 });
    const userDetailRespPromise = userPage.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      return url.pathname.endsWith(`/api/usage/events/${userEvent!.id}/detail`);
    });
    await userRow.click();
    await userDetailRespPromise;

    const detailRow = userPage.locator('tr.rlm-usage-detail-row').filter({ hasText: String(userEvent!.id) }).first();
    await expect(detailRow).toBeVisible({ timeout: 30_000 });
    await expect(detailRow.getByText('上游不可用')).toBeVisible({ timeout: 30_000 });
    await expect(detailRow.getByText('最后一次失败')).toHaveCount(0);

    await userPage.screenshot({ path: path.join(outDir, `pw-upstream-unavailable-user-${requestID}.png`), fullPage: true });

    const userTable = userPage.locator('.table-responsive').first();
    await userTable.evaluate((el) => {
      el.scrollLeft = el.scrollWidth;
    });
    await userPage.waitForTimeout(150);
    await userPage.screenshot({ path: path.join(outDir, `pw-upstream-unavailable-user-right-${requestID}.png`), fullPage: true });

    await userContext.close();
  });
});
