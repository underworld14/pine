import { describe, it, expect } from 'vitest';
import {
  insertAtCursor,
  removeUploadPlaceholder,
  replaceAll,
  replaceUploadPlaceholder,
  rewriteStagedUploads,
  uploadPlaceholder,
  uploadingPlaceholder
} from './insert-at-cursor';

describe('insertAtCursor', () => {
  it('appends when no element is provided', () => {
    expect(insertAtCursor('hello', ' world')).toEqual({ value: 'hello world', caret: 11 });
  });

  it('inserts at the caret and replaces a selection', () => {
    const el = { selectionStart: 1, selectionEnd: 4 };
    expect(insertAtCursor('abcdef', 'XX', el)).toEqual({ value: 'aXXef', caret: 3 });
  });

  it('clamps out-of-range carets', () => {
    expect(insertAtCursor('ab', '!', { selectionStart: 99, selectionEnd: 99 })).toEqual({
      value: 'ab!',
      caret: 3
    });
  });
});

describe('replaceAll', () => {
  it('replaces every occurrence', () => {
    expect(replaceAll('aa-aa', 'aa', 'b')).toBe('b-b');
  });

  it('no-ops on empty needle', () => {
    expect(replaceAll('abc', '', 'x')).toBe('abc');
  });
});

describe('upload placeholders', () => {
  it('builds a pine-upload markdown stub', () => {
    expect(uploadPlaceholder('u1', 'shot.png')).toBe('![shot](pine-upload:u1)');
  });

  it('builds a GitHub-style uploading stub', () => {
    expect(uploadingPlaceholder('shot.png')).toBe('![Uploading shot.png…]()');
  });

  it('replaces a staged placeholder with server markdown', () => {
    const body = `# Description\n\n${uploadPlaceholder('u1', 'a.png')}\n`;
    const md = '![a](../attachments/BUG-1/a.webp)';
    expect(replaceUploadPlaceholder(body, 'u1', md)).toContain(md);
    expect(replaceUploadPlaceholder(body, 'u1', md)).not.toContain('pine-upload:');
  });

  it('removes a staged placeholder', () => {
    const body = `hi\n\n${uploadPlaceholder('u2', 'b.png')}\n\nok`;
    expect(removeUploadPlaceholder(body, 'u2')).toBe('hi\n\nok');
  });
});

describe('rewriteStagedUploads', () => {
  it('swaps successes and strips failures so the body is preview-ready', () => {
    const body = [
      '# Description',
      '',
      'Allow avatar upload',
      uploadPlaceholder('u1', 'image.png'),
      uploadPlaceholder('u2', 'fail.png'),
      ''
    ].join('\n');

    const next = rewriteStagedUploads(body, ['u1', 'u2'], [
      { markdown: '![image](../attachments/FEAT-1/image-abc.webp)' },
      { error: 'too large' }
    ]);

    expect(next).toContain('![image](../attachments/FEAT-1/image-abc.webp)');
    expect(next).not.toContain('pine-upload:');
    expect(next).not.toContain('fail.png');
    expect(next).toContain('Allow avatar upload');
  });

  it('strips leftovers when the upload response is shorter than staged', () => {
    const body = `x ${uploadPlaceholder('u9', 'solo.png')}`;
    expect(rewriteStagedUploads(body, ['u9'], [])).toBe('x ');
  });
});
