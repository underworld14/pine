import { test, expect } from '@playwright/test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const fixture = path.join(path.dirname(fileURLToPath(import.meta.url)), 'fixtures', 'tiny.png');

test('upload attachment on ticket detail', async ({ page, request }) => {
  const title = `E2E attach ${Date.now()}`;
  const ticket = await request.post('/api/tickets', {
    data: { type: 'bug', title, body: '# Description\n\n' }
  }).then((r) => r.json());

  await page.goto(`/tickets/${ticket.id}`);
  await expect(page.locator('input.title')).toHaveValue(title);

  await page.getByTestId('ticket-attach-input').setInputFiles(fixture);

  await expect(page.getByTestId('attachments')).toBeVisible({ timeout: 15000 });
  await expect(page.getByTestId('attachments').locator('img, a, video').first()).toBeVisible();

  const after = await request.get(`/api/tickets/${ticket.id}`).then((r) => r.json());
  expect(after.attachments?.length).toBeGreaterThan(0);
});
