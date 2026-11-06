<script lang="ts">
  import { bytes, plural } from './format'

  let {
    report,
    freed,
    onmeasure,
    onrescan,
  }: { report: any; freed: number; onmeasure: () => void; onrescan: () => void } = $props()

  const ok = $derived(report.outcomes.filter((o: any) => o.ok))
  const problems = $derived(report.outcomes.filter((o: any) => !o.ok))
  const headline = $derived(
    freed > 0 ? `${bytes(freed)} liberados` : 'O espaço livre ainda não aumentou'
  )
</script>

<section class="result">
  <h1>{headline}</h1>
  <p class="compare">
    A estimativa era de {bytes(report.estimatedBytes)}. O valor real vem do espaço livre no disco, que agora é de
    {bytes(report.before.freeBytes + freed)}.
  </p>
  {#if report.note}<p class="note">{report.note}</p>{/if}
  {#if report.canceled}<p class="note">A limpeza foi interrompida. Os itens restantes continuam no disco.</p>{/if}

  <div class="actions">
    <button class="ghost" onclick={onmeasure}>Medir de novo</button>
    <button class="primary" onclick={onrescan}>Varrer de novo</button>
  </div>

  <p class="count">{plural(ok.length, 'item apagado', 'itens apagados')}</p>
  {#if problems.length}
    <h2>Não apagados ({problems.length})</h2>
    <ul>
      {#each problems as item}
        <li><strong>{item.name}</strong><span>{item.error}</span></li>
      {/each}
    </ul>
  {/if}
  {#if report.auditError}
    <p class="note">O registro da limpeza falhou e pode estar incompleto: {report.auditError}</p>
  {:else if report.auditPath}
    <p class="audit">Registro completo em <code>{report.auditPath}</code></p>
  {/if}
</section>

<style>
  .result { max-width: 780px; padding-top: 22px; }
  h1 {
    margin: 0;
    font-family: var(--display);
    font-size: clamp(2.6rem, 6vw, 4.2rem);
    font-weight: 650;
    line-height: 1;
    letter-spacing: -0.025em;
  }
  h2 { margin: 26px 0 6px; font-size: 1rem; }
  .compare { max-width: 62ch; margin: 16px 0 0; font-size: 1.05rem; line-height: 1.5; }
  .note { max-width: 62ch; margin: 14px 0 0; padding: 8px 12px; border-left: 3px solid var(--candidate); background: var(--hover); line-height: 1.45; }
  .actions { display: flex; gap: 10px; margin-top: 22px; }
  .count { margin: 24px 0 0; color: var(--muted); }
  ul { margin: 0; padding: 0; list-style: none; border-top: 1px solid var(--line); }
  li { display: flex; flex-direction: column; gap: 2px; padding: 8px 0; border-bottom: 1px solid var(--line); font-size: 0.9rem; }
  li span { color: var(--muted); overflow-wrap: anywhere; }
  .audit { margin-top: 18px; color: var(--muted); font-size: 0.85rem; }
  code { overflow-wrap: anywhere; }
</style>
