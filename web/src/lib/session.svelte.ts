// Administrator session state.

import { get, send, setCsrf, setUnauthorizedHandler } from './api';
import type { SessionInfo } from './types';

class Session {
  info = $state<SessionInfo | null>(null);
  expired = $state(false);
  /** Shown on the login form (e.g. after a password change). */
  notice = $state('');

  constructor() {
    setUnauthorizedHandler(() => {
      if (this.info?.authenticated) {
        this.info = { ...this.info, authenticated: false };
        this.expired = true;
      }
      setCsrf('');
    });
  }

  async refresh(): Promise<void> {
    const s = await get<SessionInfo>('/api/admin/session');
    setCsrf(s.csrf ?? '');
    this.info = s;
  }

  private async started(r: { csrf: string }): Promise<void> {
    setCsrf(r.csrf);
    this.expired = false;
    this.notice = '';
    await this.refresh();
  }

  async login(password: string): Promise<void> {
    await this.started(await send<{ csrf: string }>('POST', '/api/admin/login', { password }));
  }

  async setup(password: string): Promise<void> {
    await this.started(await send<{ csrf: string }>('POST', '/api/admin/setup', { password }));
  }

  async logout(): Promise<void> {
    try {
      await send('POST', '/api/admin/logout');
    } finally {
      setCsrf('');
      this.expired = false;
      await this.refresh().catch(() => {
        if (this.info) this.info = { ...this.info, authenticated: false };
      });
    }
  }

  /** After a password change the server closes every session. */
  loggedOut(notice = ''): void {
    this.notice = notice;
    setCsrf('');
    if (this.info) this.info = { ...this.info, authenticated: false };
  }
}

export const session = new Session();
