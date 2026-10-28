<script lang="ts">
  import { app } from '../lib/app.svelte';
</script>

<div class="toasts" role="status" aria-live="polite">
  {#each app.toasts as toast (toast.id)}
    <div class="toast {toast.kind}">
      <span>{toast.message}</span>
      <button class="link" onclick={() => app.dismiss(toast.id)} aria-label="Dismiss">×</button>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    right: 1rem;
    bottom: 1rem;
    z-index: 20;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    max-width: min(420px, calc(100vw - 2rem));
  }

  .toast {
    display: flex;
    gap: 0.75rem;
    align-items: start;
    justify-content: space-between;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow);
    font-size: 0.9rem;
  }

  .toast.ok {
    border-left: 3px solid var(--ok);
  }

  .toast.error {
    border-left: 3px solid var(--bad);
  }

  .toast button {
    font-size: 1.1rem;
    line-height: 1;
    text-decoration: none;
  }
</style>
