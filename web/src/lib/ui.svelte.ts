class UIStore {
  modalOpen = $state(false);
  modalDefaults = $state<{ type?: string; status?: string }>({});
  paletteOpen = $state(false);
  theme = $state<'dark' | 'light'>('dark');

  openModal(defaults: { type?: string; status?: string } = {}) {
    this.modalDefaults = defaults;
    this.modalOpen = true;
  }
  closeModal() { this.modalOpen = false; }

  initTheme() {
    const t = (document.documentElement.dataset.theme as 'dark' | 'light') || 'dark';
    this.theme = t;
  }
  toggleTheme() {
    this.theme = this.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = this.theme;
    try { localStorage.setItem('pine-theme', this.theme); } catch { /* ignore */ }
  }
}
export const ui = new UIStore();
