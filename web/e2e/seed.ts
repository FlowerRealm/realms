function envOr(key: string, fallback: string): string {
  const value = process.env[key]?.trim();
  return value ? value : fallback;
}

function envInt(key: string, fallback: number): number {
  const value = process.env[key]?.trim();
  if (!value) return fallback;
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

export const E2E_SEED = {
  root: {
    username: envOr('REALMS_E2E_USERNAME', 'root'),
    password: envOr('REALMS_E2E_PASSWORD', 'rootpass123'),
  },

  // cmd/realms-e2e/main.go 固定的按量计费种子（模型/用户/Token）
  billing: {
    model: envOr('REALMS_E2E_BILLING_MODEL', 'gpt-5.2'),
    user: {
      email: envOr('REALMS_E2E_BILLING_USER_EMAIL', 'e2e-user@example.com'),
      username: envOr('REALMS_E2E_BILLING_USER_USERNAME', 'e2euser'),
      password: envOr('REALMS_E2E_BILLING_USER_PASSWORD', 'pw-e2e-user-123'),
      token: envOr('REALMS_E2E_BILLING_USER_TOKEN', 'sk_playwright_e2e_user_token'),
    },
    poorUser: {
      email: envOr('REALMS_E2E_POOR_USER_EMAIL', 'e2e-poor@example.com'),
      username: envOr('REALMS_E2E_POOR_USER_USERNAME', 'e2epoor'),
      password: envOr('REALMS_E2E_POOR_USER_PASSWORD', 'pw-e2e-user-123'),
      token: envOr('REALMS_E2E_POOR_USER_TOKEN', 'sk_playwright_e2e_poor_token'),
    },
  },

  // cmd/realms-e2e/main.go 固定的 OAuth App 种子
  oauth: {
    clientId: envOr('REALMS_E2E_OAUTH_CLIENT_ID', 'oa_playwright_e2e'),
    appName: envOr('REALMS_E2E_OAUTH_APP_NAME', 'Playwright E2E App'),
    redirectURI: envOr('REALMS_E2E_OAUTH_REDIRECT_URI', 'https://example.com/callback'),
  },

  // SQLite 空库 + schema seed 下通常稳定为 1；这里使用常量以便测试覆盖动态路由。
  ids: {
    announcementId: envInt('REALMS_E2E_ID_ANNOUNCEMENT', 1),
    ticketOpenId: envInt('REALMS_E2E_ID_TICKET_OPEN', 1),
    topupOrderId: envInt('REALMS_E2E_ID_TOPUP_ORDER', 1),
    oauthAppId: envInt('REALMS_E2E_ID_OAUTH_APP', 1),
    subscriptionPlanId: envInt('REALMS_E2E_ID_SUBSCRIPTION_PLAN', 1),
    channelGroupId: envInt('REALMS_E2E_ID_CHANNEL_GROUP', 1),
  },
};
