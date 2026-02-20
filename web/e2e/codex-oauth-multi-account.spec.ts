import { test, expect, type APIRequestContext } from '@playwright/test';

import { E2E_SEED } from './seed';

type ModelsList = {
  data?: { id?: string }[];
};

async function hasModel(request: APIRequestContext, modelID: string): Promise<boolean> {
  const resp = await request.get('/v1/models', {
    headers: { Authorization: `Bearer ${E2E_SEED.billing.user.token}` },
  });
  if (!resp.ok()) return false;
  const body = (await resp.json()) as ModelsList;
  const ids = (body.data || []).map((m) => (m.id || '').trim()).filter((s) => s);
  return ids.includes(modelID);
}

test.describe('codex_oauth multi-account failover', () => {
  test('usage_limit_reached marks balance and failovers; invalid_token disables and failovers', async ({ page, request }) => {
    const hasExhaust = await hasModel(request, E2E_SEED.codex.exhaustModel);
    const hasInvalid = await hasModel(request, E2E_SEED.codex.invalidModel);
    test.skip(!hasExhaust || !hasInvalid, '未启用 codex_oauth 虚拟上游 seed（可能在真实上游模式下运行）');

    const requestID1 = `pw-codex-exhaust-${Date.now()}`;
    const input1 = [
      {
        role: 'user',
        content: [{ type: 'input_text', text: `pw codex exhaust ${requestID1}` }],
      },
    ];
    const resp1 = await request.post('/v1/responses', {
      headers: { Authorization: `Bearer ${E2E_SEED.billing.user.token}`, 'X-Request-Id': requestID1 },
      data: { model: E2E_SEED.codex.exhaustModel, input: input1, stream: false },
    });
    expect(resp1.ok()).toBeTruthy();

    const requestID2 = `pw-codex-invalid-${Date.now()}`;
    const input2 = [
      {
        role: 'user',
        content: [{ type: 'input_text', text: `pw codex invalid ${requestID2}` }],
      },
    ];
    const resp2 = await request.post('/v1/responses', {
      headers: { Authorization: `Bearer ${E2E_SEED.billing.user.token}`, 'X-Request-Id': requestID2 },
      data: { model: E2E_SEED.codex.invalidModel, input: input2, stream: false },
    });
    expect(resp2.ok()).toBeTruthy();

    await page.goto('/admin/channels', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /上游渠道管理/ }).first()).toBeVisible({ timeout: 30_000 });

    const rows = page.locator('tr[data-rlm-channel-row="main"]');
    await expect(rows.first()).toBeVisible({ timeout: 30_000 });

    // exhaust channel: should show quota_error=余额用尽 and cooldown.
    const exhaustRow = rows.filter({ hasText: E2E_SEED.codex.exhaustChannelName }).first();
    await expect(exhaustRow).toBeVisible({ timeout: 30_000 });
    await exhaustRow.locator('td').first().click();
    const exhaustDetail = exhaustRow.locator('xpath=following-sibling::tr[contains(@class,"rlm-channel-detail-row")]').first();
    await expect(exhaustDetail).toBeVisible({ timeout: 30_000 });
    await exhaustDetail.getByRole('button', { name: '账号统计' }).click();
    await expect(exhaustDetail.getByText(E2E_SEED.codex.exhaustedEmail)).toBeVisible({ timeout: 30_000 });
    const exhaustedAccRow = exhaustDetail.locator('tbody tr').filter({ hasText: E2E_SEED.codex.exhaustedEmail }).first();
    await expect(exhaustedAccRow.getByText('余额用尽')).toBeVisible({ timeout: 30_000 });

    // invalid channel: should disable invalid account.
    const invalidRow = rows.filter({ hasText: E2E_SEED.codex.invalidChannelName }).first();
    await expect(invalidRow).toBeVisible({ timeout: 30_000 });
    await invalidRow.locator('td').first().click();
    const invalidDetail = invalidRow.locator('xpath=following-sibling::tr[contains(@class,"rlm-channel-detail-row")]').first();
    await expect(invalidDetail).toBeVisible({ timeout: 30_000 });
    await invalidDetail.getByRole('button', { name: '账号统计' }).click();
    await expect(invalidDetail.getByText(E2E_SEED.codex.invalidEmail)).toBeVisible({ timeout: 30_000 });
    const invalidAccRow = invalidDetail.locator('tbody tr').filter({ hasText: E2E_SEED.codex.invalidEmail }).first();
    await expect(invalidAccRow.getByText('已禁用')).toBeVisible({ timeout: 30_000 });
  });
});
