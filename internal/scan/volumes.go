package scan

import (
	"context"
	"regexp"
	"strings"

	m "faxina/internal/model"
	"faxina/internal/runner"
)

const (
	inspectImage = "alpine:3.22"
	inspectLabel = "faxina.inspect=true"
	// inspectScript imprime "nome|entradas|registry?" para cada volume montado.
	inspectScript = `for d in /v/*; do [ -d "$d" ] || continue; r=no; [ -d "$d/docker/registry" ] && r=yes; echo "$(basename "$d")|$(ls -A "$d" | head -20 | tr '\n' ',')|$r"; done`
)

var volumeNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidVolumeName rejeita nomes que poderiam alterar a montagem ou o comando.
func ValidVolumeName(name string) bool { return volumeNameRe.MatchString(name) }

func inspectCreateArgs(names []string) []string {
	args := []string{"create", "--label", inspectLabel, "--network", "none", "--read-only"}
	for _, n := range names {
		args = append(args, "-v", n+":/v/"+n+":ro")
	}
	return append(args, inspectImage, "sh", "-c", inspectScript)
}

// inspectVolumes descobre o conteúdo de volumes sem container, com um container somente leitura.
// O container é removido sem -v, o que nunca apaga volumes.
// Em qualquer falha, o conteúdo fica desconhecido e as regras tratam o volume como Candidato.
func inspectVolumes(ctx context.Context, r runner.Runner, names []string) (map[string]m.VolumeContent, []string) {
	out := map[string]m.VolumeContent{}
	if len(names) == 0 {
		return out, nil
	}
	cleanup := context.WithoutCancel(ctx)
	removeStaleInspectors(cleanup, r)

	pulled := false
	if _, err := r.Run(ctx, "docker", "image", "inspect", inspectImage); err != nil {
		if _, err := r.Run(ctx, "docker", "pull", "-q", inspectImage); err != nil {
			return out, []string{"Não foi possível obter a imagem de inspeção; os volumes ficam como Candidatos: " + err.Error()}
		}
		pulled = true
	}
	if pulled {
		defer func() { _, _ = r.Run(cleanup, "docker", "rmi", inspectImage) }()
	}

	id, err := r.Run(ctx, "docker", inspectCreateArgs(names)...)
	if err != nil {
		return out, []string{"Não foi possível inspecionar os volumes; eles ficam como Candidatos: " + err.Error()}
	}
	id = strings.TrimSpace(id)
	defer func() { _, _ = r.Run(cleanup, "docker", "rm", id) }()

	listing, err := r.Run(ctx, "docker", "start", "-a", id)
	if err != nil {
		return out, []string{"Falha ao ler os volumes; eles ficam como Candidatos: " + err.Error()}
	}
	for _, line := range strings.Split(listing, "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) == 3 {
			out[parts[0]] = detectVolumeContent(parts[1], parts[2] == "yes")
		}
	}
	return out, nil
}

func removeStaleInspectors(ctx context.Context, r runner.Runner) {
	ids, err := r.Run(ctx, "docker", "ps", "-a", "-q", "--filter", "label="+inspectLabel)
	if err != nil {
		return
	}
	for _, id := range strings.Fields(ids) {
		_, _ = r.Run(ctx, "docker", "rm", id)
	}
}

func detectVolumeContent(entries string, hasRegistry bool) m.VolumeContent {
	list := strings.Split(strings.TrimSuffix(entries, ","), ",")
	has := func(name string) bool {
		for _, e := range list {
			if e == name || strings.HasPrefix(e, name+".") {
				return true
			}
		}
		return false
	}
	switch {
	case entries == "":
		return m.VolumeEmpty
	case entries == "docker," && hasRegistry:
		return m.VolumeRegistry
	case has("auto.cnf") || has("ibdata1") || has("mysql"):
		return m.VolumeMySQL
	case has("PG_VERSION"):
		return m.VolumePostgres
	case has("dump") || has("appendonly"):
		return m.VolumeRedis
	default:
		return m.VolumeOther
	}
}

// ContentDescription resume o conteúdo para exibir na UI.
func ContentDescription(c m.VolumeContent) string {
	switch c {
	case m.VolumeEmpty:
		return "Vazio."
	case m.VolumeRegistry:
		return "Conteúdo: dados do registry local (docker/registry)."
	case m.VolumeMySQL:
		return "Conteúdo: banco MySQL/MariaDB."
	case m.VolumePostgres:
		return "Conteúdo: banco Postgres."
	case m.VolumeRedis:
		return "Conteúdo: dump Redis/Valkey."
	case m.VolumeOther:
		return "Conteúdo não reconhecido."
	}
	return "Conteúdo não inspecionado."
}
