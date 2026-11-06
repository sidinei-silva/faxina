// Package classify concentra as regras que definem status, motivo e ação de cada item.
package classify

import (
	"fmt"
	"strings"
	"time"

	m "faxina/internal/model"
)

const (
	dockerMinAge    = 7 * 24 * time.Hour
	appCacheQuiet   = 24 * time.Hour
	steamUnplayed   = 90 * 24 * time.Hour
	builderPruneOld = "docker builder prune -a -f --filter until=168h"
	builderPruneAll = "docker builder prune -a -f"
	paccacheCommand = "pkexec paccache -rk2 && paccache -ruk0"
)

// Apply preenche status, motivo, ação e idade do item.
// O padrão é Protegido: só regra explícita libera a remoção.
func Apply(it *m.Item, now time.Time) {
	f := it.Facts
	it.AgeDays = ageDays(reference(f), now)
	it.ManualHint = f.ManualHint
	set := func(s m.Status, reason string, a m.Action) {
		it.Status, it.Reason, it.Action = s, reason, a
	}
	none := m.Action{Kind: m.ActionNone}
	removePath := m.Action{Kind: m.ActionRemovePath, Target: it.Path, Command: "remover " + it.Path}

	switch it.Source {
	case m.SourceVolume:
		rm := m.Action{Kind: m.ActionVolumeRm, Target: f.Label, Command: "docker volume rm " + f.Label}
		switch {
		case f.Containers > 0:
			set(m.StatusInUse, fmt.Sprintf("Montado em %d container(s).", f.Containers), none)
		case f.CreatedAt.IsZero():
			set(m.StatusCandidate, "Sem container, mas a data de criação é desconhecida.", rm)
		case !f.Anonymous:
			set(m.StatusCandidate, "Volume nomeado sem container: pode guardar dados que você quer manter.", rm)
		case now.Sub(f.CreatedAt) <= dockerMinAge:
			set(m.StatusCandidate, "Volume anônimo sem container, mas criado há menos de 7 dias.", rm)
		case f.Content == m.VolumeRegistry:
			set(m.StatusRemovable, "Cópia do registry local sem container há mais de 7 dias: as imagens voltam com novo build.", rm)
		case f.Content == m.VolumeEmpty:
			set(m.StatusRemovable, "Volume anônimo vazio, sem container, criado há mais de 7 dias.", rm)
		case f.Content == m.VolumeMySQL, f.Content == m.VolumePostgres, f.Content == m.VolumeRedis:
			set(m.StatusCandidate, "Contém dados de banco: confira antes de remover.", rm)
		case f.Content == m.VolumeOther:
			set(m.StatusCandidate, "Volume anônimo com conteúdo não reconhecido: confira antes de remover.", rm)
		default:
			set(m.StatusCandidate, "Volume anônimo que não pôde ser inspecionado.", rm)
		}

	case m.SourceImage:
		rm := m.Action{Kind: m.ActionImageRm, Target: f.Label, Command: "docker rmi " + f.Label}
		switch {
		case f.Containers > 0:
			set(m.StatusInUse, fmt.Sprintf("Usada por %d container(s).", f.Containers), none)
		case f.Deployed:
			set(m.StatusInUse, "Usada por um pod do cluster kind.", none)
		case f.KubeUnknown && f.LocalRegistry:
			set(m.StatusCandidate, "O cluster kind está ativo, mas não foi possível consultar os pods.", rm)
		case f.Tag == "<none>":
			set(m.StatusRemovable, "Imagem sem tag e sem container.", rm)
		case f.Infra:
			set(m.StatusProtected, "Infraestrutura do ambiente kind e do gc-stack.", none)
		case f.LocalRegistry && !f.NewestInRepo && !f.CreatedAt.IsZero() && now.Sub(f.CreatedAt) > dockerMinAge:
			set(m.StatusRemovable, "Build antigo do registry local, com tag mais recente disponível.", rm)
		case f.LocalRegistry && f.NewestInRepo:
			set(m.StatusCandidate, "Build mais recente deste serviço: remover exige novo build.", rm)
		case f.LocalRegistry:
			set(m.StatusCandidate, "Build antigo, mas criado há menos de 7 dias.", rm)
		default:
			set(m.StatusCandidate, "Imagem sem container: remover exige novo download.", rm)
		}

	case m.SourceBuildCacheUsed:
		set(m.StatusInUse, "Cache em uso pelo Docker neste momento.", none)
	case m.SourceBuildCacheOld:
		set(m.StatusRemovable, "Cache de build sem uso há mais de 7 dias.",
			m.Action{Kind: m.ActionBuilderPruneOld, Command: builderPruneOld})
	case m.SourceBuildCacheNew:
		set(m.StatusCandidate, "Cache de build usado nos últimos 7 dias: acelera os próximos builds. Limpar este item apaga também o cache antigo.",
			m.Action{Kind: m.ActionBuilderPruneAll, Command: builderPruneAll})

	case m.SourceTrash:
		if f.OpenByProcess {
			set(m.StatusInUse, "Há processo com arquivo aberto aqui.", none)
		} else {
			set(m.StatusRemovable, "Lixeira: os arquivos já foram descartados.", removePath)
		}
	case m.SourceRegenCache:
		if f.OpenByProcess {
			set(m.StatusInUse, "Há processo com arquivo aberto aqui.", none)
		} else {
			set(m.StatusRemovable, "Cache regenerado automaticamente pela ferramenta.", removePath)
		}
	case m.SourceAppCache:
		switch {
		case f.OpenByProcess:
			set(m.StatusInUse, "O aplicativo está com arquivos abertos aqui.", none)
		case now.Sub(f.LastActivity) < appCacheQuiet:
			set(m.StatusCandidate, "Cache alterado nas últimas 24 horas.", removePath)
		default:
			set(m.StatusRemovable, "Cache de aplicativo fechado, regenerado quando necessário.", removePath)
		}
	case m.SourceToolCache:
		if f.OpenByProcess {
			set(m.StatusInUse, "Há processo com arquivo aberto aqui.", none)
		} else {
			set(m.StatusCandidate, "Downloads de ferramenta: remover exige baixar de novo.", removePath)
		}

	case m.SourceNodeModules:
		switch {
		case f.OpenByProcess:
			set(m.StatusInUse, "Há processo com arquivo aberto aqui.", none)
		case !f.HasLockfile:
			set(m.StatusProtected, "Projeto sem lockfile: reinstalar pode gerar versões diferentes.", none)
		case f.ProjectActive:
			set(m.StatusProtected, "Projeto com atividade nos últimos 30 dias.", none)
		default:
			set(m.StatusCandidate, "Projeto parado há mais de 30 dias; dá para restaurar pelo lockfile.", removePath)
		}
	case m.SourceGoMod:
		if f.OpenByProcess {
			set(m.StatusInUse, "Há processo com arquivo aberto aqui.", none)
		} else {
			set(m.StatusCandidate, "Cache de módulos Go: remover exige baixar de novo.", removePath)
		}

	case m.SourceBackup:
		if f.OpenByProcess {
			set(m.StatusInUse, "Há processo com este arquivo aberto.", none)
		} else {
			set(m.StatusCandidate, "Parece um backup: confira antes de remover, pois não há como recuperar.", removePath)
		}
	case m.SourceLargeFile:
		if f.OpenByProcess {
			set(m.StatusInUse, "Há processo com este arquivo aberto.", none)
		} else {
			set(m.StatusCandidate, "Arquivo grande sem alteração há mais de 90 dias: confira antes de remover.", removePath)
		}

	case m.SourceSteamGame:
		if f.LastActivity.IsZero() || now.Sub(f.LastActivity) > steamUnplayed {
			set(m.StatusCandidate, "Jogo sem uso há mais de 90 dias: desinstale pela Steam.", none)
		} else {
			set(m.StatusProtected, "Jogo usado nos últimos 90 dias.", none)
		}
	case m.SourceUnity:
		set(m.StatusCandidate, "Editor ou cache de projeto Unity: remova pelo Unity Hub ou manualmente.", none)

	case m.SourceGCStackData:
		set(m.StatusProtected, "Dados e código do ambiente local gc-stack.", none)

	case m.SourcePacmanCache:
		set(m.StatusRemovable, "Pacotes antigos: mantém as 2 versões mais recentes e remove os de pacotes desinstalados.",
			m.Action{Kind: m.ActionPaccache, Command: paccacheCommand, NeedsRoot: true})
	case m.SourceOrphanPkgs:
		set(m.StatusCandidate, "Pacotes instalados como dependência e não mais exigidos: revise a lista antes.", none)
	case m.SourceJournal:
		set(m.StatusCandidate, "Logs do sistema: reduza com o comando sugerido.", none)
	case m.SourceSnapshots:
		set(m.StatusProtected, "Snapshots permitem rollback: a remoção é manual.", none)

	default:
		set(m.StatusProtected, "Nenhuma regra reconhece este item: ele nunca entra no Clean.", none)
	}
}

func reference(f m.Facts) time.Time {
	if !f.LastActivity.IsZero() {
		return f.LastActivity
	}
	return f.CreatedAt
}

func ageDays(t, now time.Time) int {
	if t.IsZero() {
		return -1
	}
	return int(now.Sub(t).Hours() / 24)
}

// IsInfraImage indica imagens necessárias para subir o kind e o gc-stack.
func IsInfraImage(repo string) bool {
	return repo == "registry" || strings.HasPrefix(repo, "kindest/")
}
