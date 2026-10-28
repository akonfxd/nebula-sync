<script lang="ts">
  import Login from './components/Login.svelte';
  import Shell from './components/Shell.svelte';
  import Toasts from './components/Toasts.svelte';
  import { app } from './lib/app.svelte';
  import Account from './views/Account.svelte';
  import Dashboard from './views/Dashboard.svelte';
  import Instances from './views/Instances.svelte';
  import Profiles from './views/Profiles.svelte';
  import Users from './views/Users.svelte';

  app.listen();
  void app.load();
</script>

{#if !app.ready}
  <p class="empty">Loading…</p>
{:else if app.user === null}
  <Login />
{:else}
  <Shell>
    {#if app.page === 'dashboard'}
      <Dashboard />
    {:else if app.page === 'instances'}
      <Instances />
    {:else if app.page === 'profiles'}
      <Profiles />
    {:else if app.page === 'users' && app.admin}
      <Users />
    {:else}
      <Account />
    {/if}
  </Shell>
{/if}

<Toasts />
