import { test, expect, type Page } from '@playwright/test';

import { E2E_SEED } from './seed';

const includeChannelTests = (process.env.REALMS_E2E_INCLUDE_CHANNEL_TESTS || '').trim() === '1';

async function login(page: Page) {
  await page.locator('input[name="login"]').fill(E2E_SEED.root.username);
  await page.locator('input[name="password"]').fill(E2E_SEED.root.password);
  await page.getByRole('button', { name: '立即登录' }).click();
  await page.waitForURL('**/dashboard', { timeout: 30_000 });
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
    // 偶发的资源/请求卡住会导致页面长时间停留在 RequireAuth 的加载态；
    // 这里做一次轻量级重试（reload + 重新导航），避免 E2E flake。
    await page.reload({ waitUntil: 'commit' });
    await open();
    await assertVisible();
  }
}

test.describe('public routes', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('GET /login', async ({ page }) => {
    await gotoAndExpectHeading(page, '/login', /登录 Realms/);
  });

  test('GET /register', async ({ page }) => {
    await gotoAndExpectHeading(page, '/register', /注册账号/);
  });
});

test.describe('oauth routes', () => {
  test('GET /oauth/authorize', async ({ page }) => {
    const q = new URLSearchParams({
      response_type: 'code',
      client_id: E2E_SEED.oauth.clientId,
      redirect_uri: E2E_SEED.oauth.redirectURI,
      scope: 'openid profile',
      state: 'pw-e2e',
    });
    await page.goto(`/oauth/authorize?${q.toString()}`, { waitUntil: 'commit' });
    if (page.url().includes('/login')) {
      await login(page);
      await page.goto(`/oauth/authorize?${q.toString()}`, { waitUntil: 'commit' });
    }
    await expect(page.getByRole('heading', { name: /应用授权/ })).toBeVisible();
    await expect(page.getByText(E2E_SEED.oauth.appName)).toBeVisible();
  });
});

test.describe('app routes', () => {
  test('GET /dashboard', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/dashboard', /今日费用/);
  });

  test('GET /announcements', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/announcements', /公告/);
  });

  test('GET /announcements/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/announcements/${E2E_SEED.ids.announcementId}`, /Playwright E2E Announcement/);
  });

  test('GET /tokens', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tokens', /我的 API 令牌/);
  });

  test('GET /models', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/models', /可用模型列表/);
  });

  test('GET /usage', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/usage', /用量统计/);
  });

  test('GET /account', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/account', /账号设置/);
  });

  test('GET /subscription', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/subscription', /我的订阅/);
  });

  test('GET /topup', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/topup', /创建充值订单/);
  });

  test('GET /pay/:kind/:orderId', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/pay/topup/${E2E_SEED.ids.topupOrderId}`, /支付/);
  });

  test('GET /pay/:kind/:orderId/success', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/pay/topup/${E2E_SEED.ids.topupOrderId}/success`, /支付/);
    await expect(page.getByText(/支付完成后会自动入账/).first()).toBeVisible();
  });

  test('GET /pay/:kind/:orderId/cancel', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/pay/topup/${E2E_SEED.ids.topupOrderId}/cancel`, /支付/);
    await expect(page.getByText(/支付已取消或未完成/)).toBeVisible();
  });

  test('GET /tickets', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tickets', /工单/);
  });

  test('GET /tickets/open', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tickets/open', /工单/);
  });

  test('GET /tickets/closed', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tickets/closed', /工单/);
  });

  test('GET /tickets/new', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/tickets/new', /创建工单/);
  });

  test('GET /tickets/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/tickets/${E2E_SEED.ids.ticketOpenId}`, /工单 #/);
  });
});

test.describe('admin routes', () => {
  test('GET /admin', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin', /仪表盘/);
  });

  test('GET /admin/channels', async ({ page }) => {
    test.skip(!includeChannelTests, '默认 E2E 不覆盖渠道管理页面');
    await gotoAuthedAndExpectHeading(page, '/admin/channels', /上游渠道管理/);
  });

  test('GET /admin/channel-groups', async ({ page }) => {
    test.skip(!includeChannelTests, '默认 E2E 不覆盖渠道分组页面');
    await gotoAuthedAndExpectHeading(page, '/admin/channel-groups', /分组/);
  });

  test('GET /admin/main-groups', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/main-groups', /用户分组管理/);
  });

  test('GET /admin/subscriptions', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/subscriptions', /订阅套餐/);
  });

  test('GET /admin/subscriptions/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/admin/subscriptions/${E2E_SEED.ids.subscriptionPlanId}`, /编辑套餐/);
  });

  test('GET /admin/orders', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/orders', /订单/);
  });

  test('GET /admin/payment-channels', async ({ page }) => {
    test.skip(!includeChannelTests, '默认 E2E 不覆盖渠道相关页面');
    await gotoAuthedAndExpectHeading(page, '/admin/payment-channels', /支付渠道/);
  });

  test('GET /admin/usage', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/usage', /全站用量统计/);
  });

  test('GET /admin/tickets', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/tickets', /工单管理/);
  });

  test('GET /admin/tickets/open', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/tickets/open', /工单管理/);
  });

  test('GET /admin/tickets/closed', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/tickets/closed', /工单管理/);
  });

  test('GET /admin/tickets/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/admin/tickets/${E2E_SEED.ids.ticketOpenId}`, /工单 #/);
  });

  test('GET /admin/announcements', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/announcements', /公告/);
  });

  test('GET /admin/models', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/models', /模型管理/);
  });

  test('GET /admin/users', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/users', /用户管理/);
  });

  test('GET /admin/oauth-apps', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/oauth-apps', /OAuth 应用/);
  });

  test('GET /admin/oauth-apps/:id', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, `/admin/oauth-apps/${E2E_SEED.ids.oauthAppId}`, /OAuth 应用/);
    await expect(page.getByText(E2E_SEED.oauth.clientId)).toBeVisible();
  });

  test('GET /admin/settings', async ({ page }) => {
    await gotoAuthedAndExpectHeading(page, '/admin/settings', /系统设置/);
  });
});
