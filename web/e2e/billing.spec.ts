import { test, expect, type Page } from '@playwright/test';

import { E2E_SEED } from './seed';
import { isRealUpstreamEnabledForE2E, postResponsesWithRetry } from './responses';

async function login(page: Page) {
  await page.locator('input[name="login"]').fill(E2E_SEED.root.username);
  await page.locator('input[name="password"]').fill(E2E_SEED.root.password);
  await page.getByRole('button', { name: '立即登录' }).click();
  await page.waitForURL('**/dashboard', { timeout: 30_000 });
}

async function gotoAuthed(page: Page, path: string) {
  await page.goto(path, { waitUntil: 'commit' });
  if (page.url().includes('/login')) {
    await login(page);
    await page.goto(path, { waitUntil: 'commit' });
  }
}

async function getUserBalanceUSDFromUsersTable(page: Page, email: string): Promise<number> {
  const row = page.locator('tbody tr').filter({ hasText: email });
  await expect(row).toHaveCount(1);
  const balText = (await row.locator('td').nth(5).innerText()).trim();
  const v = Number.parseFloat(balText);
  if (!Number.isFinite(v)) {
    throw new Error(`invalid balance text: ${balText}`);
  }
  return v;
}

test.describe('billing balance', () => {
  test('admin users: balance decreases after a /v1/responses call', async ({ page, request }) => {
    await gotoAuthed(page, '/admin/users');
    await expect(page.getByRole('heading', { name: /用户管理/ }).first()).toBeVisible();

    const email = E2E_SEED.billing.user.email;
    const before = await getUserBalanceUSDFromUsersTable(page, email);

    const resp = await postResponsesWithRetry(request, {
      token: E2E_SEED.billing.user.token,
      model: E2E_SEED.billing.model,
      input: `billing-balance-${Date.now()}`,
    });
    expect(resp.status()).toBe(200);

    await page.reload({ waitUntil: 'commit' });

    await expect
      .poll(async () => await getUserBalanceUSDFromUsersTable(page, email), { timeout: 20_000 })
      .toBeLessThan(before);

    if (!isRealUpstreamEnabledForE2E()) {
      const after = await getUserBalanceUSDFromUsersTable(page, email);
      const expectedDelta = 0.01; // fake upstream seed: input_tokens=1000, input_usd_per_1m=10 => 0.01 USD
      expect(after).toBeCloseTo(before - expectedDelta, 6);
    }
  });

  test('insufficient balance: /v1/responses returns 402', async ({ page, request }) => {
    const resp = await postResponsesWithRetry(request, {
      token: E2E_SEED.billing.poorUser.token,
      model: E2E_SEED.billing.model,
      input: 'hi',
    });
    expect(resp.status()).toBe(402);
    await expect(resp.text()).resolves.toContain('余额不足');

    await gotoAuthed(page, '/admin/users');
    await expect(page.getByRole('heading', { name: /用户管理/ }).first()).toBeVisible();

    const bal = await getUserBalanceUSDFromUsersTable(page, E2E_SEED.billing.poorUser.email);
    expect(bal).toBeCloseTo(0.0005, 6);
  });
});
