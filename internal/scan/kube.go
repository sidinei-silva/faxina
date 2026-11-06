package scan

import (
	"context"
	"encoding/json"
	"strings"

	"faxina/internal/runner"
)

// KubeState diz quais imagens os clusters kind ativos usam.
// Unknown fica verdadeiro se não foi possível descartar o uso por algum cluster.
type KubeState struct {
	Clusters []string
	Deployed map[string]bool
	// DigestRepos guarda repositórios usados só por digest: não dá para saber qual tag é.
	DigestRepos map[string]bool
	Unknown     bool
}

// Uses diz se algum pod pode estar usando a imagem.
func (k KubeState) Uses(ref string) bool {
	key := NormalizeImageRef(ref)
	repo, _, _ := strings.Cut(key, "@")
	if i := strings.LastIndexByte(repo, ':'); i > strings.LastIndexByte(repo, '/') {
		repo = repo[:i]
	}
	return k.Deployed[key] || k.DigestRepos[repo]
}

func (k KubeState) record(image string) {
	base, _, hasDigest := strings.Cut(image, "@")
	if hasDigest && !hasTag(base) {
		k.DigestRepos[stripRegistry(base)] = true
		return
	}
	k.Deployed[NormalizeImageRef(image)] = true
}

// CollectKube consulta os pods de cada cluster kind.
func CollectKube(ctx context.Context, r runner.Runner) (KubeState, []string) {
	state := KubeState{Deployed: map[string]bool{}, DigestRepos: map[string]bool{}}
	out, err := r.Run(ctx, "kind", "get", "clusters")
	if err != nil {
		state.Unknown = true
		return state, []string{"Não foi possível listar clusters kind; imagens do registry local ficam como Candidatas: " + err.Error()}
	}
	var warnings []string
	for _, line := range strings.Split(out, "\n") {
		cluster := strings.TrimSpace(line)
		if cluster == "" {
			continue
		}
		state.Clusters = append(state.Clusters, cluster)
		pods, err := r.Run(ctx, "kubectl", "--context", "kind-"+cluster, "get", "pods", "-A", "-o", "json")
		if err != nil {
			state.Unknown = true
			warnings = append(warnings, "Não foi possível consultar o cluster "+cluster+"; imagens do registry local ficam como Candidatas: "+err.Error())
			continue
		}
		var list struct {
			Items []struct {
				Spec struct {
					Containers     []struct{ Image string }
					InitContainers []struct{ Image string }
				}
			}
		}
		if err := json.Unmarshal([]byte(pods), &list); err != nil {
			state.Unknown = true
			warnings = append(warnings, "Resposta inesperada do cluster "+cluster+": "+err.Error())
			continue
		}
		for _, pod := range list.Items {
			for _, c := range pod.Spec.Containers {
				state.record(c.Image)
			}
			for _, c := range pod.Spec.InitContainers {
				state.record(c.Image)
			}
		}
	}
	return state, warnings
}

// NormalizeImageRef deixa a referência comparável dentro e fora do cluster:
// sem host do registry, sem digest e com a tag latest explícita.
func NormalizeImageRef(ref string) string {
	base, _, _ := strings.Cut(ref, "@")
	if !hasTag(base) {
		base += ":latest"
	}
	return stripRegistry(base)
}

func stripRegistry(ref string) string {
	if first, rest, ok := strings.Cut(ref, "/"); ok && strings.ContainsAny(first, ".:") {
		return rest
	}
	return ref
}

func hasTag(ref string) bool {
	return strings.LastIndexByte(ref, ':') > strings.LastIndexByte(ref, '/')
}
