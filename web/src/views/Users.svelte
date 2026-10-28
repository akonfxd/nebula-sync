<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import { api } from '../lib/api';
  import { app } from '../lib/app.svelte';
  import { ago } from '../lib/format';
  import type { Role, User } from '../lib/types';

  let users = $state<User[]>([]);
  let loading = $state(true);
  let busy = $state(false);
  let adding = $state(false);
  let resetting = $state<User | null>(null);
  let removing = $state<User | null>(null);

  let username = $state('');
  let password = $state('');
  let role = $state<Role>('viewer');

  const roles = $derived<Role[]>(app.schema?.userRoles ?? ['admin', 'viewer']);
  const minLength = $derived(app.schema?.passwordMinLength ?? 10);
  const admins = $derived(users.filter((user) => user.role === 'admin').length);

  async function refresh() {
    try {
      users = await api.users();
    } catch (error) {
      app.fail(error);
    } finally {
      loading = false;
    }
  }

  void refresh();

  function openAdd() {
    username = '';
    password = '';
    role = 'viewer';
    adding = true;
  }

  async function create() {
    busy = true;

    try {
      await api.createUser(username.trim(), password, role);
      adding = false;
      app.notify('account created');
      await refresh();
    } catch (error) {
      app.fail(error);
    } finally {
      busy = false;
    }
  }

  /** Changing someone's role signs them out, so a demotion takes effect at once. */
  async function changeRole(user: User, next: Role) {
    try {
      await api.updateUser(user.id, { role: next });
      app.notify(`${user.username} is now ${next}`);
      await refresh();
    } catch (error) {
      app.fail(error);
      await refresh();
    }
  }

  async function resetPassword() {
    if (resetting === null) {
      return;
    }

    busy = true;

    try {
      await api.updateUser(resetting.id, { password });
      app.notify(`password changed for ${resetting.username}`);
      resetting = null;
      password = '';
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
      await api.deleteUser(removing.id);
      removing = null;
      app.notify('account removed');
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
      <h1>Users</h1>
      <p class="muted small">
        Administrators change the configuration. Viewers only read it.
      </p>
    </div>

    <button class="primary" onclick={openAdd}>New account</button>
  </div>

  <section class="card flush">
    {#if loading}
      <p class="empty">Loading…</p>
    {:else}
      <table>
        <thead>
          <tr>
            <th>Username</th>
            <th>Role</th>
            <th>Last sign in</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each users as user (user.id)}
            <tr>
              <td>
                {user.username}
                {#if user.id === app.user?.id}<span class="pill">you</span>{/if}
              </td>
              <td>
                <select
                  class="role"
                  value={user.role}
                  disabled={user.role === 'admin' && admins === 1}
                  onchange={(event) => changeRole(user, event.currentTarget.value as Role)}
                  aria-label="{user.username} role"
                >
                  {#each roles as option (option)}
                    <option value={option}>{option}</option>
                  {/each}
                </select>
              </td>
              <td class="muted">{ago(user.lastLoginAt)}</td>
              <td class="row">
                <button
                  class="small"
                  onclick={() => {
                    password = '';
                    resetting = user;
                  }}
                >
                  Set password
                </button>
                <button
                  class="small danger"
                  disabled={user.id === app.user?.id}
                  title={user.id === app.user?.id ? 'you cannot delete your own account' : ''}
                  onclick={() => (removing = user)}
                >
                  Delete
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>
</div>

{#if adding}
  <Modal
    title="New account"
    {busy}
    confirmLabel="Create"
    onclose={() => (adding = false)}
    onconfirm={create}
  >
    <div>
      <label for="new-username">Username</label>
      <input id="new-username" bind:value={username} autocomplete="off" required />
    </div>

    <div>
      <label for="new-password">Password</label>
      <input
        id="new-password"
        type="password"
        bind:value={password}
        autocomplete="new-password"
        minlength={minLength}
        required
      />
      <p class="muted small">At least {minLength} characters.</p>
    </div>

    <div>
      <label for="new-role">Role</label>
      <select id="new-role" bind:value={role}>
        {#each roles as option (option)}
          <option value={option}>{option}</option>
        {/each}
      </select>
    </div>
  </Modal>
{/if}

{#if resetting !== null}
  <Modal
    title="Set password for {resetting.username}"
    {busy}
    confirmLabel="Set password"
    onclose={() => (resetting = null)}
    onconfirm={resetPassword}
  >
    <div>
      <label for="reset-password">New password</label>
      <input
        id="reset-password"
        type="password"
        bind:value={password}
        autocomplete="new-password"
        minlength={minLength}
        required
      />
      <p class="muted small">Their current sessions end immediately.</p>
    </div>
  </Modal>
{/if}

{#if removing !== null}
  <Modal
    title="Delete account"
    {busy}
    danger
    confirmLabel="Delete"
    onclose={() => (removing = null)}
    onconfirm={remove}
  >
    <p>Remove the account <strong>{removing.username}</strong>?</p>
  </Modal>
{/if}

<style>
  .role {
    width: auto;
    padding: 0.2rem 0.4rem;
    font-size: 0.85rem;
  }
</style>
