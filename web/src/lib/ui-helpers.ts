export function priorityMeta(p: string): { label: string; color: string; glyph: string } {
  switch (p) {
    case 'critical': return { label: 'Critical', color: 'var(--color-danger)', glyph: '◆' };
    case 'high': return { label: 'High', color: 'var(--color-warn)', glyph: '▲' };
    case 'medium': return { label: 'Medium', color: 'var(--color-info)', glyph: '●' };
    case 'low': return { label: 'Low', color: 'var(--color-dim)', glyph: '▽' };
    default: return { label: p, color: 'var(--color-dim)', glyph: '○' };
  }
}

// Deterministic label hue from a fixed 8-color palette.
const HUES = [200, 150, 45, 280, 340, 20, 100, 250];
export function labelColor(label: string): string {
  let h = 0;
  for (let i = 0; i < label.length; i++) h = (h * 31 + label.charCodeAt(i)) >>> 0;
  return `hsl(${HUES[h % HUES.length]} 55% 55%)`;
}
