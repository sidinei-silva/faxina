package classify

import (
	"testing"
	"time"

	m "faxina/internal/model"
)

func TestApply(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	old := now.Add(-30 * 24 * time.Hour)
	recent := now.Add(-2 * 24 * time.Hour)

	tests := []struct {
		name   string
		item   m.Item
		want   m.Status
		action m.ActionKind
	}{
		{"volume com container", m.Item{Source: m.SourceVolume, Facts: m.Facts{Containers: 1, Anonymous: true, CreatedAt: old}}, m.StatusInUse, m.ActionNone},
		{"anônimo não inspecionado", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true, CreatedAt: old}}, m.StatusCandidate, m.ActionVolumeRm},
		{"anônimo com registry", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true, CreatedAt: old, Content: m.VolumeRegistry}}, m.StatusRemovable, m.ActionVolumeRm},
		{"anônimo vazio", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true, CreatedAt: old, Content: m.VolumeEmpty}}, m.StatusRemovable, m.ActionVolumeRm},
		{"anônimo com mysql", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true, CreatedAt: old, Content: m.VolumeMySQL}}, m.StatusCandidate, m.ActionVolumeRm},
		{"anônimo com postgres", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true, CreatedAt: old, Content: m.VolumePostgres}}, m.StatusCandidate, m.ActionVolumeRm},
		{"anônimo com redis", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true, CreatedAt: old, Content: m.VolumeRedis}}, m.StatusCandidate, m.ActionVolumeRm},
		{"anônimo desconhecido", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true, CreatedAt: old, Content: m.VolumeOther}}, m.StatusCandidate, m.ActionVolumeRm},
		{"registry recente", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true, CreatedAt: recent, Content: m.VolumeRegistry}}, m.StatusCandidate, m.ActionVolumeRm},
		{"nomeado com registry", m.Item{Source: m.SourceVolume, Facts: m.Facts{CreatedAt: old, Content: m.VolumeRegistry}}, m.StatusCandidate, m.ActionVolumeRm},
		{"volume anônimo recente", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true, CreatedAt: recent}}, m.StatusCandidate, m.ActionVolumeRm},
		{"volume nomeado antigo", m.Item{Source: m.SourceVolume, Facts: m.Facts{CreatedAt: old}}, m.StatusCandidate, m.ActionVolumeRm},
		{"volume sem data", m.Item{Source: m.SourceVolume, Facts: m.Facts{Anonymous: true}}, m.StatusCandidate, m.ActionVolumeRm},

		{"imagem em container", m.Item{Source: m.SourceImage, Facts: m.Facts{Containers: 2, Tag: "<none>", CreatedAt: old}}, m.StatusInUse, m.ActionNone},
		{"imagem em pod", m.Item{Source: m.SourceImage, Facts: m.Facts{Deployed: true, LocalRegistry: true, CreatedAt: old}}, m.StatusInUse, m.ActionNone},
		{"kube inacessível", m.Item{Source: m.SourceImage, Facts: m.Facts{KubeUnknown: true, LocalRegistry: true, CreatedAt: old}}, m.StatusCandidate, m.ActionImageRm},
		{"sem tag", m.Item{Source: m.SourceImage, Facts: m.Facts{Tag: "<none>", CreatedAt: old}}, m.StatusRemovable, m.ActionImageRm},
		{"infra com tag", m.Item{Source: m.SourceImage, Facts: m.Facts{Infra: true, Tag: "3", CreatedAt: old}}, m.StatusProtected, m.ActionNone},
		{"build antigo", m.Item{Source: m.SourceImage, Facts: m.Facts{LocalRegistry: true, Tag: "a", CreatedAt: old}}, m.StatusRemovable, m.ActionImageRm},
		{"build mais recente", m.Item{Source: m.SourceImage, Facts: m.Facts{LocalRegistry: true, NewestInRepo: true, Tag: "a", CreatedAt: old}}, m.StatusCandidate, m.ActionImageRm},
		{"build antigo mas novo", m.Item{Source: m.SourceImage, Facts: m.Facts{LocalRegistry: true, Tag: "a", CreatedAt: recent}}, m.StatusCandidate, m.ActionImageRm},
		{"imagem externa", m.Item{Source: m.SourceImage, Facts: m.Facts{Tag: "1", CreatedAt: old}}, m.StatusCandidate, m.ActionImageRm},

		{"build cache em uso", m.Item{Source: m.SourceBuildCacheUsed}, m.StatusInUse, m.ActionNone},
		{"build cache antigo", m.Item{Source: m.SourceBuildCacheOld}, m.StatusRemovable, m.ActionBuilderPruneOld},
		{"build cache recente", m.Item{Source: m.SourceBuildCacheNew}, m.StatusCandidate, m.ActionBuilderPruneAll},

		{"lixeira", m.Item{Source: m.SourceTrash, Path: "/x"}, m.StatusRemovable, m.ActionRemovePath},
		{"cache regenerável aberto", m.Item{Source: m.SourceRegenCache, Facts: m.Facts{OpenByProcess: true}}, m.StatusInUse, m.ActionNone},
		{"cache de app aberto", m.Item{Source: m.SourceAppCache, Facts: m.Facts{OpenByProcess: true, LastActivity: old}}, m.StatusInUse, m.ActionNone},
		{"cache de app ativo", m.Item{Source: m.SourceAppCache, Facts: m.Facts{LastActivity: now.Add(-time.Hour)}}, m.StatusCandidate, m.ActionRemovePath},
		{"cache de app parado", m.Item{Source: m.SourceAppCache, Facts: m.Facts{LastActivity: old}}, m.StatusRemovable, m.ActionRemovePath},
		{"cache de ferramenta", m.Item{Source: m.SourceToolCache, Facts: m.Facts{LastActivity: old}}, m.StatusCandidate, m.ActionRemovePath},

		{"node_modules sem lockfile", m.Item{Source: m.SourceNodeModules}, m.StatusProtected, m.ActionNone},
		{"node_modules ativo", m.Item{Source: m.SourceNodeModules, Facts: m.Facts{HasLockfile: true, ProjectActive: true}}, m.StatusProtected, m.ActionNone},
		{"node_modules parado", m.Item{Source: m.SourceNodeModules, Facts: m.Facts{HasLockfile: true}}, m.StatusCandidate, m.ActionRemovePath},
		{"backup nunca é padrão", m.Item{Source: m.SourceBackup}, m.StatusCandidate, m.ActionRemovePath},

		{"módulos Go abertos", m.Item{Source: m.SourceGoMod, Facts: m.Facts{OpenByProcess: true}}, m.StatusInUse, m.ActionNone},
		{"backup aberto", m.Item{Source: m.SourceBackup, Facts: m.Facts{OpenByProcess: true}}, m.StatusInUse, m.ActionNone},
		{"arquivo grande aberto", m.Item{Source: m.SourceLargeFile, Facts: m.Facts{OpenByProcess: true}}, m.StatusInUse, m.ActionNone},
		{"jogo antigo sem ação", m.Item{Source: m.SourceSteamGame, Facts: m.Facts{LastActivity: old.Add(-100 * 24 * time.Hour)}}, m.StatusCandidate, m.ActionNone},
		{"jogo recente", m.Item{Source: m.SourceSteamGame, Facts: m.Facts{LastActivity: recent}}, m.StatusProtected, m.ActionNone},
		{"unity sem ação", m.Item{Source: m.SourceUnity}, m.StatusCandidate, m.ActionNone},
		{"dados gc-stack", m.Item{Source: m.SourceGCStackData}, m.StatusProtected, m.ActionNone},
		{"pacman exige root", m.Item{Source: m.SourcePacmanCache}, m.StatusRemovable, m.ActionPaccache},
		{"órfãos sem ação", m.Item{Source: m.SourceOrphanPkgs}, m.StatusCandidate, m.ActionNone},
		{"snapshots só leitura", m.Item{Source: m.SourceSnapshots}, m.StatusProtected, m.ActionNone},
		{"desconhecido", m.Item{Source: m.SourceUnknown}, m.StatusProtected, m.ActionNone},
		{"fonte sem regra", m.Item{Source: "nova_fonte"}, m.StatusProtected, m.ActionNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := tt.item
			Apply(&it, now)
			if it.Status != tt.want {
				t.Errorf("status = %s, quer %s", it.Status, tt.want)
			}
			if it.Action.Kind != tt.action {
				t.Errorf("ação = %s, quer %s", it.Action.Kind, tt.action)
			}
			if it.Reason == "" {
				t.Error("todo item precisa de motivo")
			}
		})
	}
}

func TestSomenteLiberadosTemAcao(t *testing.T) {
	sources := []m.Source{m.SourceVolume, m.SourceImage, m.SourceBuildCacheUsed, m.SourceGCStackData, m.SourceUnknown, m.SourceSnapshots}
	for _, s := range sources {
		it := m.Item{Source: s, Facts: m.Facts{Containers: 1}}
		Apply(&it, time.Now())
		if (it.Status == m.StatusInUse || it.Status == m.StatusProtected) && it.Action.Executable() {
			t.Errorf("%s: %s não pode ter ação executável", s, it.Status)
		}
	}
}
