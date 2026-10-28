<script lang="ts">
  import { api } from '../lib/api';
  import { app } from '../lib/app.svelte';

  let username = $state('');
  let password = $state('');
  let busy = $state(false);
  let problem = $state('');

  const bootstrapping = $derived(!app.initialized);

  async function submit(event: SubmitEvent) {
    event.preventDefault();

    busy = true;
    problem = '';

    try {
      if (bootstrapping) {
        await api.bootstrap(username, password);
      } else {
        await api.login(username, password);
      }

      await app.adopt();
    } catch (error) {
      problem = error instanceof Error ? error.message : String(error);
      password = '';
    } finally {
      busy = false;
    }
  }
</script>

<main>
  <form class="card stack" onsubmit={submit}>
    <div>
      <h1>nebula-sync</h1>
      <p class="muted small">
        {#if bootstrapping}
          No account exists yet. The first one you create is an administrator.
        {:else}
          Sign in to manage your Pi-hole instances.
        {/if}
      </p>
    </div>

    {#if problem}
      <p class="error">{problem}</p>
    {/if}

    <div>
      <label for="username">Username</label>
      <input id="username" bind:value={username} autocomplete="username" required />
    </div>

    <div>
      <label for="password">Password</label>
      <input
        id="password"
        type="password"
        bind:value={password}
        autocomplete={bootstrapping ? 'new-password' : 'current-password'}
        required
      />
      {#if bootstrapping}
        <p class="muted small">At least 10 characters.</p>
      {/if}
    </div>

    <button type="submit" class="primary" disabled={busy}>
      {busy ? 'Working…' : bootstrapping ? 'Create administrator' : 'Sign in'}
    </button>
  </form>
</main>

<style>
  main {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    padding: 1rem;
  }

  form {
    width: min(380px, 100%);
  }
</style>
