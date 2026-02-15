import { expect, test } from '@playwright/test';

test.describe('theme regression', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('primary buttons are not bright-blue', async ({ page }) => {
    await page.goto('/login', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /登录 Realms/ }).first()).toBeVisible();

    const button = page.locator('.btn.btn-primary').first();
    await expect(button).toBeVisible();

    const focusShadowRgb = await button.evaluate((el) =>
      getComputedStyle(el).getPropertyValue('--bs-btn-focus-shadow-rgb').trim(),
    );
    const normalizedFocus = focusShadowRgb.replace(/\s+/g, '');
    expect(normalizedFocus).not.toContain('13,110,253');
    if (normalizedFocus) expect(normalizedFocus).toBe('46,74,58');

    const backgroundColor = await button.evaluate((el) => getComputedStyle(el).backgroundColor.trim());
    const normalizedBg = backgroundColor.replace(/\s+/g, '');
    expect(normalizedBg).not.toContain('13,110,253');
    expect(normalizedBg).not.toContain('59,130,246');
    expect(normalizedBg).not.toContain('99,102,241');
  });
});
