import type { ToastMessage } from './types';

class ToastState {
  toasts = $state<ToastMessage[]>([]);

  show(message: string, type: 'success' | 'error' | 'info' = 'info', duration = 3000) {
    const id = Math.random().toString(36).substring(2, 9);
    const timer = setTimeout(() => {
      this.remove(id);
    }, duration);
    const toast: ToastMessage = { id, message, type, timer };
    this.toasts.push(toast);
  }

  remove(id: string) {
    const idx = this.toasts.findIndex((t) => t.id === id);
    if (idx >= 0) {
      clearTimeout(this.toasts[idx].timer);
      this.toasts = this.toasts.filter((t) => t.id !== id);
    }
  }
}

export const toast = new ToastState();
