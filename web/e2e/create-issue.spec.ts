import { test, expect } from '@playwright/test';

// The ≤10s flow: open the modal, type a title, submit — the card appears on the
// board, and the ticket file exists on disk (asserted via the API).
test('create a bug from the modal and see it on the board', async ({ page, request }) => {
  await page.goto('/board');
  await page.locator('body').press('c');
  const title = `E2E bug ${Date.now()}`;
  await page.getByPlaceholder('Title').fill(title);
  await page.getByRole('button', { name: 'Create' }).click();
  await expect(page.getByText(title)).toBeVisible();

  // The API confirms it was persisted.
  const snap = await request.get('/api/snapshot').then((r) => r.json());
  expect(snap.tickets.some((t: { title: string }) => t.title === title)).toBeTruthy();
});

test('search finds a seeded ticket', async ({ page }) => {
  await page.goto('/search');
  await page.getByPlaceholder(/Search/).fill('seed');
  await expect(page.getByText('Seed feature')).toBeVisible();
});
