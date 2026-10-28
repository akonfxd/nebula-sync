import type {
  Info,
  Instance,
  InstanceInput,
  Profile,
  ProfileInput,
  Role,
  Run,
  Schedule,
  Schema,
  Session,
  Status,
  User,
} from './types';

/** ApiError carries the status so a caller can tell "not signed in" from "wrong input". */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }

  get unauthenticated(): boolean {
    return this.status === 401;
  }
}

const base = '/api/v1';

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let response: Response;

  try {
    response = await fetch(base + path, {
      method,
      credentials: 'same-origin',
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, 'the server is unreachable');
  }

  // Read the body even when it is empty: leaving the stream of a 204 unread
  // makes the browser cancel the request after it already succeeded.
  const text = await response.text();
  const payload: unknown = text === '' ? null : JSON.parse(text);

  if (!response.ok) {
    const message =
      payload !== null && typeof payload === 'object' && 'error' in payload
        ? String((payload as { error: unknown }).error)
        : `request failed with status ${response.status}`;

    throw new ApiError(response.status, message);
  }

  return payload as T;
}

export const api = {
  info: () => request<Info>('GET', '/info'),
  schema: () => request<Schema>('GET', '/schema'),

  bootstrap: (username: string, password: string) =>
    request<Session>('POST', '/auth/bootstrap', { username, password }),
  login: (username: string, password: string) =>
    request<Session>('POST', '/auth/login', { username, password }),
  logout: () => request<void>('POST', '/auth/logout'),
  session: () => request<Session>('GET', '/auth/session'),
  changeOwnPassword: (currentPassword: string, newPassword: string) =>
    request<Session>('POST', '/auth/password', { currentPassword, newPassword }),

  status: () => request<Status>('GET', '/status'),
  runs: (limit = 25) => request<Run[]>('GET', `/runs?limit=${limit}`),
  sync: () => request<{ status: string }>('POST', '/sync'),
  schedule: () => request<Schedule>('GET', '/schedule'),
  setSchedule: (cron: string) => request<Schedule>('PUT', '/schedule', { cron }),

  instances: () => request<Instance[]>('GET', '/instances'),
  createInstance: (input: InstanceInput) => request<Instance>('POST', '/instances', input),
  updateInstance: (id: number, input: InstanceInput) =>
    request<Instance>('PUT', `/instances/${id}`, input),
  deleteInstance: (id: number) => request<void>('DELETE', `/instances/${id}`),

  profiles: () => request<Profile[]>('GET', '/profiles'),
  createProfile: (input: ProfileInput) => request<Profile>('POST', '/profiles', input),
  updateProfile: (id: number, input: ProfileInput) =>
    request<Profile>('PUT', `/profiles/${id}`, input),
  deleteProfile: (id: number) => request<void>('DELETE', `/profiles/${id}`),

  users: () => request<User[]>('GET', '/users'),
  createUser: (username: string, password: string, role: Role) =>
    request<User>('POST', '/users', { username, password, role }),
  updateUser: (id: number, changes: { role?: Role; password?: string }) =>
    request<User>('PUT', `/users/${id}`, changes),
  deleteUser: (id: number) => request<void>('DELETE', `/users/${id}`),
};
