// Board quick-filter — a pure predicate so the board can narrow cards in place
// (by free text, label, and priority) without a round-trip. Framework-free and
// unit-tested; the UI lives in components/BoardFilterBar.svelte.

import type { Ticket } from './api';

export interface BoardFilter {
  text: string;
  labels: string[];
  priorities: string[];
}

export function emptyFilter(): BoardFilter {
  return { text: '', labels: [], priorities: [] };
}

// isActive reports whether any constraint is set (drives the "clear"/empty UI).
export function isActive(f: BoardFilter): boolean {
  return f.text.trim() !== '' || f.labels.length > 0 || f.priorities.length > 0;
}

// Lowercased `id title labels` per ticket object, so a keystroke compares against
// strings built once instead of rebuilding them for every ticket. Tickets are
// replaced (never mutated) on change; the key fields are re-checked anyway so an
// in-place edit can't serve a stale haystack.
const hayCache = new WeakMap<Ticket, { id: string; title: string; labels: string[]; hay: string }>();

function haystack(t: Ticket): string {
  const hit = hayCache.get(t);
  if (hit && hit.id === t.id && hit.title === t.title && hit.labels === t.labels) return hit.hay;
  const hay = `${t.id} ${t.title} ${t.labels.join(' ')}`.toLowerCase();
  hayCache.set(t, { id: t.id, title: t.title, labels: t.labels, hay });
  return hay;
}

// matchesFilter is an AND across the set constraints: free text matches the id,
// title, or any label (case-insensitive substring); label/priority filters match
// when the card carries any selected value.
export function matchesFilter(t: Ticket, f: BoardFilter): boolean {
  const text = f.text.trim().toLowerCase();
  if (text) {
    if (!haystack(t).includes(text)) return false;
  }
  if (f.labels.length && !f.labels.some((l) => t.labels.includes(l))) return false;
  if (f.priorities.length && !f.priorities.includes(t.priority)) return false;
  return true;
}

// toggle returns a new array with value added or removed — the immutable helper
// the filter bar uses so Svelte reactivity sees a fresh reference.
export function toggle(list: string[], value: string): string[] {
  return list.includes(value) ? list.filter((v) => v !== value) : [...list, value];
}

// How long the board waits after the last keystroke before re-filtering.
export const TEXT_DEBOUNCE_MS = 120;

// isTextOnlyChange is true when only the free text differs — the one filter edit
// worth debouncing. Label/priority toggles are discrete clicks and apply at once.
export function isTextOnlyChange(prev: BoardFilter, next: BoardFilter): boolean {
  return (
    prev.text !== next.text &&
    prev.labels.length === next.labels.length &&
    prev.labels.every((l, i) => l === next.labels[i]) &&
    prev.priorities.length === next.priorities.length &&
    prev.priorities.every((p, i) => p === next.priorities[i])
  );
}
