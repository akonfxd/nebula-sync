<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import { api } from '../lib/api';
  import { app } from '../lib/app.svelte';
  import type { Instance, InstanceInput, InstanceRole, Profile } from '../lib/types';

  interface Draft extends InstanceInput {
    id: number | null;
    hasPassword: boolean;
  }

  let instances = $state<Instance[]>([]);
  let profiles = $state<Profile[]>([]);
  let loading = $state(true);
  let busy = $state(false);
  let draft = $state<Draft | null>(null);
  let removing = $state<Instance | null>(null);

  const hasPrimary = $derived(instances.some((instance) => instance.role === 'primary'));

  async function refresh() {
    try {
      [instances, profiles] = await Promise.all([api.instances(), api.profiles()]);
    } catch (error) {
      app.fail(error);
    } finally {
      loading = false;
    }
  }

  void refresh();

  function profileName(id: number | null): string {
    if (id === null) {
      return 'default';
    }

    return profiles.find((profile) => profile.id === id)?.name ?? `#${id}`;
  }

  function add(role: InstanceRole) {
    draft = {
      id: null,
      role,
      name: '',
      url: '',
      password: '',
      enabled: true,
      profileId: null,
      position: instances.filter((instance) => instance.role === 'replica').length,
      hasPassword: false,
    };
  }

  function edit(instance: Instance) {
    draft = {
      id: instance.id,
      role: instance.role,
      name: instance.name,
      url: instance.url,
      password: '',
      enabled: instance.enabled,
      profileId: instance.profileId,
      position: instance.position,
      hasPassword: instance.hasPassword,
    };
  }

  /**
   * The api never sends a password back, so an empty box on an edit means "keep
   * what is stored". Sending an empty string instead would wipe the credential.
   */
  function payload(current: Draft): InstanceInput {
    const input: InstanceInput = {
      role: current.role,
      name: current.name.trim(),
      url: current.url.trim(),
      enabled: current.enabled,
      profileId: current.role === 'primary' ? null : current.profileId,
      position: current.position,
    };

    if (current.password !== '') {
      input.password = current.password;
    }

    return input;
  }

  async function save() {
    if (draft === null) {
      return;
    }

    const current = draft;
    busy = true;

    try {
      if (current.id === null) {
        await api.createInstance(payload(current));
      } else {
        await api.updateInstance(current.id, payload(current));
      }

      draft = null;
      app.notify('instance saved and applied');
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
      await api.deleteInstance(removing.id);
      removing = null;
      app.notify('instance removed');
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
      <h1>Instances</h1>
      <p class="muted small">
        The primary is read from, every enabled replica is written to.
      </p>
    </div>

    {#if app.admin}
      <div class="row">
        {#if !hasPrimary}
          <button class="primary" onclick={() => add('primary')}>Add primary</button>
        {/if}
        <button onclick={() => add('replica')} disabled={!hasPrimary}>Add replica</button>
      </div>
    {/if}
  </div>

  <section class="card flush">
    {#if loading}
      <p class="empty">Loading…</p>
    {:else if instances.length === 0}
      <p class="empty">No instances yet.</p>
    {:else}
      <table>
        <thead>
          <tr>
            <th>Role</th>
            <th>Name</th>
            <th>Address</th>
            <th>Profile</th>
            <th>State</th>
            {#if app.admin}<th></th>{/if}
          </tr>
        </thead>
        <tbody>
          {#each instances as instance (instance.id)}
            <tr>
              <td><span class="pill">{instance.role}</span></td>
              <td>{instance.name}</td>
              <td class="mono">{instance.url}</td>
              <td>
                {#if instance.role === 'primary'}
                  <span class="muted">—</span>
                {:else}
                  {profileName(instance.profileId)}
                {/if}
              </td>
              <td>
                {#if !instance.hasPassword}
                  <span class="pill bad">no password</span>
                {:else if instance.enabled}
                  <span class="pill ok">enabled</span>
                {:else}
                  <span class="pill">disabled</span>
                {/if}
              </td>
              {#if app.admin}
                <td class="row">
                  <button class="small" onclick={() => edit(instance)}>Edit</button>
                  <button class="small danger" onclick={() => (removing = instance)}>
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
    title={draft.id === null ? `Add ${draft.role}` : `Edit ${draft.name || draft.role}`}
    {busy}
    onclose={() => (draft = null)}
    onconfirm={save}
  >
    <div>
      <label for="url">Address</label>
      <input
        id="url"
        class="mono"
        bind:value={draft.url}
        placeholder="http://pihole.local"
        required
      />
      <p class="muted small">The Pi-hole web address, with scheme and port.</p>
    </div>

    <div>
      <label for="name">Name</label>
      <input id="name" bind:value={draft.name} placeholder="taken from the address" />
    </div>

    <div>
      <label for="password">Password</label>
      <input
        id="password"
        type="password"
        bind:value={draft.password}
        autocomplete="new-password"
        placeholder={draft.hasPassword ? 'leave empty to keep the stored password' : ''}
        required={!draft.hasPassword}
      />
      <p class="muted small">
        The Pi-hole api password. It is encrypted at rest and never sent back here.
      </p>
    </div>

    {#if draft.role === 'replica'}
      <div class="grid">
        <div>
          <label for="profile">Profile</label>
          <select id="profile" bind:value={draft.profileId}>
            <option value={null}>default</option>
            {#each profiles.filter((profile) => !profile.default) as profile (profile.id)}
              <option value={profile.id}>{profile.name}</option>
            {/each}
          </select>
        </div>

        <div>
          <label for="position">Order</label>
          <input id="position" type="number" min="0" bind:value={draft.position} />
        </div>
      </div>

      <label class="check">
        <input type="checkbox" bind:checked={draft.enabled} />
        <span>Sync to this replica</span>
      </label>
    {/if}
  </Modal>
{/if}

{#if removing !== null}
  <Modal
    title="Delete instance"
    {busy}
    danger
    confirmLabel="Delete"
    onclose={() => (removing = null)}
    onconfirm={remove}
  >
    <p>
      Remove <strong>{removing.name}</strong> (<span class="mono">{removing.url}</span>)? Its
      stored password goes with it.
    </p>
  </Modal>
{/if}
