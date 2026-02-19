import { expect, test } from '@playwright/test';

test.describe('theme regression', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('primary buttons are not bright-blue', async ({ page }) => {
    await page.goto('/login', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /登录 Realms/ }).first()).toBeVisible();

    const loginNav = page.getByRole('link', { name: '登录' });
    await expect(loginNav).toHaveClass(/active/);
    const loginNavBg = await loginNav.evaluate((el) => getComputedStyle(el).backgroundColor.trim());
    const normalizedNavBg = loginNavBg.replace(/\s+/g, '');
    expect(normalizedNavBg).not.toContain('13,110,253');
    if (normalizedNavBg) expect(normalizedNavBg).toContain('60,138,97');

    const button = page.locator('.btn.btn-primary').first();
    await expect(button).toBeVisible();

    const focusShadowRgb = await button.evaluate((el) =>
      getComputedStyle(el).getPropertyValue('--bs-btn-focus-shadow-rgb').trim(),
    );
    const normalizedFocus = focusShadowRgb.replace(/\s+/g, '');
    expect(normalizedFocus).not.toContain('13,110,253');
    if (normalizedFocus) expect(normalizedFocus).toBe('60,138,97');

    const backgroundColor = await button.evaluate((el) => getComputedStyle(el).backgroundColor.trim());
    const normalizedBg = backgroundColor.replace(/\s+/g, '');
    expect(normalizedBg).not.toContain('13,110,253');
    expect(normalizedBg).not.toContain('59,130,246');
    expect(normalizedBg).not.toContain('99,102,241');
  });
});

test.describe('theme regression (admin)', () => {
  test('form switches use theme primary when checked', async ({ page }) => {
    await page.goto('/admin/settings', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: '系统设置' })).toBeVisible();

    await page.getByRole('button', { name: /邮件服务/ }).click();
    const sw = page.getByRole('switch').first();
    await expect(sw).toBeEnabled();

    await sw.click();

    const backgroundColor = await sw.evaluate((el) => getComputedStyle(el).backgroundColor.trim());
    const normalizedBg = backgroundColor.replace(/\s+/g, '');
    expect(normalizedBg).not.toContain('13,110,253');
    if (normalizedBg) expect(normalizedBg).toContain('60,138,97');
  });
});
