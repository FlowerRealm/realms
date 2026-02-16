import { test, expect } from '@playwright/test';

test.describe('tokens', () => {
  test('can create token then reveal/hide', async ({ page }) => {
    const ts = Date.now();
    const tokenName = `pw_token_${ts}`;

    await page.goto('/tokens', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /我的 API 令牌/ }).first()).toBeVisible({ timeout: 30_000 });

    await page.getByRole('button', { name: /创建令牌/ }).click();

    const createModal = page.locator('#createTokenModal');
    await expect(createModal).toBeVisible({ timeout: 30_000 });
    await createModal.locator('input[name="name"]').fill(tokenName);

    await createModal.getByRole('button', { name: '创建' }).click();
    await expect(createModal).toBeHidden({ timeout: 30_000 });

    const generatedModal = page.locator('#generatedTokenModal');
    await expect(generatedModal).toBeVisible({ timeout: 30_000 });

    const token = (await generatedModal.locator('input.form-control').inputValue()).trim();
    expect(token).not.toBe('');

    await generatedModal.getByRole('button', { name: '关闭' }).filter({ hasText: '关闭' }).click();
    await expect(generatedModal).toBeHidden({ timeout: 30_000 });

    const row = page.locator('tbody tr').filter({ hasText: tokenName }).first();
    await expect(row).toBeVisible({ timeout: 30_000 });

    const previewCell = row.locator('td').nth(1);
    await expect(previewCell).toContainText('sk_', { timeout: 30_000 });

    await row.getByRole('button', { name: '查看' }).click();
    await expect(previewCell).toContainText(token, { timeout: 30_000 });

    await row.getByRole('button', { name: '隐藏' }).click();
    await expect(previewCell).not.toContainText(token, { timeout: 30_000 });
    await expect(previewCell).toContainText('sk_', { timeout: 30_000 });
  });
});
