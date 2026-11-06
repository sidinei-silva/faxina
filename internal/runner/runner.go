// Package runner isola a execução de comandos externos para permitir testes sem efeitos.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executa um comando e devolve o stdout.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// Exec é o Runner real.
type Exec struct{}

// Run executa o comando diretamente, sem shell.
func (Exec) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return stdout.String(), fmt.Errorf("%s: %w", name, err)
		}
		return stdout.String(), fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return stdout.String(), nil
}

// Fake é um Runner para testes: responde por comando completo e registra as chamadas.
type Fake struct {
	Outputs map[string]string
	Errors  map[string]error
	Calls   []string
}

// Run devolve a resposta configurada; comando desconhecido gera erro.
func (f *Fake) Run(_ context.Context, name string, args ...string) (string, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.Calls = append(f.Calls, key)
	if err, ok := f.Errors[key]; ok {
		return "", err
	}
	if out, ok := f.Outputs[key]; ok {
		return out, nil
	}
	return "", fmt.Errorf("fake: comando inesperado: %s", key)
}
