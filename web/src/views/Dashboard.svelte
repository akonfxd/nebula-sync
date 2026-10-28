<script lang="ts">
  import { api } from '../lib/api';
  import { app } from '../lib/app.svelte';
  import { ago, at, duration } from '../lib/format';
  import type { Run, Status } from '../lib/types';

  const idlePoll = 10_000;
  const busyPoll = 2000;

  let status = $state<Status | null>(null);
  let runs = $state<Run[]>([]);
  let cron = $state('');
  let editingCron = $state(false);
  let busy = $state(false);
  let expanded = $state<number | null>(null);

  async function refresh() {
    try {
      const [next, history] = await Promise.all([api.status(), api.runs()]);

      status = next;
      runs = history;

      if (!editingCron) {
        cron = next.schedule.cron;
      }
    } catch (error) {
      app.fail(error);
    }
  }

  /**
   * A sync answers 202 and keeps running, so the only way to learn how it went
   * is to keep asking. The poll tightens while one is in flight.
   */
  $effect(() => {
    const syncing = status?.syncing ?? false;
    const timer = setInterval(refresh, syncing ? busyPoll : idlePoll);

    return () => clearInterval(timer);
  });

  void refresh();

  async function syncNow() {
    busy = true;

    try {
      await api.sync();
      app.notify('sync started');
      await refresh();
    } catch (error) {
      app.fail(error);
    } finally {
      busy = false;
    }
  }

  async function saveCron() {
    busy = true;

    try {
      const saved = await api.setSchedule(cron.trim());

      cron = saved.cron;
      editingCron = false;
      app.notify(saved.cron === '' ? 'schedule cleared' : 'schedule saved');
      await refresh();
    } catch (error) {
      app.fail(error);
    } finally {
      busy = false;
    }
  }
</script>

<div class="stack">
  <section class="grid">
    <div class="card">
      <p class="label">State</p>
      <p class="value">
        {#if status?.syncing}
          <span class="pill">syncing…</span>
        {:else if status?.latest}
          <span class="pill {status.latest.success ? 'ok' : 'bad'}">
            {status.latest.success ? 'in sync' : 'failed'}
          </span>
        {:else}
          <span class="pill">no sync yet</span>
        {/if}
      </p>
    </div>

    <div class="card">
      <p class="label">Last sync</p>
      <p class="value">{ago(status?.latest?.timestamp)}</p>
      {#if status?.latest}
        <p class="muted small">
          {status.latest.mode} · {duration(status.latest.durationMs)}
        </p>
      {/if}
    </div>

    <div class="card">
      <p class="label">Next run</p>
      <p class="value">
        {status?.schedule.nextRun ? ago(status.schedule.nextRun) : 'not scheduled'}
      </p>
      {#if status?.schedule.cron}
        <p class="muted small mono">{status.schedule.cron}</p>
      {/if}
    </div>

    <div class="card actions">
      <button class="primary" onclick={syncNow} disabled={!app.admin || busy || status?.syncing}>
        Sync now
      </button>
      {#if !app.admin}
        <p class="muted small">Viewers cannot start a sync.</p>
      {/if}
    </div>
  </section>

  {#if app.admin}
    <section class="card">
      <div class="row spread wrap">
        <div>
          <h2>Schedule</h2>
          <p class="muted small">
            A cron expression, or empty to sync only on start and on request.
          </p>
        </div>

        {#if !editingCron}
          <div class="row">
            <span class="mono">{status?.schedule.cron || 'none'}</span>
            <button class="small" onclick={() => (editingCron = true)}>Change</button>
          </div>
        {/if}
      </div>

      {#if editingCron}
        <form
          class="row wrap edit"
          onsubmit={(event) => {
            event.preventDefault();
            void saveCron();
          }}
        >
          <input class="mono" bind:value={cron} placeholder="0 * * * *" aria-label="Cron" />
          <button type="submit" class="primary" disabled={busy}>Save</button>
          <button
            type="button"
            disabled={busy}
            onclick={() => {
              editingCron = false;
              cron = status?.schedule.cron ?? '';
            }}
          >
            Cancel
          </button>
        </form>
      {/if}
    </section>
  {/if}

  {#if status?.latest}
    <section class="card flush">
      <header><h2>Replicas</h2></header>

      {#if status.latest.replicas.length === 0}
        <p class="empty">
          {status.latest.error || 'The last sync touched no replicas.'}
        </p>
      {:else}
        <table>
          <thead>
            <tr>
              <th>Target</th>
              <th>Profile</th>
              <th>Mode</th>
              <th>Took</th>
              <th>Result</th>
            </tr>
          </thead>
          <tbody>
            {#each status.latest.replicas as replica (replica.target)}
              <tr>
                <td class="mono">{replica.target}</td>
                <td>{replica.profile}</td>
                <td>{replica.mode}</td>
                <td class="muted">{duration(replica.durationMs)}</td>
                <td>
                  {#if replica.error}
                    <span class="pill bad" title={replica.error}>failed</span>
                  {:else}
                    <span class="pill ok">ok</span>
                  {/if}
                </td>
              </tr>
              {#if replica.error}
                <tr>
                  <td colspan="5" class="small"><span class="error">{replica.error}</span></td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      {/if}
    </section>
  {/if}

  <section class="card flush">
    <header><h2>History</h2></header>

    {#if runs.length === 0}
      <p class="empty">Nothing has run yet.</p>
    {:else}
      <table>
        <thead>
          <tr>
            <th>Started</th>
            <th>Trigger</th>
            <th>Mode</th>
            <th>Took</th>
            <th>Result</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each runs as run (run.id)}
            <tr>
              <td title={at(run.startedAt)}>{ago(run.startedAt)}</td>
              <td>{run.source}</td>
              <td>{run.mode || '—'}</td>
              <td class="muted">{duration(run.durationMs)}</td>
              <td>
                <span class="pill {run.success ? 'ok' : 'bad'}">
                  {run.success ? 'ok' : 'failed'}
                </span>
              </td>
              <td>
                {#if run.replicas.length > 0 || run.error}
                  <button
                    class="link small"
                    onclick={() => (expanded = expanded === run.id ? null : run.id)}
                  >
                    {expanded === run.id ? 'hide' : 'details'}
                  </button>
                {/if}
              </td>
            </tr>

            {#if expanded === run.id}
              <tr>
                <td colspan="6">
                  {#if run.error}
                    <p class="error">{run.error}</p>
                  {/if}
                  <ul class="detail small">
                    {#each run.replicas as replica (replica.target)}
                      <li>
                        <span class="mono">{replica.target}</span>
                        <span class="muted">
                          {replica.profile} · {replica.mode} · {duration(replica.durationMs)}
                        </span>
                        {#if replica.error}
                          <span class="pill bad">{replica.error}</span>
                        {/if}
                      </li>
                    {/each}
                  </ul>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    {/if}
  </section>
</div>

<style>
  .value {
    margin: 0;
    font-size: 1.1rem;
  }

  .actions {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    justify-content: center;
  }

  .edit {
    margin-top: 0.75rem;
  }

  .edit input {
    flex: 1 1 200px;
  }

  .detail {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    margin: 0.4rem 0 0;
    padding-left: 1rem;
  }

  .detail li {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: center;
  }
</style>
