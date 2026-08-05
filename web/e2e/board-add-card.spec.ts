import { test, expect } from '@playwright/test';

// The inline composer adds a card to the bottom of a column without the modal.
test('inline composer adds a card to a column', async ({ page }) => {
  const title = `Inline ${Date.now()}`;

  await page.goto('/board');
  await expect(page.getByTestId('board')).toBeVisible();

  await page.getByTestId('add-card-todo').click();
  const input = page.getByTestId('add-card-input-todo');
  await input.fill(title);
  await input.press('Enter');

  await expect(page.getByTestId('col-list-todo').getByText(title)).toBeVisible({ timeout: 10000 });
  // Composer stays open for rapid entry.
  await expect(page.getByTestId('add-card-input-todo')).toBeVisible();
});
