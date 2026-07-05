import type { Ticket } from './api';

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
  if (ticket.hash === baseHash) {
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
