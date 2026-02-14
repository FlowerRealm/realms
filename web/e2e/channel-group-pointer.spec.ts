import { test, expect } from '@playwright/test';

import { E2E_SEED } from './seed';

test.describe('admin channel groups pointer', () => {
  test('can set pointer and list shows pointer even when not pinned', async ({ page }) => {
    const groupID = E2E_SEED.ids.channelGroupId;

    await page.goto('/admin/channel-groups', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /渠道组/ }).first()).toBeVisible({ timeout: 30_000 });

    const getRealmsUser = async () =>
      page.evaluate(() => {
        try {
          const raw = localStorage.getItem('user');
          if (!raw) return '';
          const parsed = JSON.parse(raw) as { id?: unknown };
          const id = parsed?.id;
          return typeof id === 'number' && id > 0 ? String(id) : '';
        } catch {
          return '';
        }
      });
    await expect.poll(getRealmsUser, { timeout: 30_000 }).not.toBe('');
    const realmsUser = await getRealmsUser();

    // 1) Arrange: set an unpinned pointer via API, then ensure list renders it (not '-')
    const candsResp = await page.request.get(`/api/admin/channel-groups/${groupID}/pointer/candidates`, {
      headers: { 'Realms-User': realmsUser },
    });
    expect(candsResp.ok()).toBeTruthy();
    const candsJson = (await candsResp.json()) as {
      success: boolean;
      message?: string;
      data?: Array<{ id: number; name?: string | null }>;
    };
    expect(candsJson.success, candsJson.message || 'list candidates failed').toBeTruthy();

    const cand = (candsJson.data || []).find((c) => (c.name || '').trim() === 'pw-e2e-upstream') || (candsJson.data || [])[0];
    expect(cand, 'missing pointer candidates').toBeTruthy();

    // 0) Ensure list shows a default pointer when no pointer record exists (at least one, highest priority).
    const defaultGroupName = `pw-e2e-default-pointer-${Date.now()}`;
    const createResp = await page.request.post('/api/admin/channel-groups', {
      headers: { 'Realms-User': realmsUser },
      data: { name: defaultGroupName, price_multiplier: '1', max_attempts: 5, status: 1 },
    });
    expect(createResp.ok()).toBeTruthy();
    const createJson = (await createResp.json()) as { success: boolean; message?: string; data?: { id?: number } };
    expect(createJson.success, createJson.message || 'create group failed').toBeTruthy();
    const newGroupID = typeof createJson.data?.id === 'number' ? createJson.data.id : 0;
    expect(newGroupID, 'missing created group id').toBeTruthy();

    const addMemberResp = await page.request.post(`/api/admin/channel-groups/${newGroupID}/children/channels`, {
      headers: { 'Realms-User': realmsUser },
      data: { channel_id: cand!.id },
    });
    expect(addMemberResp.ok()).toBeTruthy();
    const addMemberJson = (await addMemberResp.json()) as { success: boolean; message?: string };
    expect(addMemberJson.success, addMemberJson.message || 'add channel member failed').toBeTruthy();

    await page.goto('/admin/channel-groups', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /渠道组/ }).first()).toBeVisible({ timeout: 30_000 });
    const defaultRow = page.locator('tbody tr').filter({ hasText: defaultGroupName }).first();
    await expect(defaultRow).toBeVisible({ timeout: 30_000 });
    const defaultPointerCell = defaultRow.locator('td').nth(1);
    await expect(defaultPointerCell.locator('code')).toContainText('pw-e2e-upstream', { timeout: 30_000 });
    await expect(defaultPointerCell.locator('span[title="未固定"]')).toBeVisible({ timeout: 30_000 });

    const setUnpinnedResp = await page.request.put(`/api/admin/channel-groups/${groupID}/pointer`, {
      headers: { 'Realms-User': realmsUser },
      data: { channel_id: cand!.id, pinned: false },
    });
    expect(setUnpinnedResp.ok()).toBeTruthy();
    const setUnpinnedJson = (await setUnpinnedResp.json()) as { success: boolean; message?: string };
    expect(setUnpinnedJson.success, setUnpinnedJson.message || 'set pointer failed').toBeTruthy();

    await page.reload({ waitUntil: 'commit' });

    const row = page.locator('tbody tr').filter({ hasText: 'pw-e2e' }).first();
    await expect(row).toBeVisible({ timeout: 30_000 });

    const pointerCell = row.locator('td').nth(1);
    await expect(pointerCell.locator('code')).toContainText('pw-e2e-upstream', { timeout: 30_000 });
    await expect(pointerCell.locator('span[title="未固定"]')).toBeVisible({ timeout: 30_000 });

    // 2) Use UI to set pointer (pinned=true) from channel list (channel-level action).
    await page.goto('/admin/channels', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /上游渠道管理/ }).first()).toBeVisible({ timeout: 30_000 });

    const chRow = page.locator('tr[data-rlm-channel-row="main"]').filter({ hasText: 'pw-e2e-upstream' }).first();
    await expect(chRow).toBeVisible({ timeout: 30_000 });
    await chRow.locator('button[title="设为指针"]').click();

    const modal = page.locator('#setChannelPointerModal');
    await expect(modal).toBeVisible({ timeout: 30_000 });

    const select = modal.locator('select.form-select').first();
    await expect(select).toBeVisible({ timeout: 30_000 });

    const seededOpt = select.locator('option', { hasText: 'pw-e2e' }).first();
    const seededValue = await seededOpt.getAttribute('value');
    if (seededValue) {
      await select.selectOption(seededValue);
    }

    page.once('dialog', async (d) => {
      await d.accept();
    });
    await modal.getByRole('button', { name: '保存' }).click();

    await expect(modal).toBeHidden({ timeout: 30_000 });

    // 3) Verify channel-groups list reflects pinned pointer.
    await page.goto('/admin/channel-groups', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /渠道组/ }).first()).toBeVisible({ timeout: 30_000 });
    const rowAfter = page.locator('tbody tr').filter({ hasText: 'pw-e2e' }).first();
    await expect(rowAfter).toBeVisible({ timeout: 30_000 });
    const pointerCellAfter = rowAfter.locator('td').nth(1);
    await expect(pointerCellAfter.locator('code')).toContainText('pw-e2e-upstream', { timeout: 30_000 });
    await expect(pointerCellAfter.locator('span[title="已固定"]')).toBeVisible({ timeout: 30_000 });

    // 4) Ensure "清除指针" is not shown in group detail page.
    await page.goto(`/admin/channel-groups/${groupID}`, { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /渠道组/ }).first()).toBeVisible({ timeout: 30_000 });
    await expect(page.getByRole('button', { name: '清除指针' })).toHaveCount(0);
  });
});
