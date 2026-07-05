import { describe, it, expect } from 'vitest';
import { renderMarkdown } from './markdown';

describe('renderMarkdown task lists', () => {
  it('renders "- [ ]" as an enabled checkbox', () => {
    const html = renderMarkdown('# Acceptance Criteria\n- [ ] a\n- [x] b\n');
    expect(html).toContain('type="checkbox"');
    expect(html).not.toContain('disabled');
    expect((html.match(/type="checkbox"/g) ?? []).length).toBe(2);
  });
});
