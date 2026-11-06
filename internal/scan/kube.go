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
	Unknown  bool
}

// CollectKube consulta os pods de cada cluster kind.
func CollectKube(ctx context.Context, r runner.Runner) (KubeState, []string) {
	state := KubeState{Deployed: map[string]bool{}}
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
				state.Deployed[NormalizeImageRef(c.Image)] = true
			}
			for _, c := range pod.Spec.InitContainers {
				state.Deployed[NormalizeImageRef(c.Image)] = true
			}
		}
	}
	return state, warnings
}

// NormalizeImageRef remove o host do registry, pois o nome muda dentro e fora do cluster.
func NormalizeImageRef(ref string) string {
	if first, rest, ok := strings.Cut(ref, "/"); ok && strings.ContainsAny(first, ".:") {
		return rest
	}
	return ref
}
