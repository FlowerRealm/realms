import { test, expect } from '@playwright/test';

test.describe('admin main-groups', () => {
  test('can rename group and save', async ({ page }) => {
    const ts = Date.now();
    const groupName = `pw_group_${ts}`;
    const renamed = `pw_group_renamed_${ts}`;
    const firstDesc = 'created by playwright';
    const updatedDesc = 'updated by playwright';

    await page.goto('/admin/main-groups', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /用户分组管理/ }).first()).toBeVisible();

    await page.getByRole('button', { name: '创建用户分组' }).click();
    await expect(page.locator('#createMainGroupModal')).toBeVisible();

    await page.locator('#createMainGroupModal input[placeholder="例如: team_a"]').fill(groupName);
    await page.locator('#createMainGroupModal input[placeholder="可选"]').fill(firstDesc);
    await page.locator('#createMainGroupModal button[type="submit"]').click();

    // create 成功后会 refresh；页面 notice 会被 refresh 清空，这里直接等待表格行出现即可。
    await expect(page.locator('tr', { hasText: groupName }).first()).toBeVisible({ timeout: 30_000 });

    const row = page.locator('tr', { hasText: groupName }).first();
    await row.locator('button[title="编辑用户分组"]').click();
    await expect(page.locator('#editMainGroupModal')).toBeVisible();

    await expect(page.locator('#editMainGroupModal input.form-control.font-monospace')).toHaveValue(groupName);

    await page.locator('#editMainGroupModal input.form-control.font-monospace').fill(renamed);

    await page.locator('#editMainGroupModal input[placeholder="可选"]').fill(updatedDesc);
    await page.locator('#editMainGroupModal button[type="submit"]').click();

    await expect(page.locator('tr', { hasText: renamed }).first()).toBeVisible({ timeout: 30_000 });
    await expect(page.locator('tr', { hasText: renamed }).first()).toContainText(updatedDesc, { timeout: 30_000 });
  });
});
