# Faxina

App desktop (Go + Wails v2 + Svelte/TS) que varre o disco, classifica o que ocupa espaço e limpa só o que for seguro. Alvo: Garuda (Arch), btrfs, Docker/kind. UI em pt-BR.

## Comandos
- `make dev`: app em modo desenvolvimento
- `make build`: binário em `build/bin/faxina`
- `make check`: `go vet`, testes e `svelte-check`
- `make scan`: varredura em JSON, sem UI
- `make dry-run`: prévia do Clean, sem alterar nada

O build exige `-tags webkit2_41` (o sistema tem `webkit2gtk-4.1`). O Makefile já passa a tag.

## Regras invioláveis
- Item sem regra é **Protegido**.
- Em uso e Protegido são recusados no backend, independentemente do que a UI envie.
- O frontend envia apenas IDs. Status, ação e alvo vêm sempre da última varredura do backend.
- Nada de `docker system prune` ou `docker volume prune` globais. A única exceção é `docker builder prune`.
- `pkexec` só com comandos fixos, sem interpolar dados da varredura.
- Testes nunca tocam dados reais: usam `runner` falso, diretório temporário ou recursos Docker marcados `faxina-test`.

## Estrutura
- `internal/model`: tipos compartilhados
- `internal/runner`: execução de comandos, com fake para testes
- `internal/scan`: coletores que só levantam fatos
- `internal/classify`: tabela de regras que define status, motivo e ação
- `internal/clean`: prévia, revalidação, execução e auditoria
- `app.go`: bindings Wails finos
