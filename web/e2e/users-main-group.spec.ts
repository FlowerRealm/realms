import { test, expect, type Page } from '@playwright/test';

import { E2E_SEED } from './seed';

async function login(page: Page, login: string, password: string) {
  await page.goto('/login', { waitUntil: 'commit' });
  await page.locator('input[name="login"]').fill(login);
  await page.locator('input[name="password"]').fill(password);
  await page.getByRole('button', { name: '立即登录' }).click();
  await page.waitForURL('**/dashboard', { timeout: 30_000, waitUntil: 'commit' });
}

test.describe('admin users main_group', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('can edit user_group when user has no main_group', async ({ page }) => {
    const ts = Date.now();
    const email = `pw-user-${ts}@example.com`;
    const username = `pwuser${ts}`;
    const password = 'pw-e2e-user-123';

    // 1) Register a new user (user.main_group will be empty by default).
    await page.goto('/register', { waitUntil: 'commit' });
    await page.locator('input[name="email"]').fill(email);
    await page.locator('input[name="username"]').fill(username);
    await page.locator('input[name="password"]').fill(password);
    await page.getByRole('button', { name: '创建账号' }).click();
    await page.waitForURL('**/dashboard', { timeout: 30_000, waitUntil: 'commit' });

    // 2) Logout the new user, then login as root.
    await page.request.get('/api/user/logout');
    await login(page, E2E_SEED.root.username, E2E_SEED.root.password);

    // 3) Edit the newly registered user in admin panel; user_group should be selectable and savable.
    await page.goto('/admin/users', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /用户管理/ }).first()).toBeVisible({ timeout: 30_000 });

    const row = page.locator('tr', { hasText: email }).first();
    await expect(row).toBeVisible({ timeout: 30_000 });
    await row.locator('button[title="编辑用户"]').click();

    await expect(page.locator('#editUserModal')).toBeVisible({ timeout: 30_000 });
    const select = page.locator('#editUserModal select.font-monospace').first();

    // Fix expectation: when user.main_group is empty, UI should default to the first enabled main_group (seeded: pw-e2e-users).
    await expect(select).toHaveValue('pw-e2e-users');

    await page.locator('#editUserModal button[type="submit"]').click();

    // After save+refresh, the user row should show the selected main_group.
    await expect(page.locator('tr', { hasText: email }).first()).toContainText('pw-e2e-users', { timeout: 30_000 });
  });
});
