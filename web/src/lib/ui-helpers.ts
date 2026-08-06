export const PRIORITIES = ['low', 'medium', 'high', 'critical'] as const;
export type PriorityValue = (typeof PRIORITIES)[number];

export type PriorityMeta = {
  label: string;
  /** Compact control label — readable without a tooltip. */
  short: string;
  color: string;
  /** Legacy glyph; prefer `short` for UI. */
  glyph: string;
};

export function priorityMeta(p: string): PriorityMeta {
  switch (p) {
    case 'critical':
      return { label: 'Critical', short: 'Crit', color: 'var(--color-danger)', glyph: '◆' };
    case 'high':
      return { label: 'High', short: 'High', color: 'var(--color-warn)', glyph: '▲' };
    case 'medium':
      return { label: 'Medium', short: 'Med', color: 'var(--color-info)', glyph: '●' };
    case 'low':
      return { label: 'Low', short: 'Low', color: 'var(--color-dim)', glyph: '▽' };
    default:
      return { label: p, short: p, color: 'var(--color-dim)', glyph: '○' };
  }
}

// shortBranch trims a display branch name (drops any leading "origin/").
export function shortBranch(branch: string): string {
  return branch.replace(/^origin\//, '');
}

// Deterministic label hue from a fixed 8-color palette.
const HUES = [200, 150, 45, 280, 340, 20, 100, 250];
export function labelColor(label: string): string {
  let h = 0;
  for (let i = 0; i < label.length; i++) h = (h * 31 + label.charCodeAt(i)) >>> 0;
  return `hsl(${HUES[h % HUES.length]} 55% 55%)`;
}
