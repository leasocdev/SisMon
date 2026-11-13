// Promise-based confirmation dialog shown by <ConfirmHost/>.

export interface ConfirmRequest {
  message: string;
  confirm: string;
  cancel: string;
  danger?: boolean;
  /** When set, the confirm button stays disabled until this exact text is typed. */
  typed?: string;
  resolve: (ok: boolean) => void;
}

export const confirmState = $state<{ current: ConfirmRequest | null }>({ current: null });

export function confirmDialog(message: string, confirm = 'Da', cancel = 'Renunță', danger = false, typed?: string): Promise<boolean> {
  confirmState.current?.resolve(false);
  return new Promise((resolve) => {
    confirmState.current = {
      message,
      confirm,
      cancel,
      danger,
      typed,
      resolve: (ok) => {
        confirmState.current = null;
        resolve(ok);
      },
    };
  });
}
