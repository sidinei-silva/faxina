<script lang="ts">
  import { bytes, statusHelp, statusLabel } from './format'

  type Totals = Record<string, number>
  let {
    totals,
    selected,
    freeBytes,
    filter,
    onfilter,
  }: {
    totals: Totals
    selected: Totals
    freeBytes: number
    filter: string
    onfilter: (status: string) => void
  } = $props()

  const order = ['removable', 'candidate', 'in_use', 'protected']
  const scale = $derived(order.reduce((sum, s) => sum + (totals[s] ?? 0), 0) + freeBytes || 1)
  const pct = (n: number) => `${(n / scale) * 100}%`
</script>

<div class="strip" role="group" aria-label="Espaço medido por status">
  {#each order as status}
    {#if (totals[status] ?? 0) > 0}
      <button
        class="segment {status}"
        class:dim={filter !== 'all' && filter !== status}
        style:width={pct(totals[status])}
        title={`${statusLabel[status]}: ${bytes(totals[status])}`}
        aria-label={`${statusLabel[status]}: ${bytes(totals[status])}`}
        aria-pressed={filter === status}
        onclick={() => onfilter(filter === status ? 'all' : status)}
      >
        <span
          class="marked"
          style:width={`${Math.min(100, ((selected[status] ?? 0) / totals[status]) * 100)}%`}
        ></span>
      </button>
    {/if}
  {/each}
  <div class="free" style:width={pct(freeBytes)} title={`Livre: ${bytes(freeBytes)}`}></div>
</div>

<ul class="legend">
  {#each order as status}
    <li>
      <button
        class="key {status}"
        class:active={filter === status}
        aria-pressed={filter === status}
        onclick={() => onfilter(filter === status ? 'all' : status)}
      >
        <span class="name"><i class="dot"></i>{statusLabel[status]}</span>
        <strong>{bytes(totals[status] ?? 0)}</strong>
        <small>{statusHelp[status]}</small>
      </button>
    </li>
  {/each}
</ul>

<style>
  .strip {
    display: flex;
    height: 76px;
    gap: 2px;
    background: var(--strip-bg);
    border-radius: 3px;
    overflow: hidden;
  }
  .segment {
    position: relative;
    min-width: 6px;
    padding: 0;
    border: 0;
    background: var(--c);
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .segment:focus-visible {
    outline-offset: -4px;
    outline-color: var(--paper);
  }
  .dim {
    opacity: 0.35;
  }
  .marked {
    position: absolute;
    inset: 0 auto 0 0;
    transition: width 0.25s ease-out;
    background: repeating-linear-gradient(
      -45deg,
      var(--paper) 0 3px,
      transparent 3px 11px
    );
    opacity: 0.75;
  }
  .free {
    min-width: 4px;
  }
  .removable { --c: var(--removable); }
  .candidate { --c: var(--candidate); }
  .in_use { --c: var(--in-use); }
  .protected { --c: var(--protected); }

  .legend {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 8px 22px;
    margin: 12px 0 0;
    padding: 0;
    list-style: none;
  }
  .key {
    display: block;
    width: calc(100% + 8px);
    padding: 6px 8px 8px;
    margin-left: -8px;
    border: 0;
    border-radius: 4px;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .key:hover { background: var(--hover); }
  .key.active { background: var(--selected-row); }
  .name {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 0.9rem;
  }
  .dot {
    width: 10px;
    height: 10px;
    border-radius: 2px;
    background: var(--c);
  }
  strong {
    display: block;
    font-family: var(--display);
    font-size: 1.45rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    letter-spacing: -0.01em;
  }
  small {
    display: block;
    color: var(--muted);
    font-size: 0.8rem;
    line-height: 1.35;
  }
  @media (max-width: 900px) {
    .legend { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
</style>
