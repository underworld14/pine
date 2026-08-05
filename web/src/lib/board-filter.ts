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

// matchesFilter is an AND across the set constraints: free text matches the id,
// title, or any label (case-insensitive substring); label/priority filters match
// when the card carries any selected value.
export function matchesFilter(t: Ticket, f: BoardFilter): boolean {
  const text = f.text.trim().toLowerCase();
  if (text) {
    const hay = `${t.id} ${t.title} ${t.labels.join(' ')}`.toLowerCase();
    if (!hay.includes(text)) return false;
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
