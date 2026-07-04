import * as assert from 'node:assert';
import { getBoardHtml, makeNonce } from '../../src/webview-html';

describe('getBoardHtml', () => {
  const html = getBoardHtml({
    iframeSrc: 'http://127.0.0.1:3412/',
    frameOrigin: 'http://127.0.0.1:3412',
    nonce: 'TESTNONCE',
  });

  it('embeds the iframe src', () => {
    assert.ok(html.includes('src="http://127.0.0.1:3412/"'));
  });

  it('restricts frame-src to the server origin', () => {
    assert.ok(html.includes('frame-src http://127.0.0.1:3412'));
  });

  it('uses the nonce for inline script and style', () => {
    assert.ok(html.includes("script-src 'nonce-TESTNONCE'"));
    assert.ok(html.includes("style-src 'nonce-TESTNONCE'"));
    assert.ok(html.includes('nonce="TESTNONCE"'));
  });

  it('never allows unsafe-inline', () => {
    assert.ok(!html.includes('unsafe-inline'));
  });

  it('locks down default-src', () => {
    assert.ok(html.includes("default-src 'none'"));
  });
});

describe('makeNonce', () => {
  it('is 32 alphanumeric characters', () => {
    const n = makeNonce();
    assert.strictEqual(n.length, 32);
    assert.match(n, /^[A-Za-z0-9]{32}$/);
  });

  it('varies between calls', () => {
    assert.notStrictEqual(makeNonce(), makeNonce());
  });
});
