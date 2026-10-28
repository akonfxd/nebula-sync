import { api, ApiError } from './api';
import type { Schema, User } from './types';

export type Page = 'dashboard' | 'instances' | 'profiles' | 'users' | 'account';

const pages: Page[] = ['dashboard', 'instances', 'profiles', 'users', 'account'];

export interface Toast {
  id: number;
  kind: 'ok' | 'error';
  message: string;
}

function pageFromHash(): Page {
  const name = globalThis.location.hash.replace(/^#\/?/, '') as Page;

  return pages.includes(name) ? name : 'dashboard';
}

/**
 * App is the state every view shares: who is signed in, what vocabulary the
 * server published, and which page the address bar points at. It is one rune
 * backed object rather than a store per concern, because every view needs the
 * session and the schema and almost nothing else.
 */
class App {
  user = $state<User | null>(null);
  schema = $state<Schema | null>(null);
  version = $state('');
  initialized = $state(true);
  ready = $state(false);
  page = $state<Page>(pageFromHash());
  toasts = $state<Toast[]>([]);

  private nextToast = 0;

  get admin(): boolean {
    return this.user?.role === 'admin';
  }

  listen() {
    globalThis.addEventListener('hashchange', () => {
      this.page = pageFromHash();
    });
  }

  go(page: Page) {
    globalThis.location.hash = `/${page}`;
    this.page = page;
  }

  /** load decides which screen to show: bootstrap, sign in, or the console. */
  async load() {
    try {
      const info = await api.info();

      this.version = info.version;
      this.initialized = info.initialized;

      if (info.authenticated) {
        await this.adopt();
      }
    } catch (error) {
      this.fail(error);
    } finally {
      this.ready = true;
    }
  }

  /** adopt pulls the signed in user and the vocabulary that depends on them. */
  async adopt() {
    const [session, schema] = await Promise.all([api.session(), api.schema()]);

    this.user = session.user;
    this.schema = schema;
    this.initialized = true;

    if (this.page === 'users' && session.user.role !== 'admin') {
      this.go('dashboard');
    }
  }

  async signOut() {
    try {
      await api.logout();
    } catch (error) {
      this.fail(error);
    }

    this.user = null;
    this.schema = null;
  }

  notify(message: string) {
    this.push('ok', message);
  }

  /**
   * fail turns any thrown value into a toast. An expired session drops straight
   * back to the sign in screen instead, because every other view would only
   * repeat the same 401.
   */
  fail(error: unknown) {
    if (error instanceof ApiError && error.unauthenticated && this.user !== null) {
      this.user = null;
      this.push('error', 'your session expired, sign in again');

      return;
    }

    this.push('error', error instanceof Error ? error.message : String(error));
  }

  dismiss(id: number) {
    this.toasts = this.toasts.filter((toast) => toast.id !== id);
  }

  private push(kind: Toast['kind'], message: string) {
    const id = this.nextToast++;

    this.toasts = [...this.toasts, { id, kind, message }];
    setTimeout(() => this.dismiss(id), kind === 'ok' ? 3000 : 8000);
  }
}

export const app = new App();
