<script lang="ts">
  import { bytes, plural } from './format'

  let {
    plan,
    onconfirm,
    onclose,
  }: { plan: any; onconfirm: () => void; onclose: () => void } = $props()

  let reviewed = $state(false)
  let dialog: HTMLDialogElement
  const accepted = $derived(plan.entries.filter((e: any) => !e.refused))
  const refused = $derived(plan.entries.filter((e: any) => e.refused))
  const candidates = $derived(accepted.filter((e: any) => e.status === 'candidate'))
  const blocked = $derived(accepted.length === 0 || (plan.hasCandidate && !reviewed))

  $effect(() => {
    dialog.showModal()
  })
</script>

<dialog bind:this={dialog} oncancel={onclose} aria-labelledby="review-title">
  <h2 id="review-title">Revise antes de limpar</h2>
  <p class="lead">
    {plural(accepted.length, 'item será apagado', 'itens serão apagados')}, cerca de
    {bytes(plan.totalBytes)}. Não há como desfazer.
  </p>

  {#if plan.needsRoot}
    <p class="note">O sistema vai pedir sua senha para limpar os pacotes do pacman.</p>
  {/if}

  {#if refused.length}
    <h3>Recusados pela verificação ({refused.length})</h3>
    <ul class="list">
      {#each refused as entry}
        <li><span class="name">{entry.name}</span><span class="why">{entry.refused}</span></li>
      {/each}
    </ul>
  {/if}

  {#if candidates.length}
    <h3>Candidatos que você marcou ({candidates.length})</h3>
    <ul class="list">
      {#each candidates as entry}
        <li><span class="name">{entry.name}</span><span class="size">{bytes(entry.sizeBytes)}</span></li>
      {/each}
    </ul>
  {/if}

  <h3>Tudo o que será executado</h3>
  <ul class="list commands">
    {#each accepted as entry}
      <li><code>{entry.command}</code><span class="size">{entry.sizeBytes > 0 ? bytes(entry.sizeBytes) : '—'}</span></li>
    {/each}
  </ul>

  {#if plan.hasCandidate}
    <label class="ack">
      <input type="checkbox" bind:checked={reviewed} />
      Conferi os candidatos e quero apagá-los.
    </label>
  {/if}

  <div class="actions">
    <button class="ghost" onclick={onclose}>Voltar</button>
    <button class="danger" disabled={blocked} onclick={onconfirm}>Apagar {bytes(plan.totalBytes)}</button>
  </div>
</dialog>

<style>
  dialog {
    width: min(760px, calc(100vw - 48px));
    max-height: calc(100vh - 64px);
    padding: 26px 28px 22px;
    border: 0;
    border-radius: 6px;
    background: var(--paper);
    color: var(--ink);
    box-shadow: 0 24px 70px var(--shadow);
  }
  dialog::backdrop { background: var(--scrim); }
  h2 { margin: 0; font-family: var(--display); font-size: 1.7rem; font-weight: 650; letter-spacing: -0.015em; }
  h3 { margin: 18px 0 6px; font-size: 0.95rem; }
  .lead { margin: 6px 0 0; }
  .note { margin: 12px 0 0; padding: 8px 12px; border-left: 3px solid var(--candidate); background: var(--hover); }
  .list {
    max-height: 170px;
    margin: 0;
    padding: 0;
    overflow: auto;
    list-style: none;
    border-top: 1px solid var(--line);
    font-size: 0.87rem;
  }
  .commands { max-height: 230px; }
  li {
    display: flex;
    justify-content: space-between;
    gap: 16px;
    padding: 5px 2px;
    border-bottom: 1px solid var(--line);
  }
  .name, code { min-width: 0; overflow-wrap: anywhere; }
  code { font-size: 0.8rem; }
  .size { flex: none; font-variant-numeric: tabular-nums; }
  .why { color: var(--muted); text-align: right; }
  .ack { display: flex; gap: 8px; align-items: center; margin-top: 16px; cursor: pointer; }
  .ack input { width: 16px; height: 16px; accent-color: var(--candidate); }
  .actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 20px; }
</style>
