// Package scan coleta fatos sobre o uso de disco, sem alterar nada.
package scan

import (
	"context"
	"sort"
	"sync"
	"time"

	"faxina/internal/classify"
	m "faxina/internal/model"
	"faxina/internal/runner"
)

// SystemRoots são medidos apenas para informação.
var SystemRoots = []string{"/usr", "/opt", "/srv", "/var/tmp", "/var/log"}

// Options configura uma varredura.
type Options struct {
	Home     string
	Runner   runner.Runner
	Now      time.Time
	ProcRoot string
	Progress func(stage string)
}

// Run executa todos os coletores em paralelo e classifica o resultado.
func Run(ctx context.Context, o Options) m.ScanResult {
	start := time.Now()
	if o.Now.IsZero() {
		o.Now = start
	}
	if o.ProcRoot == "" {
		o.ProcRoot = "/proc"
	}
	progress := func(s string) {
		if o.Progress != nil {
			o.Progress(s)
		}
	}

	var (
		mu       sync.Mutex
		items    []m.Item
		warnings []string
		wg       sync.WaitGroup
	)
	collect := func(stage string, fn func() ([]m.Item, []string)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			progress(stage)
			its, w := fn()
			mu.Lock()
			items = append(items, its...)
			warnings = append(warnings, w...)
			mu.Unlock()
		}()
	}
	collect("Analisando Docker e kind", func() ([]m.Item, []string) {
		kube, w := CollectKube(ctx, o.Runner)
		its, w2 := collectDocker(ctx, o.Runner, kube, o.Now)
		return its, append(w, w2...)
	})
	collect("Analisando pacotes do sistema", func() ([]m.Item, []string) { return collectSystem(ctx, o.Runner) })
	collect("Medindo diretórios do sistema", func() ([]m.Item, []string) { return ScanSystem(ctx, SystemRoots, o.Now) })
	collect("Percorrendo a pasta pessoal", func() ([]m.Item, []string) { return ScanHome(ctx, o.Home, o.Now, progress) })
	wg.Wait()

	progress("Classificando")
	open := CollectOpenPaths(o.ProcRoot)
	for i := range items {
		it := &items[i]
		if it.Path != "" && it.Source != m.SourceUnknown {
			it.Facts.OpenByProcess = open.Contains(it.Path)
		}
		classify.Apply(it, o.Now)
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].SizeBytes != items[b].SizeBytes {
			return items[a].SizeBytes > items[b].SizeBytes
		}
		return items[a].Name < items[b].Name
	})

	disk, err := ReadDisk("/")
	if err != nil {
		warnings = append(warnings, "Não foi possível ler o uso do disco: "+err.Error())
	}
	if ctx.Err() != nil {
		warnings = append(warnings, "Varredura cancelada: os resultados estão incompletos.")
	}
	return m.ScanResult{
		ScannedAt: o.Now, DurationMs: time.Since(start).Milliseconds(),
		Disk: disk, Items: items, Warnings: warnings,
	}
}
