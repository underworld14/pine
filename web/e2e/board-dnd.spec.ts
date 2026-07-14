import { test, expect } from '@playwright/test';

test('drag card from todo to doing updates status', async ({ page, request }) => {
  const title = `E2E drag ${Date.now()}`;
  const created = await request.post('/api/tickets', {
    data: { type: 'feature', title, status: 'todo' }
  }).then((r) => r.json());

  await page.goto('/board');
  await expect(page.getByTestId('board')).toBeVisible();

  // svelte-dnd-action keyboard path: Enter to pick up, focus target zone, Enter to drop.
  // Focus the listitem wrapper (not the inner <a>) so Space/Enter start a drag.
  const item = page.getByTestId('col-list-todo').getByRole('listitem').filter({ hasText: title });
  await expect(item).toBeVisible();
  await item.focus();
  await page.keyboard.press('Enter');
  await page.getByTestId('col-list-doing').focus();
  await page.keyboard.press('Enter');

  await expect(page.getByTestId('col-list-doing').getByText(title)).toBeVisible({ timeout: 10000 });

  const after = await request.get(`/api/tickets/${created.id}`).then((r) => r.json());
  expect(after.status).toBe('doing');
});
