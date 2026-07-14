/** File/folder @-mention helpers for markdown editors. */

export type FileKind = 'file' | 'dir';

export interface FileSuggestItem {
  path: string;
  kind: FileKind;
}

export interface MentionTrigger {
  /** Absolute start index of the '@' in value. */
  start: number;
  /** Absolute end index (caret) — exclusive end of the query. */
  end: number;
  /** Text after '@' (may be empty). */
  query: string;
}

const triggerRe = /(^|[\s(])@([^\s@]*)$/;

/**
 * If the text immediately before `caret` is an active `@query` token,
 * return its span; otherwise null.
 */
export function detectMentionTrigger(value: string, caret: number): MentionTrigger | null {
  const before = value.slice(0, Math.max(0, Math.min(caret, value.length)));
  const m = before.match(triggerRe);
  if (!m || m.index == null) return null;
  const at = m.index + m[1].length;
  return { start: at, end: before.length, query: m[2] ?? '' };
}

/**
 * True when the caret sits in the body of a "# Related Files" markdown section
 * (after that heading, before the next same-or-higher heading).
 */
export function isInRelatedFilesSection(text: string, caret: number): boolean {
  const before = text.slice(0, Math.max(0, Math.min(caret, text.length)));
  const lines = before.split('\n');
  let inSection = false;
  for (const line of lines) {
    const h = line.match(/^(#{1,6})\s+(.*)$/);
    if (h) {
      const title = h[2].trim().toLowerCase();
      inSection = title === 'related files';
    }
  }
  return inSection;
}

/** Markdown snippet inserted when the user picks a suggestion. */
export function formatFileMention(item: FileSuggestItem, inRelatedFiles: boolean): string {
  const mention = `@${item.path}`;
  if (inRelatedFiles) {
    return `- ${mention}`;
  }
  return mention;
}

/**
 * Replace the active `@query` span with the formatted mention.
 * Returns the new value and caret position after the insert.
 */
export function insertFileMention(
  value: string,
  caret: number,
  item: FileSuggestItem,
  opts?: { inRelatedFiles?: boolean }
): { value: string; caret: number } {
  const trigger = detectMentionTrigger(value, caret);
  if (!trigger) {
    const snippet = formatFileMention(item, !!opts?.inRelatedFiles);
    const next = value.slice(0, caret) + snippet + value.slice(caret);
    return { value: next, caret: caret + snippet.length };
  }
  const inRelated = opts?.inRelatedFiles ?? isInRelatedFilesSection(value, caret);
  let snippet = formatFileMention(item, inRelated);
  // When inserting a Related Files bullet mid-line after '@', ensure newline before '- '.
  if (inRelated) {
    const prev = trigger.start > 0 ? value[trigger.start - 1] : '\n';
    if (prev !== '\n') snippet = '\n' + snippet;
  }
  const next = value.slice(0, trigger.start) + snippet + value.slice(trigger.end);
  return { value: next, caret: trigger.start + snippet.length };
}
