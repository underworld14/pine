import type { Ticket } from './api';

/**
 * True while a ticket's markdown body hasn't been loaded yet (the snapshot omits
 * it; see workspace.ensureBody). An editor buffer must never be seeded from a
 * pending body — `body ?? ''` would turn "not loaded" into "empty", and the next
 * autosave would wipe the ticket. Off-branch tickets always ship their body and
 * can't be saved, so they are never pending.
 */
export function bodyPending(t: Pick<Ticket, 'body' | 'readOnly'>): boolean {
  return t.body === undefined && !t.readOnly;
}

/** Editor buffer for a freshly opened ticket, or null while its body is still loading. */
export function initialEditorState(
  ticket: Ticket
): { text: string; baseBody: string; baseHash: string } | null {
  if (bodyPending(ticket)) return null;
  const body = ticket.body ?? '';
  return { text: body, baseBody: body, baseHash: ticket.hash };
}

export interface EditorReconcileInput {
  text: string;
  baseBody: string;
  baseHash: string;
  ticket: Ticket;
}

export interface EditorReconcileResult {
  text: string;
  baseBody: string;
  baseHash: string;
  conflict: Ticket | null;
}

/** Reconcile local editor state when the disk ticket hash changes. */
export function reconcileEditor(input: EditorReconcileInput): EditorReconcileResult {
  const { text, baseBody, baseHash, ticket } = input;
  // Hash changed but the body isn't loaded (a resync drops stale bodies): there is
  // nothing to compare against yet. Stay put; the caller re-fetches the body and
  // reconciles when it arrives.
  if (ticket.hash === baseHash || bodyPending(ticket)) {
    return { text, baseBody, baseHash, conflict: null };
  }
  const diskBody = ticket.body ?? '';
  if (text === baseBody) {
    return { text: diskBody, baseBody: diskBody, baseHash: ticket.hash, conflict: null };
  }
  if (diskBody !== baseBody) {
    return { text, baseBody, baseHash, conflict: ticket };
  }
  return { text, baseBody, baseHash: ticket.hash, conflict: null };
}
