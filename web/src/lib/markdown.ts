import MarkdownIt from 'markdown-it';
import DOMPurify from 'dompurify';

const md = new MarkdownIt({ html: true, linkify: true, breaks: false });

// Rewrite relative attachment paths (portable on disk) to the served URL, and
// force links to open safely.
const defaultImageRender = md.renderer.rules.image!;
md.renderer.rules.image = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  const srcIdx = token.attrIndex('src');
  if (srcIdx >= 0) {
    const src = token.attrs![srcIdx][1];
    token.attrs![srcIdx][1] = rewriteAttachment(src);
  }
  return defaultImageRender(tokens, idx, options, env, self);
};

function rewriteAttachment(src: string): string {
  const m = src.match(/(?:\.\.\/)?attachments\/(.+)$/);
  if (m) return `/attachments/${m[1]}`;
  return src;
}

export function renderMarkdown(source: string): string {
  const raw = md.render(source ?? '');
  return DOMPurify.sanitize(raw, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'form', 'script', 'iframe'],
    ADD_ATTR: ['target'],
    ALLOW_DATA_ATTR: false
  });
}
