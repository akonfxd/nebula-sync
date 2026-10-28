<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import { api } from '../lib/api';
  import { app } from '../lib/app.svelte';
  import type { FilterMode, Profile, ProfileInput, SectionSchema } from '../lib/types';

  interface Draft extends ProfileInput {
    id: number | null;
    isDefault: boolean;
  }

  let profiles = $state<Profile[]>([]);
  let loading = $state(true);
  let busy = $state(false);
  let draft = $state<Draft | null>(null);
  let removing = $state<Profile | null>(null);

  const sections = $derived<SectionSchema[]>(app.schema?.sections ?? []);
  const tables = $derived<string[]>(app.schema?.gravity ?? []);
  const filters = $derived<FilterMode[]>(app.schema?.filters ?? ['none', 'include', 'exclude']);

  async function refresh() {
    try {
      profiles = await api.profiles();
    } catch (error) {
      app.fail(error);
    } finally {
      loading = false;
    }
  }

  void refresh();

  /** blank builds a draft with every section and table the server knows about. */
  function blank(): Draft {
    const draftSections: ProfileInput['sections'] = {};
    for (const section of sections) {
      draftSections[section.name] = { enabled: false, filter: 'none', keys: [] };
    }

    const gravity: ProfileInput['gravity'] = {};
    for (const table of tables) {
      gravity[table] = false;
    }

    return {
      id: null,
      isDefault: false,
      name: '',
      fullSync: false,
      runGravity: false,
      gravity,
      sections: draftSections,
    };
  }

  function edit(profile: Profile) {
    const base = blank();

    draft = {
      ...base,
      id: profile.id,
      isDefault: profile.default,
      name: profile.name,
      fullSync: profile.fullSync,
      runGravity: profile.runGravity,
      gravity: { ...base.gravity, ...profile.gravity },
      sections: { ...base.sections, ...structuredClone(profile.sections) },
    };
  }

  function keysOf(name: string): string {
    return draft?.sections[name]?.keys.join(', ') ?? '';
  }

  function setKeys(name: string, raw: string) {
    if (draft === null) {
      return;
    }

    draft.sections[name].keys = raw
      .split(',')
      .map((key) => key.trim())
      .filter((key) => key !== '');
  }

  function summary(profile: Profile): string {
    if (profile.fullSync) {
      return 'everything';
    }

    const enabled = Object.entries(profile.sections)
      .filter(([, section]) => section.enabled)
      .map(([name]) => name);

    const gravity = Object.values(profile.gravity).filter(Boolean).length;
    const parts: string[] = [];

    if (enabled.length > 0) {
      parts.push(enabled.join(', '));
    }

    if (gravity > 0) {
      parts.push(`${gravity} gravity ${gravity === 1 ? 'table' : 'tables'}`);
    }

    return parts.length === 0 ? 'nothing' : parts.join(' · ');
  }

  async function save() {
    if (draft === null) {
      return;
    }

    const current = draft;
    const input: ProfileInput = {
      name: current.name.trim(),
      fullSync: current.fullSync,
      runGravity: current.runGravity,
      gravity: current.gravity,
      sections: current.sections,
    };

    busy = true;

    try {
      if (current.id === null) {
        await api.createProfile(input);
      } else {
        await api.updateProfile(current.id, input);
      }

      draft = null;
      app.notify('profile saved and applied');
      await refresh();
    } catch (error) {
      app.fail(error);
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (removing === null) {
      return;
    }

    busy = true;

    try {
      await api.deleteProfile(removing.id);
      removing = null;
      app.notify('profile removed');
      await refresh();
    } catch (error) {
      app.fail(error);
    } finally {
      busy = false;
    }
  }
</script>

<div class="stack">
  <div class="row spread wrap">
    <div>
      <h1>Profiles</h1>
      <p class="muted small">
        A profile says what to copy. Every replica follows one; the default covers the rest.
      </p>
    </div>

    {#if app.admin}
      <button class="primary" onclick={() => (draft = blank())}>New profile</button>
    {/if}
  </div>

  <section class="card flush">
    {#if loading}
      <p class="empty">Loading…</p>
    {:else}
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Syncs</th>
            <th>Gravity</th>
            <th>Used by</th>
            {#if app.admin}<th></th>{/if}
          </tr>
        </thead>
        <tbody>
          {#each profiles as profile (profile.id)}
            <tr>
              <td>
                {profile.name}
                {#if profile.default}<span class="pill">default</span>{/if}
              </td>
              <td class="muted small">{summary(profile)}</td>
              <td>{profile.runGravity ? 'yes' : 'no'}</td>
              <td class="muted">
                {profile.inUse}
                {profile.inUse === 1 ? 'replica' : 'replicas'}
              </td>
              {#if app.admin}
                <td class="row">
                  <button class="small" onclick={() => edit(profile)}>Edit</button>
                  <button
                    class="small danger"
                    disabled={profile.default || profile.inUse > 0}
                    title={profile.default
                      ? 'the default profile cannot be removed'
                      : profile.inUse > 0
                        ? 'move its replicas to another profile first'
                        : ''}
                    onclick={() => (removing = profile)}
                  >
                    Delete
                  </button>
                </td>
              {/if}
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>
</div>

{#if draft !== null}
  <Modal
    title={draft.id === null ? 'New profile' : `Edit ${draft.name}`}
    {busy}
    onclose={() => (draft = null)}
    onconfirm={save}
  >
    <div>
      <label for="profile-name">Name</label>
      <input
        id="profile-name"
        bind:value={draft.name}
        required
        disabled={draft.isDefault}
        placeholder="dns-only"
      />
      {#if draft.isDefault}
        <p class="muted small">The default profile keeps its name.</p>
      {/if}
    </div>

    <fieldset class="stack tight">
      <label class="check">
        <input type="checkbox" bind:checked={draft.fullSync} />
        <span>Sync every section (full sync)</span>
      </label>

      <label class="check">
        <input type="checkbox" bind:checked={draft.runGravity} />
        <span>Run gravity after syncing</span>
      </label>
    </fieldset>

    {#if !draft.fullSync}
      <fieldset>
        <p class="label">Sections</p>
        <div class="sections">
          {#each sections as section (section.name)}
            {@const setting = draft.sections[section.name]}
            <div class="section" class:off={!section.syncable}>
              <label class="check">
                <input
                  type="checkbox"
                  bind:checked={setting.enabled}
                  disabled={!section.syncable}
                />
                <span class="mono">{section.name}</span>
              </label>

              {#if !section.syncable}
                <p class="muted small">{section.reason}</p>
              {:else if setting.enabled}
                <div class="row wrap filter">
                  <select bind:value={setting.filter} aria-label="{section.name} filter">
                    {#each filters as mode (mode)}
                      <option value={mode}>{mode}</option>
                    {/each}
                  </select>

                  {#if setting.filter !== 'none'}
                    <input
                      class="mono"
                      value={keysOf(section.name)}
                      oninput={(event) => setKeys(section.name, event.currentTarget.value)}
                      placeholder="upstreams, blockESNI"
                      aria-label="{section.name} keys"
                    />
                  {/if}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      </fieldset>
    {/if}

    {#if draft.runGravity}
      <fieldset>
        <p class="label">Gravity tables</p>
        <div class="tables">
          {#each tables as table (table)}
            <label class="check">
              <input type="checkbox" bind:checked={draft.gravity[table]} />
              <span class="mono">{table}</span>
            </label>
          {/each}
        </div>
      </fieldset>
    {/if}
  </Modal>
{/if}

{#if removing !== null}
  <Modal
    title="Delete profile"
    {busy}
    danger
    confirmLabel="Delete"
    onclose={() => (removing = null)}
    onconfirm={remove}
  >
    <p>Remove the profile <strong>{removing.name}</strong>?</p>
  </Modal>
{/if}

<style>
  .tight {
    gap: 0.4rem;
  }

  .sections {
    display: grid;
    gap: 0.5rem;
  }

  .section {
    padding: 0.5rem 0.65rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface-2);
  }

  .section.off {
    opacity: 0.65;
  }

  .filter {
    margin-top: 0.4rem;
  }

  .filter select {
    width: auto;
  }

  .filter input {
    flex: 1 1 220px;
  }

  .tables {
    display: grid;
    gap: 0.35rem;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  }
</style>
