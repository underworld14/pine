export interface Toast {
  id: number;
  msg: string;
  kind: 'info' | 'success' | 'error';
  href?: string;
  action?: string;
}

class ToastStore {
  items = $state<Toast[]>([]);
  private n = 0;

  push(msg: string, kind: Toast['kind'] = 'info', opts: { href?: string; action?: string } = {}) {
    const id = ++this.n;
    this.items = [...this.items, { id, msg, kind, ...opts }];
    setTimeout(() => this.dismiss(id), 4500);
  }
  dismiss(id: number) {
    this.items = this.items.filter((t) => t.id !== id);
  }
}
export const toasts = new ToastStore();
