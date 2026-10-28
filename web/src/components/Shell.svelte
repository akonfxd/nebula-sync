<script lang="ts">
  import type { Snippet } from 'svelte';

  import { app, type Page } from '../lib/app.svelte';

  let { children }: { children: Snippet } = $props();

  const tabs: { page: Page; label: string; adminOnly?: boolean }[] = [
    { page: 'dashboard', label: 'Dashboard' },
    { page: 'instances', label: 'Instances' },
    { page: 'profiles', label: 'Profiles' },
    { page: 'users', label: 'Users', adminOnly: true },
    { page: 'account', label: 'Account' },
  ];

  const visible = $derived(tabs.filter((tab) => !tab.adminOnly || app.admin));
</script>

<header>
  <div class="bar">
    <div class="row">
      <strong>nebula-sync</strong>
      <span class="muted small mono">{app.version}</span>
    </div>

    <div class="row">
      <span class="muted small">
        {app.user?.username}
        <span class="pill">{app.user?.role}</span>
      </span>
      <button class="small" onclick={() => app.signOut()}>Sign out</button>
    </div>
  </div>

  <nav>
    {#each visible as tab (tab.page)}
      <button
        class="tab"
        class:active={app.page === tab.page}
        onclick={() => app.go(tab.page)}
        aria-current={app.page === tab.page ? 'page' : undefined}
      >
        {tab.label}
      </button>
    {/each}
  </nav>
</header>

<main>
  {@render children()}
</main>

<style>
  header {
    position: sticky;
    top: 0;
    z-index: 5;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }

  .bar,
  nav {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    width: min(1040px, 100%);
    margin: 0 auto;
    padding: 0 1rem;
  }

  .bar {
    justify-content: space-between;
    height: 48px;
  }

  nav {
    overflow-x: auto;
  }

  .tab {
    padding: 0.5rem 0.2rem;
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    margin-right: 1rem;
    background: none;
    color: var(--muted);
    white-space: nowrap;
  }

  .tab:hover:not(:disabled) {
    background: none;
    color: var(--text);
  }

  .tab.active {
    border-bottom-color: var(--accent);
    color: var(--text);
    font-weight: 500;
  }

  main {
    width: min(1040px, 100%);
    margin: 0 auto;
    padding: 1.25rem 1rem 3rem;
  }
</style>
