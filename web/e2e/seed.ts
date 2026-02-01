export const E2E_SEED = {
  root: {
    username: process.env.REALMS_E2E_USERNAME?.trim() || 'root',
    password: process.env.REALMS_E2E_PASSWORD?.trim() || 'rootpass123',
  },

  // cmd/realms-e2e/main.go 固定的 OAuth App 种子
  oauth: {
    clientId: 'oa_playwright_e2e',
    appName: 'Playwright E2E App',
    redirectURI: 'https://example.com/callback',
  },

  // SQLite 空库 + schema seed 下通常稳定为 1；这里使用常量以便测试覆盖动态路由。
  ids: {
    announcementId: 1,
    ticketOpenId: 1,
    topupOrderId: 1,
    oauthAppId: 1,
    subscriptionPlanId: 1,
    channelGroupId: 1,
  },
};
