import { describe, it, expect } from 'vitest';
import { bodyPending, initialEditorState, reconcileEditor } from './ticket-editor';
import type { Ticket } from './api';

function mk(body: string, hash: string): Ticket {
  return {
    id: 'BUG-001',
    type: 'BUG',
    title: 'x',
    status: 'todo',
    priority: 'medium',
    labels: [],
    deps: [],
    created: '2026-07-04T00:00:00Z',
    updated: '2026-07-04T00:00:00Z',
    blocked: false,
    hash,
    body,
    attachments: []
  };
}

describe('reconcileEditor', () => {
  it('no-ops when hash unchanged', () => {
    const r = reconcileEditor({
      text: 'draft',
      baseBody: 'old',
      baseHash: 'h1',
      ticket: mk('disk', 'h1')
    });
    expect(r).toEqual({ text: 'draft', baseBody: 'old', baseHash: 'h1', conflict: null });
  });

  it('silently adopts disk when editor is clean', () => {
    const r = reconcileEditor({
      text: 'old',
      baseBody: 'old',
      baseHash: 'h1',
      ticket: mk('new body', 'h2')
    });
    expect(r.text).toBe('new body');
    expect(r.baseBody).toBe('new body');
    expect(r.baseHash).toBe('h2');
    expect(r.conflict).toBeNull();
  });

  it('keeps draft when only metadata changed on disk', () => {
    const r = reconcileEditor({
      text: 'unsaved draft',
      baseBody: 'old',
      baseHash: 'h1',
      ticket: mk('old', 'h2')
    });
    expect(r.text).toBe('unsaved draft');
    expect(r.baseBody).toBe('old');
    expect(r.baseHash).toBe('h2');
    expect(r.conflict).toBeNull();
  });

  it('flags conflict when disk body changed while editor is dirty', () => {
    const ticket = mk('agent rewrite', 'h2');
    const r = reconcileEditor({
      text: 'unsaved draft',
      baseBody: 'old',
      baseHash: 'h1',
      ticket
    });
    expect(r.text).toBe('unsaved draft');
    expect(r.conflict).toBe(ticket);
  });
});

describe('body not loaded yet (lazy snapshot bodies)', () => {
  function pending(hash: string): Ticket {
    const t = mk('', hash);
    delete t.body;
    return t;
  }

  it('bodyPending: only an undefined body on an editable ticket', () => {
    expect(bodyPending({ body: undefined })).toBe(true);
    expect(bodyPending({ body: '' })).toBe(false); // empty is loaded
    expect(bodyPending({ body: undefined, readOnly: true })).toBe(false); // off-branch: never lazy
  });

  it('initialEditorState refuses to seed a buffer from a pending body', () => {
    expect(initialEditorState(pending('h1'))).toBeNull();
    expect(initialEditorState(mk('hello', 'h1'))).toEqual({ text: 'hello', baseBody: 'hello', baseHash: 'h1' });
    expect(initialEditorState(mk('', 'h1'))).toEqual({ text: '', baseBody: '', baseHash: 'h1' });
  });

  it('reconcileEditor never adopts "" from a body-less ticket whose hash changed', () => {
    const clean = reconcileEditor({ text: 'old', baseBody: 'old', baseHash: 'h1', ticket: pending('h2') });
    expect(clean).toEqual({ text: 'old', baseBody: 'old', baseHash: 'h1', conflict: null });
    const dirty = reconcileEditor({ text: 'draft', baseBody: 'old', baseHash: 'h1', ticket: pending('h2') });
    expect(dirty).toEqual({ text: 'draft', baseBody: 'old', baseHash: 'h1', conflict: null });
  });

  it('reconciles normally once the body arrives', () => {
    const r = reconcileEditor({ text: 'old', baseBody: 'old', baseHash: 'h1', ticket: mk('new', 'h2') });
    expect(r.text).toBe('new');
    expect(r.baseHash).toBe('h2');
  });
});
