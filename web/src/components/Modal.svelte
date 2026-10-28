<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    title: string;
    busy?: boolean;
    confirmLabel?: string;
    danger?: boolean;
    onclose: () => void;
    onconfirm: () => void;
    children: Snippet;
  }

  let {
    title,
    busy = false,
    confirmLabel = 'Save',
    danger = false,
    onclose,
    onconfirm,
    children,
  }: Props = $props();

  function onkeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      onclose();
    }
  }
</script>

<svelte:window {onkeydown} />

<div class="backdrop">
  <div class="sheet card" role="dialog" aria-modal="true" aria-label={title}>
    <h2>{title}</h2>

    <form
      class="stack"
      onsubmit={(event) => {
        event.preventDefault();
        onconfirm();
      }}
    >
      {@render children()}

      <div class="row spread">
        <button type="button" onclick={onclose} disabled={busy}>Cancel</button>
        <button type="submit" class="primary" class:danger disabled={busy}>
          {busy ? 'Working…' : confirmLabel}
        </button>
      </div>
    </form>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 10;
    display: flex;
    align-items: start;
    justify-content: center;
    padding: 2rem 1rem;
    overflow-y: auto;
    background: rgb(0 0 0 / 45%);
  }

  .sheet {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    width: min(620px, 100%);
  }
</style>
