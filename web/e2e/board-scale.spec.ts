import { test, expect, type APIRequestContext } from '@playwright/test';

// Scale behaviour on a big board: one snapshot per load, a capped column with a
// "Show more" control, and ticket bodies that lazy-load (the snapshot omits them)
// without ever being blanked by the editor.
//
// The seeded cards go in `testing`, a column no other spec touches, so the
// 50-card cap here can't hide cards other specs expect to see in todo/doing.

const SEED = 300;

async function seed(request: APIRequestContext, nonce: string, n: number) {
  for (let i = 0; i < n; i++) {
    const res = await request.post('/api/tickets', {
      data: { type: 'feature', title: `Scale ${nonce} #${i}`, status: 'testing' }
    });
    expect(res.ok()).toBeTruthy();
  }
}

// How many cards the server holds in `testing` (a reused local server keeps the
// cards of earlier runs, so the expected badge is read back instead of assumed).
async function testingTotal(request: APIRequestContext): Promise<number> {
  const snap = await request.get('/api/snapshot?bodies=0').then((r) => r.json());
  return snap.tickets.filter((t: { status: string }) => t.status === 'testing').length;
}

test.describe('board at scale', () => {
  test.describe.configure({ mode: 'serial' });

  test('loads with a single snapshot and caps a long column behind "Show more"', async ({ page, request }) => {
    test.setTimeout(120_000);
    const nonce = `scale-${Date.now()}`;
    await seed(request, nonce, SEED);
    const total = await testingTotal(request);
    expect(total).toBeGreaterThanOrEqual(SEED);

    const snapshots: string[] = [];
    page.on('request', (r) => {
      if (new URL(r.url()).pathname === '/api/snapshot') snapshots.push(r.url());
    });
    const snapshotResponse = page.waitForResponse((r) => new URL(r.url()).pathname === '/api/snapshot');

    await page.goto('/board');
    await expect(page.getByTestId('board')).toBeVisible();
    await expect(page.getByTestId('sync-dot')).toHaveAttribute('data-connection', 'live', { timeout: 10000 });

    // (a) One snapshot per page load — the SSE open handler is the only trigger —
    // and it leaves the markdown bodies behind.
    const body = await (await snapshotResponse).json();
    expect(body.tickets.length).toBeGreaterThanOrEqual(SEED);
    const withBody = body.tickets.filter((t: { readOnly?: boolean; body?: string }) => !t.readOnly && t.body);
    expect(withBody).toHaveLength(0);
    await page.waitForTimeout(750); // room for a (wrong) second request to show up
    expect(snapshots).toHaveLength(1);
    expect(snapshots[0]).toContain('bodies=0');

    // (b) The column renders one page of cards, but its badge counts every card.
    const cards = page.getByTestId('col-list-testing').locator('a[href^="/tickets/"]');
    await expect(cards).toHaveCount(50);
    await expect(page.getByTestId('col-count-testing')).toHaveText(String(total));
    const more = page.getByTestId('col-more-testing');
    await expect(more).toContainText('Show 50 more');

    await more.click();
    await expect(cards).toHaveCount(100);
    await expect(page.getByTestId('col-count-testing')).toHaveText(String(total)); // still the total
    await expect(page.getByTestId('col-more-testing')).toBeVisible();
  });

  test('a ticket body lazy-loads on open and survives edit + save', async ({ page, request }) => {
    const nonce = `lazy-${Date.now()}`;
    const original = `# Lazy heading\n\nBody text ${nonce}`;
    const ticket = await request
      .post('/api/tickets', {
        data: { type: 'feature', title: `Lazy ${nonce}`, status: 'testing', body: original }
      })
      .then((r) => r.json());

    // In-app navigation: the board's snapshot has no body for this card, so the
    // detail page has to fetch it. The filter narrows the (capped) column to it.
    await page.goto('/board');
    await expect(page.getByTestId('board')).toBeVisible();
    await page.getByTestId('board-filter-text').fill(nonce);
    const card = page.getByTestId('col-list-testing').locator(`a[href="/tickets/${ticket.id}"]`);
    await expect(card).toBeVisible();
    await card.click();

    // (c) The body shows up once loaded...
    const preview = page.locator('.preview');
    await expect(preview).toContainText(`Body text ${nonce}`);
    await expect(page.getByTestId('body-loading')).toHaveCount(0);

    // ...and editing + saving starts from it (never from an empty buffer).
    await preview.dblclick();
    const editor = page.locator('textarea');
    await expect(editor).toHaveValue(new RegExp(`Body text ${nonce}`));
    const edited = `${original}\n\nEdited ${nonce}`;
    await editor.fill(edited);
    await page.keyboard.press('Control+s');

    await expect
      .poll(async () => (await request.get(`/api/tickets/${ticket.id}`).then((r) => r.json())).body)
      .toContain(`Edited ${nonce}`);
    const saved = await request.get(`/api/tickets/${ticket.id}`).then((r) => r.json());
    expect(saved.body).toContain(`Body text ${nonce}`);

    // A cold load of the detail page (no board visit first) shows the saved body too.
    await page.reload();
    await expect(page.locator('.preview')).toContainText(`Edited ${nonce}`);
    await expect(page.locator('.preview')).toContainText(`Body text ${nonce}`);
  });

  test('closing a ticket opened cold never wipes its body', async ({ page, request }) => {
    const nonce = `cold-${Date.now()}`;
    const ticket = await request
      .post('/api/tickets', {
        data: { type: 'feature', title: `Cold ${nonce}`, status: 'testing', body: `Keep me ${nonce}` }
      })
      .then((r) => r.json());

    // Straight to the detail page, then away again before touching anything.
    await page.goto(`/tickets/${ticket.id}`);
    await page.keyboard.press('Control+s'); // ⌘S while (maybe) still loading must be inert
    await expect(page.locator('.preview')).toContainText(`Keep me ${nonce}`);
    await page.goto('/board');
    await expect(page.getByTestId('board')).toBeVisible();

    const after = await request.get(`/api/tickets/${ticket.id}`).then((r) => r.json());
    expect(after.body).toContain(`Keep me ${nonce}`);
  });
});
