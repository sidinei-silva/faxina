# Faxina

App desktop que mostra o que ocupa o disco, separa o que pode sair do que deve ficar e limpa só o que você confirmar. Feito para Garuda/Arch com btrfs, Docker e kind.

## Status dos itens

| Status | Significado | Limpeza |
|---|---|---|
| Removível | Regra explícita diz que pode sair | Já vem marcado |
| Candidato | Provavelmente dispensável | Só se você marcar |
| Em uso | Usado por container, pod ou processo | Recusada |
| Protegido | Dados pessoais, gc-stack ou item sem regra | Recusada |

## Requisitos

- Go, Node e o CLI do Wails v2 (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1`)
- `webkit2gtk-4.1` e `gtk3`
- Docker, `kind` e `kubectl` para analisar o ambiente Docker/kind
- `pacman-contrib` (`paccache`) e `pkexec` para limpar pacotes

## Uso

```sh
make build             # gera build/bin/faxina
./build/bin/faxina     # abre o app e já faz a varredura
make check             # vet, testes e svelte-check
make scan              # varredura em JSON, sem UI
make dry-run           # prévia do Clean padrão, sem alterar nada
```

## O que a varredura altera

Nada nos seus dados. Para saber o conteúdo dos volumes Docker sem container, ela cria um container temporário de `alpine:3.22`, com os volumes em modo somente leitura e sem rede, e o remove em seguida sem `-v`. Se a imagem não existir, ela é baixada e removida no fim. Se a inspeção falhar, os volumes ficam como Candidatos.

## Regras de volumes

| Volume sem container | Status |
|---|---|
| Anônimo, mais de 7 dias, só com dados de registry | Removível |
| Anônimo, mais de 7 dias, vazio | Removível |
| Anônimo com MySQL, Postgres, Redis ou conteúdo desconhecido | Candidato |
| Anônimo com menos de 7 dias | Candidato |
| Nomeado | Candidato |

## Segurança da limpeza

- A UI envia só IDs. Status, ação e alvo vêm da última varredura no backend.
- Cada prévia vale uma única vez, e cada item é revalidado logo antes de executar.
- Volumes e imagens são removidos um a um, sem `-f` e sem prune global. A exceção é `docker builder prune`, sempre `-a -f`, com filtro de idade quando o item é o cache antigo.
- Caminhos seguem uma allowlist por tipo de item, sem seguir symlinks.
- `pkexec` só roda um script fixo de `paccache`.
- Toda execução real fica em `~/.local/state/faxina/clean.jsonl`.

**Apagar não tem como desfazer.** Confira a prévia antes de confirmar.

## Por que o valor real difere da estimativa

O sistema usa `compress=zstd`, então o tamanho medido pode ser maior que o ocupado de fato. Camadas de imagens são compartilhadas, e snapshots btrfs podem manter dados apagados. Por isso a tela final mostra a diferença real no espaço livre, com a opção de medir de novo.

## Limitações conhecidas

- Mede só a biblioteca padrão da Steam e ignora subvolumes aninhados na home.
- Snapshots do snapper são apenas indicados: listar e medir exige root.
- Um volume nomeado nunca é Removível, mesmo vazio.
- A detecção de arquivos abertos enxerga apenas os processos do seu usuário.

## Origem do maior resíduo nesta máquina

O `gc cluster up` cria o `registry:3` sem volume nomeado. A imagem declara `VOLUME`, então cada container recriado gera um volume anônimo novo, e os antigos ficam para trás ao remover o container sem `-v`. Na primeira inspeção eram 31 volumes e 140 GB. Usar um volume nomeado em `cli/internal/actions/cluster.go` do gc-stack evitaria o acúmulo.
