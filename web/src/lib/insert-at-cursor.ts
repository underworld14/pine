/** Result of inserting text into a string at a caret position. */
export interface InsertResult {
  value: string;
  caret: number;
}

/**
 * Insert `snippet` into `value` at the given caret (or at the end if out of range).
 * When `el` is provided, uses its selection range so a selection is replaced.
 */
export function insertAtCursor(
  value: string,
  snippet: string,
  el?: Pick<HTMLTextAreaElement, 'selectionStart' | 'selectionEnd'> | null
): InsertResult {
  const start = el?.selectionStart ?? value.length;
  const end = el?.selectionEnd ?? start;
  const s = Math.max(0, Math.min(start, value.length));
  const e = Math.max(s, Math.min(end, value.length));
  const next = value.slice(0, s) + snippet + value.slice(e);
  return { value: next, caret: s + snippet.length };
}

/** Replace every occurrence of `from` with `to`. */
export function replaceAll(text: string, from: string, to: string): string {
  if (!from) return text;
  return text.split(from).join(to);
}

/** Markdown placeholder used before upload resolves to a real attachment path. */
export function uploadPlaceholder(key: string, name: string): string {
  const alt = name.replace(/\.[^.]+$/, '') || name || 'image';
  return `![${alt}](pine-upload:${key})`;
}

/** Match any markdown image that points at a pine-upload key (alt text may vary). */
export function uploadPlaceholderPattern(key: string): RegExp {
  const escaped = key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return new RegExp(`!\\[[^\\]]*\\]\\(pine-upload:${escaped}\\)`, 'g');
}

export function replaceUploadPlaceholder(text: string, key: string, markdown: string): string {
  return text.replace(uploadPlaceholderPattern(key), markdown);
}

export function removeUploadPlaceholder(text: string, key: string): string {
  return text
    .replace(uploadPlaceholderPattern(key), '')
    .replace(/\n{3,}/g, '\n\n');
}

/**
 * After create+upload, swap each staged key for the server markdown (or strip on failure).
 * `results` is parallel to `keys`.
 */
export function rewriteStagedUploads(
  body: string,
  keys: string[],
  results: Array<{ markdown?: string; error?: string } | undefined>
): string {
  let next = body;
  for (let i = 0; i < keys.length; i++) {
    const r = results[i];
    if (r && !r.error && r.markdown) next = replaceUploadPlaceholder(next, keys[i], r.markdown);
    else next = removeUploadPlaceholder(next, keys[i]);
  }
  for (const key of keys) next = removeUploadPlaceholder(next, key);
  return next;
}

/** GitHub-style transient placeholder while an existing-ticket upload is in flight. */
export function uploadingPlaceholder(name: string): string {
  return `![Uploading ${name}…]()`;
}

/** Remove markdown image/link refs that point at one attachment file. */
export function stripAttachmentMarkdown(body: string, ticketId: string, name: string): string {
  const escId = ticketId.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const escName = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  // Disk paths use ../attachments/…; preview/HTML may use /attachments/….
  const path = `(?:\\.\\./|/)?attachments/${escId}/${escName}`;
  const re = new RegExp(`!?\\[[^\\]]*\\]\\(${path}\\)`, 'g');
  return body.replace(re, '').replace(/\n{3,}/g, '\n\n');
}
