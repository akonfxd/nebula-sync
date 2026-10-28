<script lang="ts">
  import { api } from '../lib/api';
  import { app } from '../lib/app.svelte';
  import { at } from '../lib/format';

  let currentPassword = $state('');
  let newPassword = $state('');
  let repeated = $state('');
  let busy = $state(false);
  let problem = $state('');

  const minLength = $derived(app.schema?.passwordMinLength ?? 10);

  async function submit(event: SubmitEvent) {
    event.preventDefault();

    if (newPassword !== repeated) {
      problem = 'the two new passwords do not match';

      return;
    }

    busy = true;
    problem = '';

    try {
      await api.changeOwnPassword(currentPassword, newPassword);
      currentPassword = '';
      newPassword = '';
      repeated = '';
      app.notify('password changed, other sessions signed out');
    } catch (error) {
      problem = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }
</script>

<div class="stack">
  <h1>Account</h1>

  <section class="card">
    <h2>{app.user?.username}</h2>
    <p class="muted small">
      Role <span class="pill">{app.user?.role}</span> · signed up {at(app.user?.createdAt)}
    </p>
  </section>

  <section class="card">
    <form class="stack" onsubmit={submit}>
      <div>
        <h2>Change password</h2>
        <p class="muted small">
          Every session you hold ends, including this one, which is then signed back in.
        </p>
      </div>

      {#if problem}
        <p class="error">{problem}</p>
      {/if}

      <div>
        <label for="current">Current password</label>
        <input
          id="current"
          type="password"
          bind:value={currentPassword}
          autocomplete="current-password"
          required
        />
      </div>

      <div class="grid">
        <div>
          <label for="next">New password</label>
          <input
            id="next"
            type="password"
            bind:value={newPassword}
            autocomplete="new-password"
            minlength={minLength}
            required
          />
        </div>

        <div>
          <label for="repeat">Repeat</label>
          <input
            id="repeat"
            type="password"
            bind:value={repeated}
            autocomplete="new-password"
            minlength={minLength}
            required
          />
        </div>
      </div>

      <div>
        <button type="submit" class="primary" disabled={busy}>
          {busy ? 'Working…' : 'Change password'}
        </button>
      </div>
    </form>
  </section>
</div>
