<script lang="ts">
  import { onMount } from 'svelte'
  import { Cancel, Execute, Preview, ReadDisk, Scan } from '../wailsjs/go/main/App'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import DiskStrip from './lib/DiskStrip.svelte'
  import ItemRow from './lib/ItemRow.svelte'
  import ReviewDialog from './lib/ReviewDialog.svelte'
  import ResultView from './lib/ResultView.svelte'
  import { bytes, groupLabel, plural } from './lib/format'

  type View = 'scanning' | 'list' | 'cleaning' | 'result'
  let view = $state<View>('scanning')
  let stage = $state('Preparando')
  let result = $state<any>(null)
  let selected = $state<Set<string>>(new Set())
  let filter = $state('all')
  let search = $state('')
  let openGroups = $state<Set<string>>(new Set())
  let plan = $state<any>(null)
  let reviewing = $state(false)
  let preparing = $state(false)
  let error = $state('')
  let cleanProgress = $state({ done: 0, total: 0, name: '' })
  let report = $state<any>(null)
  let freed = $state(0)

  const canSelect = (it: any) =>
    (it.status === 'removable' || it.status === 'candidate') && it.action?.kind !== 'none'

  const items = $derived<any[]>(result?.items ?? [])

  const totals = $derived.by(() => {
    const out: Record<string, number> = {}
    for (const it of items) out[it.status] = (out[it.status] ?? 0) + it.sizeBytes
    return out
  })
  const selectedByStatus = $derived.by(() => {
    const out: Record<string, number> = {}
    for (const it of items) {
      if (selected.has(it.id)) out[it.status] = (out[it.status] ?? 0) + it.sizeBytes
    }
    return out
  })
  const selectedBytes = $derived(Object.values(selectedByStatus).reduce((a, b) => a + b, 0))

  const visibleGroups = $derived.by(() => {
    const term = search.trim().toLowerCase()
    const groups = new Map<string, any[]>()
    for (const it of items) {
      if (filter !== 'all' && it.status !== filter) continue
      if (term && !`${it.name} ${it.path} ${it.detail}`.toLowerCase().includes(term)) continue
      if (!groups.has(it.group)) groups.set(it.group, [])
      groups.get(it.group)!.push(it)
    }
    return [...groups.entries()]
      .map(([id, list]) => ({
        id,
        list,
        size: list.reduce((sum, it) => sum + it.sizeBytes, 0),
        marked: list.filter((it) => selected.has(it.id)).length,
        selectable: list.filter(canSelect),
      }))
      .sort((a, b) => b.size - a.size)
  })
  const largest = $derived(Math.max(1, ...items.map((it) => it.sizeBytes)))

  async function runScan() {
    view = 'scanning'
    stage = 'Preparando'
    error = ''
    plan = null
    reviewing = false
    try {
      const scanned = await Scan()
      result = scanned
      selected = new Set(
        scanned.items.filter((it: any) => it.status === 'removable' && canSelect(it)).map((it: any) => it.id)
      )
      const reclaim = new Map<string, number>()
      for (const it of scanned.items) {
        if (it.status === 'removable') reclaim.set(it.group, (reclaim.get(it.group) ?? 0) + it.sizeBytes)
      }
      const biggest = [...reclaim.entries()].sort((a, b) => b[1] - a[1])[0]
      openGroups = new Set(biggest ? [biggest[0]] : [])
      filter = 'all'
      search = ''
    } catch (err) {
      error = `A varredura falhou: ${err}. Tente varrer de novo.`
    }
    view = 'list'
  }

  function toggle(id: string) {
    const next = new Set(selected)
    next.has(id) ? next.delete(id) : next.add(id)
    selected = next
  }

  function toggleGroupOpen(id: string) {
    const next = new Set(openGroups)
    next.has(id) ? next.delete(id) : next.add(id)
    openGroups = next
  }

  function selectRemovable(group: any, on: boolean) {
    const next = new Set(selected)
    for (const it of group.selectable) {
      if (it.status !== 'removable') continue
      on ? next.add(it.id) : next.delete(it.id)
    }
    selected = next
  }

  async function review() {
    if (preparing) return
    error = ''
    preparing = true
    try {
      plan = await Preview([...selected])
      reviewing = true
    } catch (err) {
      error = `Não foi possível preparar a limpeza: ${err}.`
    } finally {
      preparing = false
    }
  }

  async function confirm() {
    const id = plan.id
    reviewing = false
    cleanProgress = { done: 0, total: plan.accepted, name: '' }
    view = 'cleaning'
    try {
      report = await Execute(id)
      freed = report.freedBytes
      view = 'result'
    } catch (err) {
      error = `A limpeza não começou: ${err}. Revise de novo.`
      view = 'list'
    }
  }

  async function measure() {
    try {
      const disk = await ReadDisk()
      freed = disk.freeBytes - report.before.freeBytes
    } catch (err) {
      error = `Não foi possível medir o disco: ${err}.`
    }
  }

  onMount(() => {
    const offScan = EventsOn('scan:progress', (s: string) => (stage = s))
    const offClean = EventsOn('clean:progress', (p: any) => (cleanProgress = p))
    runScan()
    return () => {
      offScan()
      offClean()
    }
  })
</script>

<main>
  {#if view === 'scanning'}
    <section class="waiting" aria-live="polite">
      <h1>Varrendo o disco</h1>
      <div class="sweep"><span></span></div>
      <p class="stage">{stage}</p>
      <button class="ghost" onclick={Cancel}>Cancelar</button>
    </section>
  {:else if view === 'cleaning'}
    <section class="waiting" aria-live="polite">
      <h1>Limpando</h1>
      <progress max={cleanProgress.total || 1} value={cleanProgress.done}></progress>
      <p class="stage">
        {cleanProgress.done} de {cleanProgress.total}{cleanProgress.name ? `: ${cleanProgress.name}` : ''}
      </p>
      <button class="ghost" onclick={Cancel}>Parar depois do item atual</button>
    </section>
  {:else if view === 'result' && report}
    <ResultView {report} {freed} onmeasure={measure} onrescan={runScan} />
  {:else}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    {#if result}
      <header class="top">
        <div>
          <h1>
            {#if selectedBytes > 0}
              Dá para liberar cerca de {bytes(selectedBytes)}
            {:else}
              Nada selecionado para limpar
            {/if}
          </h1>
          <p class="sub">
            O disco está {Math.round(result.disk.usedPercent)}% cheio, com {bytes(result.disk.freeBytes)} livres.
            Os tamanhos são estimativas: a compressão do btrfs e as camadas compartilhadas do Docker mudam o valor real.
          </p>
        </div>
        <button class="ghost rescan" disabled={preparing} onclick={runScan}>Varrer de novo</button>
      </header>

      <DiskStrip
        {totals}
        selected={selectedByStatus}
        freeBytes={result.disk.freeBytes}
        {filter}
        onfilter={(s) => (filter = s)}
      />
      <p class="hint">A parte hachurada da faixa é o que está selecionado. O trecho vazio à direita é o espaço livre.</p>

      {#if result.warnings?.length}
        <details class="warnings">
          <summary>{plural(result.warnings.length, 'aviso da varredura', 'avisos da varredura')}</summary>
          <ul>
            {#each result.warnings as warning}<li>{warning}</li>{/each}
          </ul>
        </details>
      {/if}

      <div class="toolbar">
        <input type="search" placeholder="Buscar por nome ou caminho" aria-label="Buscar itens" bind:value={search} />
        {#if filter !== 'all'}
          <button class="link" onclick={() => (filter = 'all')}>Mostrar todos os status</button>
        {/if}
      </div>

      {#if visibleGroups.length === 0}
        <p class="empty">Nenhum item com este filtro. Limpe a busca ou mostre todos os status.</p>
      {/if}

      <div class="groups">
        {#each visibleGroups as group (group.id)}
          {@const open = openGroups.has(group.id)}
          {@const removable = group.selectable.filter((it) => it.status === 'removable')}
          {@const allRemovable = removable.length > 0 && removable.every((it) => selected.has(it.id))}
          <section class="group">
            <div class="group-head">
              <button class="group-toggle" aria-expanded={open} onclick={() => toggleGroupOpen(group.id)}>
                <span class="caret" class:open aria-hidden="true"></span>
                <h2>{groupLabel[group.id] ?? group.id}</h2>
                <span class="count">{plural(group.list.length, 'item', 'itens')}</span>
                {#if group.marked}<span class="marked">{group.marked} para limpar</span>{/if}
              </button>
              {#if removable.length}
                <button class="link" onclick={() => selectRemovable(group, !allRemovable)}>
                  {allRemovable ? 'Desmarcar removíveis' : 'Marcar removíveis'}
                </button>
              {/if}
              <span class="group-size">{bytes(group.size)}</span>
            </div>
            {#if open}
              <ul class="rows">
                {#each group.list as item (item.id)}
                  <ItemRow {item} scale={largest} checked={selected.has(item.id)} ontoggle={() => toggle(item.id)} />
                {/each}
              </ul>
            {/if}
          </section>
        {/each}
      </div>
    {:else}
      <section class="waiting">
        <h1>Nenhuma varredura disponível</h1>
        <button class="primary" onclick={runScan}>Varrer de novo</button>
      </section>
    {/if}
  {/if}
</main>

{#if view === 'list' && result}
  <footer class="dock">
    <span>
      <strong>{plural(selected.size, 'item', 'itens')}</strong> para limpar, cerca de
      <strong>{bytes(selectedBytes)}</strong>
    </span>
    {#if preparing}
      <span class="dock-actions">
        Conferindo os itens
        <button class="primary" onclick={Cancel}>Cancelar</button>
      </span>
    {:else}
      <button class="primary" disabled={selected.size === 0} onclick={review}>Revisar limpeza</button>
    {/if}
  </footer>
{/if}

{#if reviewing && plan}
  <ReviewDialog {plan} onconfirm={confirm} onclose={() => (reviewing = false)} />
{/if}

<style>
  main {
    max-width: 1120px;
    margin: 0 auto;
    padding: 28px 28px 96px;
  }
  .top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 20px;
  }
  h1 {
    margin: 0;
    font-family: var(--display);
    font-size: clamp(2.1rem, 4.6vw, 3.4rem);
    font-weight: 650;
    line-height: 1.04;
    letter-spacing: -0.025em;
    font-variant-numeric: tabular-nums;
  }
  .sub { max-width: 72ch; margin: 10px 0 0; color: var(--muted); line-height: 1.45; }
  .rescan { flex: none; }
  .hint { margin: 10px 0 0; color: var(--muted); font-size: 0.82rem; }
  .warnings { margin-top: 14px; font-size: 0.88rem; }
  .warnings summary { cursor: pointer; }
  .warnings ul { margin: 6px 0 0; padding-left: 20px; color: var(--muted); }
  .toolbar { display: flex; align-items: center; gap: 14px; margin: 22px 0 10px; }
  input[type='search'] {
    width: min(380px, 100%);
    padding: 7px 10px;
    border: 1px solid var(--line-strong);
    border-radius: 4px;
    background: var(--field);
    color: inherit;
    font: inherit;
    font-size: 0.92rem;
  }
  .empty { padding: 20px 0; color: var(--muted); }
  .group { border-top: 2px solid var(--ink); }
  .group:last-child { border-bottom: 1px solid var(--line); }
  .group-head { display: flex; align-items: center; gap: 14px; padding-right: 12px; }
  .group-toggle {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: baseline;
    gap: 12px;
    padding: 11px 0 11px 12px;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .group-toggle:hover h2 { text-decoration: underline; text-underline-offset: 3px; text-decoration-thickness: 1px; }
  .caret {
    align-self: center;
    width: 0;
    height: 0;
    border-left: 6px solid var(--ink);
    border-top: 4.5px solid transparent;
    border-bottom: 4.5px solid transparent;
    transition: transform 0.12s;
  }
  .caret.open { transform: rotate(90deg); }
  h2 { margin: 0; font-family: var(--display); font-size: 1.15rem; font-weight: 600; letter-spacing: -0.01em; }
  .count { color: var(--muted); font-size: 0.85rem; }
  .marked { color: var(--removable-text); font-size: 0.85rem; font-weight: 700; }
  .group-size {
    min-width: 92px;
    text-align: right;
    font-family: var(--display);
    font-size: 1.1rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .rows { margin: 0; padding: 0; list-style: none; }
  .dock {
    position: fixed;
    inset: auto 0 0 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 12px max(28px, calc((100vw - 1064px) / 2));
    background: var(--ink);
    color: var(--paper);
  }
  .dock :global(.primary) { background: var(--paper); color: var(--ink); }
  .dock-actions { display: flex; align-items: center; gap: 12px; }
  .waiting { display: flex; flex-direction: column; align-items: flex-start; gap: 16px; padding-top: 14vh; }
  .stage { min-height: 1.4em; margin: 0; color: var(--muted); overflow-wrap: anywhere; }
  .sweep { width: 100%; height: 76px; overflow: hidden; border-radius: 3px; background: var(--strip-bg); }
  .sweep span {
    display: block;
    width: 34%;
    height: 100%;
    background: repeating-linear-gradient(-45deg, var(--removable) 0 3px, transparent 3px 11px);
    animation: sweep 1.5s ease-in-out infinite alternate;
  }
  @keyframes sweep { to { transform: translateX(195%); } }
  progress { width: 100%; height: 14px; accent-color: var(--removable); }
  .error { margin: 0 0 16px; padding: 9px 12px; border-left: 3px solid var(--danger); background: var(--hover); }
</style>
