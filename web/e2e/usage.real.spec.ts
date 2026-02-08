import fs from 'node:fs/promises';
import path from 'node:path';

import { test, expect } from '@playwright/test';

type AdminUsageAPIResponse = {
  success: boolean;
  message?: string;
  data?: {
    window?: {
      avg_first_token_latency?: string;
      tokens_per_second?: string;
    };
    events?: Array<{
      first_token_latency_ms?: string;
      tokens_per_second?: string;
      status_code?: string;
    }>;
  };
};

test.describe('usage real profile', () => {
  test('admin usage uses real TTFT/Tokens/s data', async ({ page }) => {
    await page.goto('/admin/usage', { waitUntil: 'commit' });
    if (page.url().includes('/login')) {
      throw new Error('real profile 需要有效 root 登录态，请检查 REALMS_E2E_USERNAME/REALMS_E2E_PASSWORD');
    }
    await expect(page.getByRole('heading', { name: /全站用量统计/ }).first()).toBeVisible();

    const usageRespPromise = page.waitForResponse((r) => {
      if (r.request().method() !== 'GET') return false;
      const url = new URL(r.url());
      return url.pathname.endsWith('/api/admin/usage');
    });
    await page.getByRole('button', { name: '更新统计' }).click();

    const usageResp = await usageRespPromise;
    const payload = (await usageResp.json()) as AdminUsageAPIResponse;
    expect(payload.success).toBeTruthy();

    const avgTTFT = (payload.data?.window?.avg_first_token_latency || '').trim();
    const avgTPS = (payload.data?.window?.tokens_per_second || '').trim();
    expect(avgTTFT).not.toBe('');
    expect(avgTPS).not.toBe('');
    expect(avgTTFT).not.toBe('-');
    expect(avgTPS).not.toBe('-');

    const hasEventWithMetrics = (payload.data?.events || []).some((ev) => {
      return (ev.first_token_latency_ms || '').trim() !== '-' && (ev.tokens_per_second || '').trim() !== '-';
    });
    expect(hasEventWithMetrics).toBeTruthy();

    const outputDir = path.resolve(process.cwd(), '../output/playwright');
    await fs.mkdir(outputDir, { recursive: true });
    await page.screenshot({ path: path.join(outputDir, 'e2e-real-admin-usage.png'), fullPage: true });
  });
});
