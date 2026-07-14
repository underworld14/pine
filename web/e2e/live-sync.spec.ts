import { test, expect } from '@playwright/test';

test('board reflects external API title change while live', async ({ page, request }) => {
  const title = `E2E sync ${Date.now()}`;
  const ticket = await request.post('/api/tickets', {
    data: { type: 'feature', title, status: 'todo' }
  }).then((r) => r.json());

  await page.goto('/board');
  await expect(page.getByTestId('sync-dot')).toHaveAttribute('data-connection', 'live', { timeout: 10000 });
  await expect(page.getByText(title)).toBeVisible();

  const nextTitle = `${title} updated`;
  const res = await request.patch(`/api/tickets/${ticket.id}`, {
    data: { title: nextTitle },
    headers: { 'If-Match': ticket.hash, 'Content-Type': 'application/json' }
  });
  expect(res.ok()).toBeTruthy();

  await expect(page.getByText(nextTitle)).toBeVisible({ timeout: 10000 });
  await expect(page.getByTestId('sync-dot')).toHaveAttribute('data-connection', 'live');
  await expect(page.getByTestId('sync-dot')).not.toHaveClass(/down/);
});
