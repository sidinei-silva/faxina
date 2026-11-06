package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"faxina/internal/clean"
	"faxina/internal/model"
	"faxina/internal/runner"
	"faxina/internal/scan"
)

// CleanProgress é enviado à UI durante a limpeza.
type CleanProgress struct {
	Done  int    `json:"done"`
	Total int    `json:"total"`
	Name  string `json:"name"`
}

// App expõe a varredura e a limpeza à UI. Toda decisão fica nos pacotes internos.
type App struct {
	ctx  context.Context
	home string
	svc  *clean.Service

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewApp monta o serviço de limpeza.
func NewApp() *App {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return &App{
		home: home,
		svc: clean.New(clean.Options{
			Home:      home,
			Runner:    runner.Exec{},
			AuditPath: filepath.Join(home, ".local/state/faxina/clean.jsonl"),
		}),
	}
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// begin garante uma operação por vez, para o Cancelar sempre valer para a operação atual.
func (a *App) begin() (context.Context, func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		return nil, nil, errors.New("outra operação está em andamento")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel
	return ctx, func() {
		a.mu.Lock()
		a.cancel = nil
		a.mu.Unlock()
		cancel()
	}, nil
}

// Cancel interrompe a varredura ou a limpeza em andamento.
func (a *App) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// Scan executa uma varredura e a guarda como referência para a limpeza.
func (a *App) Scan() (model.ScanResult, error) {
	if a.home == "" {
		return model.ScanResult{}, errors.New("não foi possível identificar a pasta pessoal")
	}
	ctx, done, err := a.begin()
	if err != nil {
		return model.ScanResult{}, err
	}
	defer done()
	result := scan.Run(ctx, scan.Options{
		Home:   a.home,
		Runner: runner.Exec{},
		Progress: func(stage string) {
			runtime.EventsEmit(a.ctx, "scan:progress", stage)
		},
	})
	a.svc.SetScan(result)
	return result, nil
}

// Preview valida os IDs escolhidos e devolve a prévia da limpeza.
func (a *App) Preview(ids []string) (*clean.Plan, error) {
	ctx, done, err := a.begin()
	if err != nil {
		return nil, err
	}
	defer done()
	return a.svc.Preview(ctx, ids)
}

// Execute roda uma prévia já gerada.
func (a *App) Execute(planID string) (*clean.Report, error) {
	ctx, done, err := a.begin()
	if err != nil {
		return nil, err
	}
	defer done()
	return a.svc.Execute(ctx, planID, false, func(n, total int, name string) {
		runtime.EventsEmit(a.ctx, "clean:progress", CleanProgress{Done: n, Total: total, Name: name})
	})
}

// ReadDisk mede o espaço atual do disco.
func (a *App) ReadDisk() (model.Disk, error) {
	return scan.ReadDisk("/")
}
