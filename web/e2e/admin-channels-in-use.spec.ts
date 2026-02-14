import fs from 'node:fs/promises';
import path from 'node:path';

import { test, expect } from '@playwright/test';

test.describe('admin channels (in_use)', () => {
  test('shows in-use badge for used channels', async ({ page }) => {
    await page.goto('/admin/channels', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /上游渠道管理/ }).first()).toBeVisible();

    const rows = page.locator('tr[data-rlm-channel-row="main"]');
    await expect(rows.first()).toBeVisible({ timeout: 30_000 });

    const seeded = rows.filter({ hasText: 'pw-e2e-upstream' }).first();
    const targetRow = (await seeded.count()) > 0 ? seeded : rows.filter({ hasText: '使用中' }).first();
    const badge = targetRow.getByText('使用中', { exact: true });
    await expect(badge).toBeVisible();

    await targetRow.scrollIntoViewIfNeeded();
    const outDir = path.join(process.cwd(), '..', 'output');
    await fs.mkdir(outDir, { recursive: true });
    const filePath = path.join(outDir, 'admin-channels-in-use.png');
    await targetRow.locator('td').first().screenshot({ path: filePath });
  });
});
