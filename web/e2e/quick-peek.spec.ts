import { test, expect } from '@playwright/test';

// The quick-peek popover changes a card's status without leaving the board.
test('quick-peek popover changes status', async ({ page, request }) => {
  const title = `Peek ${Date.now()}`;
  const t = await request
    .post('/api/tickets', { data: { type: 'feature', title, status: 'todo' } })
    .then((r) => r.json());

  await page.goto('/board');
  await expect(page.getByTestId('board')).toBeVisible();

  const card = page.getByTestId('col-list-todo').getByRole('listitem').filter({ hasText: title });
  await expect(card).toBeVisible();
  await card.hover();
  await page.getByTestId(`peek-${t.id}`).click();

  await expect(page.getByTestId('quick-peek')).toBeVisible();
  await page.getByTestId('quick-peek-status').selectOption('doing');

  await expect
    .poll(async () => {
      const after = await request.get(`/api/tickets/${t.id}`).then((r) => r.json());
      return after.status;
    })
    .toBe('doing');
});
