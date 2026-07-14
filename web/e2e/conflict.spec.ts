import { test, expect } from '@playwright/test';

test('dirty editor shows conflict banner when disk changes', async ({ page, request }) => {
  const title = `E2E conflict ${Date.now()}`;
  const ticket = await request.post('/api/tickets', {
    data: {
      type: 'bug',
      title,
      body: '# Description\n\nOriginal body\n'
    }
  }).then((r) => r.json());

  await page.goto(`/tickets/${ticket.id}`);
  await expect(page.locator('input.title')).toHaveValue(title);

  // Enter edit mode and dirtify the body without saving.
  await page.locator('body').press('ControlOrMeta+e');
  const textarea = page.locator('textarea');
  await expect(textarea).toBeVisible();
  await textarea.fill('# Description\n\nLocal draft that must conflict\n');

  // External write advances the disk hash (simulates an agent edit).
  const res = await request.patch(`/api/tickets/${ticket.id}`, {
    data: { body: '# Description\n\nChanged on disk by agent\n' },
    headers: { 'If-Match': ticket.hash, 'Content-Type': 'application/json' }
  });
  expect(res.ok()).toBeTruthy();

  const banner = page.getByTestId('conflict-banner');
  await expect(banner).toBeVisible({ timeout: 10000 });
  await expect(banner.getByRole('button', { name: 'Reload from disk' })).toBeVisible();
  await expect(banner.getByRole('button', { name: 'Keep mine & overwrite' })).toBeVisible();

  await banner.getByRole('button', { name: 'Reload from disk' }).click();
  await expect(banner).toBeHidden({ timeout: 10000 });

  // Leave edit mode so we assert rendered preview text (not textarea value).
  await page.locator('body').press('Escape');
  await expect(page.locator('.preview')).toContainText('Changed on disk by agent');
});
