// These mirror the json the api emits. They are hand written rather than
// generated, so a change in internal/api/dto.go belongs here too.

export type Role = 'admin' | 'viewer';
export type InstanceRole = 'primary' | 'replica';
export type FilterMode = 'none' | 'include' | 'exclude';

export interface Info {
  version: string;
  initialized: boolean;
  authenticated: boolean;
}

export interface User {
  id: number;
  username: string;
  role: Role;
  createdAt: string;
  lastLoginAt: string | null;
}

export interface Session {
  user: User;
  expiresAt: string;
}

export interface Instance {
  id: number;
  role: InstanceRole;
  name: string;
  url: string;
  hasPassword: boolean;
  enabled: boolean;
  profileId: number | null;
  position: number;
  updatedAt: string;
}

export interface InstanceInput {
  role: InstanceRole;
  name: string;
  url: string;
  /** Omitted entirely to keep the stored password. */
  password?: string;
  enabled: boolean;
  profileId: number | null;
  position: number;
}

export interface SectionSetting {
  enabled: boolean;
  filter: FilterMode;
  keys: string[];
}

export interface Profile {
  id: number;
  name: string;
  default: boolean;
  inUse: number;
  fullSync: boolean;
  runGravity: boolean;
  gravity: Record<string, boolean>;
  sections: Record<string, SectionSetting>;
}

export type ProfileInput = Omit<Profile, 'id' | 'default' | 'inUse'>;

export interface ReplicaOutcome {
  target: string;
  profile: string;
  mode: string;
  durationMs: number;
  error?: string;
}

export interface Outcome {
  timestamp: string;
  success: boolean;
  mode: string;
  durationMs: number;
  error?: string;
  replicas: ReplicaOutcome[];
}

export interface Run extends Outcome {
  id: number;
  startedAt: string;
  finishedAt: string;
  source: 'startup' | 'schedule' | 'manual';
}

export interface Schedule {
  cron: string;
  nextRun: string | null;
}

export interface Status {
  version: string;
  syncing: boolean;
  schedule: Schedule;
  latest: Outcome | null;
  outcomes: Outcome[];
}

export interface SectionSchema {
  name: string;
  syncable: boolean;
  reason?: string;
}

export interface Schema {
  sections: SectionSchema[];
  gravity: string[];
  filters: FilterMode[];
  userRoles: Role[];
  instanceRoles: InstanceRole[];
  passwordMinLength: number;
}
