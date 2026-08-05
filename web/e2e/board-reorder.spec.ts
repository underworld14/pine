import { test, expect } from '@playwright/test';

// Manual reorder must stick: drag a card above its neighbor, and it stays there
// after a reload (persisted via the frontmatter `order` field).
test('reordering a card within a column persists across reload', async ({ page, request }) => {
  const nonce = `reorder-${Date.now()}`;
  const a = await request
    .post('/api/tickets', { data: { type: 'feature', title: `A ${nonce}`, status: 'todo' } })
    .then((r) => r.json());
  const b = await request
    .post('/api/tickets', { data: { type: 'feature', title: `B ${nonce}`, status: 'todo' } })
    .then((r) => r.json());

  await page.goto('/board');
  await expect(page.getByTestId('board')).toBeVisible();

  // Isolate our two cards with the quick-filter so the reorder is deterministic.
  await page.getByTestId('board-filter-text').fill(nonce);
  const list = page.getByTestId('col-list-todo');
  await expect(list.getByRole('listitem')).toHaveCount(2);

  // The card currently at the bottom is our mover.
  const secondText = await list.getByRole('listitem').nth(1).innerText();
  const moverId = secondText.includes(a.id) ? a.id : b.id;

  // svelte-dnd-action keyboard path: lift, move up one, drop.
  await list.getByRole('listitem').nth(1).focus();
  await page.keyboard.press('Enter');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('Enter');

  // The mover is now first, and the server persisted an `order` for it.
  await expect(list.getByRole('listitem').nth(0)).toContainText(moverId);
  await expect
    .poll(async () => {
      const t = await request.get(`/api/tickets/${moverId}`).then((r) => r.json());
      return typeof t.order === 'number' && t.order !== 0;
    })
    .toBe(true);

  // The manual order survives a reload.
  await page.reload();
  await page.getByTestId('board-filter-text').fill(nonce);
  await expect(page.getByTestId('col-list-todo').getByRole('listitem').nth(0)).toContainText(moverId);
});
