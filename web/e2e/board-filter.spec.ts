import { test, expect } from '@playwright/test';

// The quick-filter narrows visible cards in place and shows an empty state when
// nothing matches.
test('quick-filter narrows visible cards', async ({ page, request }) => {
  const nonce = `filter-${Date.now()}`;
  await request.post('/api/tickets', { data: { type: 'feature', title: `Match ${nonce}`, status: 'todo' } });

  await page.goto('/board');
  await expect(page.getByTestId('board')).toBeVisible();

  await page.getByTestId('board-filter-text').fill(nonce);
  await expect(page.getByTestId('col-list-todo').getByText(`Match ${nonce}`)).toBeVisible();

  // A term nothing matches empties the column and shows its empty state.
  await page.getByTestId('board-filter-text').fill('zzz-no-match-zzz');
  await expect(page.getByTestId('col-empty-todo')).toBeVisible();
});
