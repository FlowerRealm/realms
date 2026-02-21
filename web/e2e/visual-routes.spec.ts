import { test, expect, type Locator, type Page } from '@playwright/test';

import { E2E_SEED } from './seed';

async function login(page: Page) {
  await page.locator('input[name="login"]').fill(E2E_SEED.root.username);
  await page.locator('input[name="password"]').fill(E2E_SEED.root.password);
  await page.getByRole('button', { name: '立即登录' }).click();
  await page.waitForURL('**/dashboard', { timeout: 30_000 });
}

async function prepareForVisual(page: Page) {
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.emulateMedia({ reducedMotion: 'reduce' });

  // Visual snapshots should not depend on remote font/icon CSS (Playwright may wait on fonts indefinitely).
  await page.route(/https:\/\/fonts\.googleapis\.com\/.*/i, async (route) => {
    await route.fulfill({ status: 200, contentType: 'text/css; charset=utf-8', body: '' });
  });
  await page.route(/https:\/\/cdn\.jsdelivr\.net\/npm\/remixicon@.*\/fonts\/remixicon\.css/i, async (route) => {
    await route.fulfill({ status: 200, contentType: 'text/css; charset=utf-8', body: '' });
  });
}

async function stabilizeForScreenshot(page: Page) {
  await page.addStyleTag({
    content: `
      * { animation: none !important; transition: none !important; scroll-behavior: auto !important; }
      html { caret-color: transparent !important; overflow: hidden !important; }
      body { overflow: hidden !important; }
      /* Visual snapshots should be stable across machines: avoid remote webfonts/icon fonts. */
      :root {
        --bs-font-sans-serif: system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif !important;
        --bs-font-monospace: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace !important;
      }
      body { font-family: var(--bs-font-sans-serif) !important; }
      code, pre, .font-monospace { font-family: var(--bs-font-monospace) !important; }
      .material-symbols-rounded, i[class^="ri-"], i[class*=" ri-"] { visibility: hidden !important; }
    `,
  });

  await page.evaluate(async () => {
    // Wait for web fonts (if any) to avoid layout shifting between snapshots.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const fonts = (document as any).fonts;
    if (fonts && fonts.ready) {
      await Promise.race([fonts.ready, new Promise((r) => setTimeout(r, 1500))]);
    }
  });
}

async function gotoAndExpectHeading(page: Page, path: string, heading: RegExp) {
  await page.goto(path, { waitUntil: 'commit' });
  await expect(page.getByRole('heading', { name: heading }).first()).toBeVisible();
}

async function gotoAuthedAndExpectHeading(page: Page, path: string, heading: RegExp) {
  const open = async () => {
    await page.goto(path, { waitUntil: 'commit' });
    if (page.url().includes('/login')) {
      await login(page);
      await page.goto(path, { waitUntil: 'commit' });
    }
  };

  await open();

  const assertVisible = async () => {
    await expect(page.getByRole('heading', { name: heading }).first()).toBeVisible();
  };

  try {
    await assertVisible();
  } catch {
    // 与 routes.spec.ts 一致：轻量重试，避免偶发卡在 RequireAuth loading。
    await page.reload({ waitUntil: 'commit' });
    await open();
    await assertVisible();
  }
}

async function snapRoot(
  page: Page,
  name: string,
  options: {
    maxDiffPixelRatio?: number;
    mask?: Locator[];
  } = {},
) {
  await stabilizeForScreenshot(page);
  await expect(page.locator('#root')).toHaveScreenshot(name, {
    animations: 'disabled',
    caret: 'hide',
    maxDiffPixelRatio: options.maxDiffPixelRatio ?? 0.01,
    mask: options.mask,
  });
}

async function snapSidebar(page: Page, name: string) {
  await stabilizeForScreenshot(page);
  await expect(page.locator('aside.sidebar')).toHaveScreenshot(name, {
    animations: 'disabled',
    caret: 'hide',
    // Sidebar 变化通常应该是“可见且明确”的（例如文字颜色），这里更严格以避免被全页阈值吞掉。
    maxDiffPixelRatio: 0.001,
  });
}

async function snapLocator(page: Page, locator: Locator, name: string, maxDiffPixelRatio = 0.01) {
  await stabilizeForScreenshot(page);
  await expect(locator).toHaveScreenshot(name, {
    animations: 'disabled',
    caret: 'hide',
    maxDiffPixelRatio,
  });
}

async function expectSidebarLinkUsesMuted(page: Page) {
  const result = await page.evaluate(() => {
    const activeLink = document.querySelector('aside.sidebar a.sidebar-link.active') as HTMLElement | null;
    const inactiveLink = document.querySelector('aside.sidebar a.sidebar-link:not(.active)') as HTMLElement | null;
    const probe = document.createElement('span');
    probe.style.color = 'var(--rlm-text-muted)';
    probe.style.position = 'absolute';
    probe.style.left = '-99999px';
    document.body.appendChild(probe);
    const mutedColor = getComputedStyle(probe).color;
    probe.style.color = 'var(--rlm-heading)';
    const headingColor = getComputedStyle(probe).color;
    probe.remove();
    return {
      activeColor: activeLink ? getComputedStyle(activeLink).color : null,
      inactiveColor: inactiveLink ? getComputedStyle(inactiveLink).color : null,
      mutedColor,
      headingColor,
    };
  });

  expect(result.inactiveColor).toBe(result.mutedColor);
  expect(result.activeColor).toBe(result.headingColor);
}

test.describe('visual routes (seed)', () => {
  test.beforeEach(async ({ page }) => {
    await prepareForVisual(page);
  });

  test.describe('public (no auth)', () => {
    test.use({ storageState: { cookies: [], origins: [] } });

    test('visual /login', async ({ page }) => {
      await gotoAndExpectHeading(page, '/login', /登录 Realms/);
      await snapRoot(page, 'public-login.png');
    });

    test('visual /register', async ({ page }) => {
      await gotoAndExpectHeading(page, '/register', /注册账号/);
      await snapRoot(page, 'public-register.png');
    });

    test('visual not found', async ({ page }) => {
      await gotoAndExpectHeading(page, '/__playwright_not_found__', /404/);
      await snapRoot(page, 'public-404.png');
    });
  });

  test('visual /oauth/authorize', async ({ page }) => {
    const q = new URLSearchParams({
      response_type: 'code',
      client_id: E2E_SEED.oauth.clientId,
      redirect_uri: E2E_SEED.oauth.redirectURI,
      scope: 'openid profile',
      state: 'pw-e2e',
    });
    const path = `/oauth/authorize?${q.toString()}`;
    await page.goto(path, { waitUntil: 'commit' });
    if (page.url().includes('/login')) {
      await login(page);
      await page.goto(path, { waitUntil: 'commit' });
    }
    await expect(page.getByRole('heading', { name: /应用授权/ })).toBeVisible();
    await snapRoot(page, 'oauth-authorize.png');
  });

  test('visual /dashboard', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/dashboard', /今日费用/);
    await expectSidebarLinkUsesMuted(page);
    await snapRoot(page, 'app-dashboard.png');
    await snapSidebar(page, 'app-sidebar.png');

    // User dropdown menu (border + item outline)
    await page.locator('#dropdownUser1').click();
    const menu = page.locator('ul.dropdown-menu[aria-labelledby="dropdownUser1"]');
    await expect(menu).toBeVisible();
    await snapLocator(page, menu, 'app-user-menu.png', 0.005);
  });

  test('visual /guide', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/guide', /使用教程/);
    await expectSidebarLinkUsesMuted(page);
    await snapRoot(page, 'app-guide.png');
  });

  test('visual /announcements', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/announcements', /公告/);
    await snapRoot(page, 'app-announcements.png');
  });

  test('visual /announcements/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/announcements/${E2E_SEED.ids.announcementId}`, /Playwright E2E Announcement/);
    await snapRoot(page, 'app-announcement-detail.png');
  });

  test('visual /tokens', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tokens', /我的 API 令牌/);
    await snapRoot(page, 'app-tokens.png');
  });

  test('visual /tokens/created', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tokens/created', /令牌已生成/);
    await snapRoot(page, 'app-token-created.png');
  });

  test('visual /models', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/models', /可用模型列表/);
    await snapRoot(page, 'app-models.png');
  });

  test('visual /usage', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/usage', /用量统计/);
    await snapRoot(page, 'app-usage.png');
  });

  test('visual /account', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/account', /账号设置/);
    await snapRoot(page, 'app-account.png');
  });

  test('visual /subscription', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/subscription', /我的订阅/);
    await snapRoot(page, 'app-subscription.png');
  });

  test('visual /topup', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/topup', /创建充值订单/);
    await snapRoot(page, 'app-topup.png');
  });

  test('visual /pay/:kind/:orderId', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/pay/topup/${E2E_SEED.ids.topupOrderId}`, /支付/);
    await snapRoot(page, 'app-pay.png');
  });

  test('visual /pay/:kind/:orderId/success', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/pay/topup/${E2E_SEED.ids.topupOrderId}/success`, /支付/);
    await snapRoot(page, 'app-pay-success.png');
  });

  test('visual /pay/:kind/:orderId/cancel', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/pay/topup/${E2E_SEED.ids.topupOrderId}/cancel`, /支付/);
    await snapRoot(page, 'app-pay-cancel.png');
  });

  test('visual /tickets', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tickets', /工单/);
    await snapRoot(page, 'app-tickets.png');
  });

  test('visual /tickets/open', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tickets/open', /工单/);
    await snapRoot(page, 'app-tickets-open.png');
  });

  test('visual /tickets/closed', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tickets/closed', /工单/);
    await snapRoot(page, 'app-tickets-closed.png');
  });

  test('visual /tickets/new', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tickets/new', /创建工单/);
    await snapRoot(page, 'app-tickets-new.png');
  });

  test('visual /tickets/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/tickets/${E2E_SEED.ids.ticketOpenId}`, /工单 #/);
    await snapRoot(page, 'app-ticket-detail.png');
  });

  test('visual /admin', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin', /仪表盘/);
    await expectSidebarLinkUsesMuted(page);
    await snapRoot(page, 'admin-dashboard.png');
    await snapSidebar(page, 'admin-sidebar.png');
  });

  test('visual /admin/channels', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/channels', /上游渠道管理/);
    await snapRoot(page, 'admin-channels.png');

    // Expand first row to capture inline usage summary (includes "缓存" value).
    await page.locator('tr[data-rlm-channel-row="main"]').first().click();
    await expect(page.getByText('缓存:').first()).toBeVisible();
    await snapRoot(page, 'admin-channels-expanded.png');

    // Open channel settings modal to capture nav-tabs + boxed sections.
    await page.getByRole('button', { name: '设置' }).first().click();
    const modal = page.locator('#editChannelModal');
    await expect(modal).toHaveClass(/show/);
    await snapLocator(page, modal.locator('.modal-dialog'), 'admin-channel-settings-modal.png');
    await page.keyboard.press('Escape');
  });

  test('visual /admin/channel-groups', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/channel-groups', /渠道组/);
    await snapRoot(page, 'admin-channel-groups.png');
  });

  test('visual /admin/channel-groups/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/admin/channel-groups/${E2E_SEED.ids.channelGroupId}`, /渠道组/);
    await snapRoot(page, 'admin-channel-group-detail.png');
  });

  test('visual /admin/main-groups', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/main-groups', /用户分组管理/);
    await snapRoot(page, 'admin-main-groups.png');
  });

  test('visual /admin/subscriptions', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/subscriptions', /订阅套餐/);
    await snapRoot(page, 'admin-subscriptions.png');
  });

  test('visual /admin/subscriptions/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/admin/subscriptions/${E2E_SEED.ids.subscriptionPlanId}`, /编辑套餐/);
    await snapRoot(page, 'admin-subscription-detail.png');
  });

  test('visual /admin/orders', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/orders', /订单/);
    await snapRoot(page, 'admin-orders.png');
  });

  test('visual /admin/payment-channels', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/payment-channels', /支付渠道/);
    await snapRoot(page, 'admin-payment-channels.png');
  });

  test('visual /admin/usage', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/usage', /全站用量统计/);
    await snapRoot(page, 'admin-usage.png');
  });

  test('visual /admin/tickets', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/tickets', /工单管理/);
    await snapRoot(page, 'admin-tickets.png');
  });

  test('visual /admin/tickets/open', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/tickets/open', /工单管理/);
    await snapRoot(page, 'admin-tickets-open.png');
  });

  test('visual /admin/tickets/closed', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/tickets/closed', /工单管理/);
    await snapRoot(page, 'admin-tickets-closed.png');
  });

  test('visual /admin/tickets/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/admin/tickets/${E2E_SEED.ids.ticketOpenId}`, /工单 #/);
    await snapRoot(page, 'admin-ticket-detail.png');
  });

  test('visual /admin/announcements', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/announcements', /公告/);
    await snapRoot(page, 'admin-announcements.png');
  });

  test('visual /admin/models', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/models', /模型管理/);
    await snapRoot(page, 'admin-models.png');
  });

  test('visual /admin/users', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/users', /用户管理/);
    await snapRoot(page, 'admin-users.png');
  });

  test('visual /admin/oauth-apps', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/oauth-apps', /OAuth 应用/);
    await snapRoot(page, 'admin-oauth-apps.png');
  });

  test('visual /admin/oauth-apps/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/admin/oauth-apps/${E2E_SEED.ids.oauthAppId}`, /OAuth 应用/);
    await snapRoot(page, 'admin-oauth-app-detail.png');
  });

  test('visual /admin/settings', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/settings', /系统设置/);
    await snapRoot(page, 'admin-settings.png');
  });
});
