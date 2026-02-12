import { test, expect, type Page } from '@playwright/test';

async function rowIDs(page: Page): Promise<string[]> {
  const rows = page.locator('tr[data-rlm-channel-row="main"]');
  return rows.evaluateAll((els) => els.map((el) => el.getAttribute('data-rlm-channel-id') || ''));
}

async function dragBefore(page: Page, movingIndex: number, targetIndex: number) {
  const rows = page.locator('tr[data-rlm-channel-row="main"]');
  const movingRow = rows.nth(movingIndex);
  const targetRow = rows.nth(targetIndex);

  await movingRow.scrollIntoViewIfNeeded();
  await targetRow.scrollIntoViewIfNeeded();

  // Drag from the row content (not the small handle), to ensure full-row dragging works.
  const from = await movingRow.locator('td').nth(1).boundingBox();
  const to = await targetRow.boundingBox();
  if (!from || !to) throw new Error('missing bounding box for drag');

  const reorderResp = page.waitForResponse((resp) => resp.url().includes('/api/channel/reorder') && resp.request().method() === 'POST');

  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(to.x + to.width / 2, to.y + to.height * 0.25, { steps: 12 });
  await page.mouse.up();

  const resp = await reorderResp;
  expect(resp.ok()).toBeTruthy();
  const body = (await resp.json()) as { success?: boolean; message?: string };
  expect(body.success).toBeTruthy();
}

test.describe('admin channels', () => {
  test('can reorder channels by dragging row', async ({ page }) => {
    await page.goto('/admin/channels', { waitUntil: 'commit' });
    await expect(page.getByRole('heading', { name: /上游渠道管理/ }).first()).toBeVisible();

    const rows = page.locator('tr[data-rlm-channel-row="main"]');
    await expect(rows.first()).toBeVisible({ timeout: 30_000 });
    const count = await rows.count();
    expect(count).toBeGreaterThanOrEqual(2);

    const before = await rowIDs(page);
    expect(before[0]).toBeTruthy();
    expect(before[1]).toBeTruthy();
    expect(before[0]).not.toBe(before[1]);

    await dragBefore(page, 1, 0);

    const after = await rowIDs(page);
    expect(after[0]).toBe(before[1]);
    expect(after[1]).toBe(before[0]);

    // restore to keep suite deterministic
    await dragBefore(page, 1, 0);

    const restored = await rowIDs(page);
    expect(restored[0]).toBe(before[0]);
    expect(restored[1]).toBe(before[1]);
  });
});
