/** Approximate caret pixel position inside a textarea (mirror technique). */
export function caretViewportRect(
  el: HTMLTextAreaElement,
  caret: number
): { top: number; left: number; lineHeight: number } {
  const style = getComputedStyle(el);
  const div = document.createElement('div');
  const props = [
    'boxSizing',
    'width',
    'height',
    'overflowX',
    'overflowY',
    'borderTopWidth',
    'borderRightWidth',
    'borderBottomWidth',
    'borderLeftWidth',
    'paddingTop',
    'paddingRight',
    'paddingBottom',
    'paddingLeft',
    'fontStyle',
    'fontVariant',
    'fontWeight',
    'fontStretch',
    'fontSize',
    'fontSizeAdjust',
    'lineHeight',
    'fontFamily',
    'textAlign',
    'textTransform',
    'textIndent',
    'textDecoration',
    'letterSpacing',
    'wordSpacing',
    'tabSize',
    'whiteSpace',
    'wordBreak',
    'wordWrap'
  ] as const;
  div.style.position = 'absolute';
  div.style.visibility = 'hidden';
  div.style.whiteSpace = 'pre-wrap';
  div.style.wordWrap = 'break-word';
  div.style.top = '0';
  div.style.left = '-9999px';
  for (const p of props) {
    div.style[p] = style[p];
  }
  div.style.width = `${el.clientWidth}px`;
  div.textContent = el.value.slice(0, caret);
  const span = document.createElement('span');
  span.textContent = el.value.slice(caret) || '.';
  div.appendChild(span);
  document.body.appendChild(div);
  const rect = el.getBoundingClientRect();
  const lineHeight = parseFloat(style.lineHeight) || parseFloat(style.fontSize) * 1.45;
  const top = rect.top + (span.offsetTop - el.scrollTop) + lineHeight;
  const left = rect.left + (span.offsetLeft - el.scrollLeft);
  document.body.removeChild(div);
  return { top, left, lineHeight };
}
