import type { ToastMessage } from './types';

class ToastState {
  toasts = $state<ToastMessage[]>([]);

  show(message: string, type: 'success' | 'error' | 'info' = 'info', duration = 3000) {
    const id = Math.random().toString(36).substring(2, 9);
    const toast: ToastMessage = { id, message, type };
    this.toasts.push(toast);

    setTimeout(() => {
      this.remove(id);
    }, duration);
  }

  remove(id: string) {
    this.toasts = this.toasts.filter((t) => t.id !== id);
  }
}

export const toast = new ToastState();
