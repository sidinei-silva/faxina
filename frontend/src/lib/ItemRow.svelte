<script lang="ts">
  import { age, bytes, statusLabel } from './format'

  let {
    item,
    checked,
    scale,
    ontoggle,
  }: { item: any; checked: boolean; scale: number; ontoggle: () => void } = $props()

  let open = $state(false)
  let copied = $state(false)
  const selectable = $derived(
    (item.status === 'removable' || item.status === 'candidate') && item.action?.kind !== 'none'
  )
  const when = $derived(age(item.ageDays))

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      copied = true
      setTimeout(() => (copied = false), 1500)
    } catch {
      copied = false
    }
  }
</script>

<li class="row" class:on={checked}>
  <div class="line">
    <label class="check" title={selectable ? 'Incluir na limpeza' : 'Este item não pode ser limpo pelo app'}>
      <input
        type="checkbox"
        {checked}
        disabled={!selectable}
        aria-label={`Incluir ${item.name} na limpeza`}
        onchange={ontoggle}
      />
    </label>
    <button class="main" aria-expanded={open} onclick={() => (open = !open)}>
      <span class="name" title={item.name}>{item.name}</span>
      <span class="status {item.status}"><i></i>{statusLabel[item.status]}</span>
      <span class="age">{when}</span>
      <span class="size">
        {item.sizeBytes > 0 ? bytes(item.sizeBytes) : '—'}
        <span class="bar"><b style:width={`${Math.max(1, (item.sizeBytes / scale) * 100)}%`}></b></span>
      </span>
    </button>
  </div>
  {#if open}
    <div class="more">
      <p>{item.reason}</p>
      {#if item.detail}<p class="muted break">{item.detail}</p>{/if}
      {#if item.path}<p class="muted break">{item.path}</p>{/if}
      {#if item.action?.command}
        <p class="cmd"><span>Comando da limpeza</span><code>{item.action.command}</code></p>
      {/if}
      {#if item.manualHint}
        <div class="cmd">
          <span>Faça você mesmo no terminal</span>
          <code>{item.manualHint}</code>
          <button class="copy" onclick={() => copy(item.manualHint)}>{copied ? 'Copiado' : 'Copiar comando'}</button>
        </div>
      {/if}
    </div>
  {/if}
</li>

<style>
  .row { border-top: 1px solid var(--line); }
  .row.on { background: var(--selected-row); }
  .line { display: flex; align-items: stretch; }
  .check {
    display: flex;
    align-items: center;
    padding: 0 10px 0 12px;
    cursor: pointer;
  }
  input { width: 16px; height: 16px; accent-color: var(--removable); cursor: inherit; }
  input:disabled { opacity: 0.3; }
  .main {
    flex: 1;
    min-width: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr) 104px 86px 130px;
    gap: 12px;
    align-items: center;
    padding: 7px 12px 7px 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    font-size: 0.92rem;
    text-align: left;
    cursor: pointer;
  }
  .main:hover .name { text-decoration: underline; text-underline-offset: 2px; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .status { display: flex; align-items: center; gap: 6px; font-size: 0.82rem; }
  .status i { width: 8px; height: 8px; border-radius: 2px; background: var(--c); }
  .removable { --c: var(--removable); }
  .candidate { --c: var(--candidate); }
  .in_use { --c: var(--in-use); }
  .protected { --c: var(--protected); }
  .age { color: var(--muted); font-size: 0.82rem; }
  .size { text-align: right; font-variant-numeric: tabular-nums; }
  .bar { display: block; height: 3px; margin-top: 3px; background: var(--hover); }
  .bar b { display: block; height: 100%; margin-left: auto; background: var(--ink); opacity: 0.55; }
  .more { padding: 2px 16px 12px 38px; font-size: 0.9rem; max-width: 780px; }
  .more p { margin: 6px 0; line-height: 1.45; }
  .muted { color: var(--muted); }
  .break { overflow-wrap: anywhere; }
  .cmd { margin: 6px 0; }
  .cmd span { display: block; color: var(--muted); font-size: 0.8rem; }
  code {
    display: block;
    margin-top: 2px;
    padding: 5px 8px;
    border-radius: 3px;
    background: var(--hover);
    font-size: 0.82rem;
    overflow-wrap: anywhere;
  }
  .copy {
    margin-top: 6px;
    padding: 3px 9px;
    border: 1px solid var(--line-strong);
    border-radius: 3px;
    background: none;
    color: inherit;
    font: inherit;
    font-size: 0.82rem;
    cursor: pointer;
  }
  @media (max-width: 900px) {
    .main { grid-template-columns: minmax(0, 1fr) 96px 110px; }
    .age { display: none; }
  }
</style>
