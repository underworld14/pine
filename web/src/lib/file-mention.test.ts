import { describe, it, expect } from 'vitest';
import {
  detectMentionTrigger,
  formatFileMention,
  insertFileMention,
  isInRelatedFilesSection
} from './file-mention';

describe('detectMentionTrigger', () => {
  it('detects @ at start and after whitespace', () => {
    expect(detectMentionTrigger('@', 1)).toEqual({ start: 0, end: 1, query: '' });
    expect(detectMentionTrigger('see @src/', 9)).toEqual({ start: 4, end: 9, query: 'src/' });
    expect(detectMentionTrigger('(@login', 7)).toEqual({ start: 1, end: 7, query: 'login' });
  });

  it('returns null when not in a mention', () => {
    expect(detectMentionTrigger('email@x.com', 11)).toBeNull();
    expect(detectMentionTrigger('no at', 5)).toBeNull();
  });
});

describe('isInRelatedFilesSection', () => {
  it('is true only under Related Files', () => {
    const body = [
      '# Description',
      '',
      'hello',
      '',
      '# Related Files',
      '',
      '',
      '# Attachments',
      ''
    ].join('\n');
    const relatedCaret = body.indexOf('# Related Files') + '# Related Files\n\n'.length;
    const descCaret = body.indexOf('hello');
    const attachCaret = body.indexOf('# Attachments') + 5;
    expect(isInRelatedFilesSection(body, relatedCaret)).toBe(true);
    expect(isInRelatedFilesSection(body, descCaret)).toBe(false);
    expect(isInRelatedFilesSection(body, attachCaret)).toBe(false);
  });
});

describe('formatFileMention / insertFileMention', () => {
  it('formats @path inline and Related Files bullets', () => {
    expect(formatFileMention({ path: 'src/a.ts', kind: 'file' }, false)).toBe('@src/a.ts');
    expect(formatFileMention({ path: 'src/', kind: 'dir' }, true)).toBe('- @src/');
  });

  it('replaces the @query token with @path', () => {
    const value = 'see @log';
    const r = insertFileMention(value, value.length, { path: 'src/login.ts', kind: 'file' }, {
      inRelatedFiles: false
    });
    expect(r.value).toBe('see @src/login.ts');
    expect(r.caret).toBe(r.value.length);
  });

  it('keeps autocomplete alive when renaming an inserted path', () => {
    const value = '# Description\n\n@internal/server/gi';
    const trigger = detectMentionTrigger(value, value.length);
    expect(trigger).toEqual({
      start: value.indexOf('@'),
      end: value.length,
      query: 'internal/server/gi'
    });
  });

  it('inserts a Related Files bullet with @path', () => {
    const value = '# Related Files\n\n@api';
    const r = insertFileMention(value, value.length, { path: 'src/lib/api.ts', kind: 'file' });
    expect(r.value).toContain('- @src/lib/api.ts');
    expect(r.value).not.toMatch(/@api(?!\.ts)/);
  });
});

describe('mention key policy', () => {
  it('selects on bare Enter/Tab but not on meta/ctrl+Enter', () => {
    const selectKeys = (e: { key: string; metaKey?: boolean; ctrlKey?: boolean }) =>
      (e.key === 'Enter' || e.key === 'Tab') && !e.metaKey && !e.ctrlKey;
    expect(selectKeys({ key: 'Enter' })).toBe(true);
    expect(selectKeys({ key: 'Tab' })).toBe(true);
    expect(selectKeys({ key: 'Enter', metaKey: true })).toBe(false);
    expect(selectKeys({ key: 'Enter', ctrlKey: true })).toBe(false);
  });
});
