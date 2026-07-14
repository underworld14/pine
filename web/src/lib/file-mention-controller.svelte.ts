import { api } from '$lib/api';
import { caretViewportRect } from '$lib/caret-coords';
import {
  detectMentionTrigger,
  insertFileMention,
  isInRelatedFilesSection,
  type FileSuggestItem
} from '$lib/file-mention';

/** Controller for @ file/folder autocomplete on a textarea. */
export class FileMentionController {
  open = $state(false);
  items = $state<FileSuggestItem[]>([]);
  active = $state(0);
  loading = $state(false);
  top = $state(0);
  left = $state(0);

  private timer: ReturnType<typeof setTimeout> | null = null;
  private seq = 0;

  dispose() {
    if (this.timer) clearTimeout(this.timer);
  }

  close() {
    this.open = false;
    this.items = [];
    this.active = 0;
    this.loading = false;
  }

  /** Call from textarea input; returns true if a mention trigger is active. */
  onInput(el: HTMLTextAreaElement | null, value: string) {
    if (!el) {
      this.close();
      return false;
    }
    const caret = el.selectionStart ?? value.length;
    const trigger = detectMentionTrigger(value, caret);
    if (!trigger) {
      this.close();
      return false;
    }
    this.position(el, trigger.start);
    this.scheduleFetch(trigger.query);
    return true;
  }

  /**
   * Handle keys while the popup may be open.
   * Returns true when the event was consumed (caller should preventDefault).
   */
  onKeydown(
    e: KeyboardEvent,
    el: HTMLTextAreaElement | null,
    value: string,
    apply: (next: string, caret: number) => void
  ): boolean {
    if (!this.open) {
      // Allow Esc to bubble (e.g. leave edit mode) when popup closed.
      return false;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      this.close();
      return true;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      e.stopPropagation();
      if (this.items.length) this.active = Math.min(this.active + 1, this.items.length - 1);
      return true;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      e.stopPropagation();
      if (this.items.length) this.active = Math.max(this.active - 1, 0);
      return true;
    }
    // ⌘/Ctrl+Enter must reach New Issue submit; don't steal it for selection.
    if ((e.key === 'Enter' || e.key === 'Tab') && !e.metaKey && !e.ctrlKey) {
      if (!this.items.length) return false;
      e.preventDefault();
      e.stopPropagation();
      this.select(this.items[this.active], el, value, apply);
      return true;
    }
    return false;
  }

  select(
    item: FileSuggestItem,
    el: HTMLTextAreaElement | null,
    value: string,
    apply: (next: string, caret: number) => void
  ) {
    const caret = el?.selectionStart ?? value.length;
    const r = insertFileMention(value, caret, item, {
      inRelatedFiles: isInRelatedFilesSection(value, caret)
    });
    apply(r.value, r.caret);
    this.close();
  }

  private position(el: HTMLTextAreaElement, caret: number) {
    const { top, left } = caretViewportRect(el, caret);
    const maxLeft = window.innerWidth - 300;
    this.top = Math.min(top + 4, window.innerHeight - 250);
    this.left = Math.max(8, Math.min(left, maxLeft));
  }

  private scheduleFetch(query: string) {
    if (this.timer) clearTimeout(this.timer);
    this.loading = true;
    this.open = true;
    const seq = ++this.seq;
    this.timer = setTimeout(async () => {
      try {
        const res = await api.files(query);
        if (seq !== this.seq) return;
        this.items = res.items ?? [];
        this.active = 0;
      } catch {
        if (seq !== this.seq) return;
        this.items = [];
      } finally {
        if (seq === this.seq) this.loading = false;
      }
    }, 150);
  }
}
