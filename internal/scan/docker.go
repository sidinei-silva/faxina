package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"faxina/internal/classify"
	m "faxina/internal/model"
	"faxina/internal/runner"
)

type dockerDF struct {
	Images []struct {
		ID, Repository, Tag, CreatedAt, Size, UniqueSize, Containers string
	}
	Volumes []struct {
		Name, Labels, Links, Size string
	}
	BuildCache []struct {
		InUse, Shared, Size, CreatedAt, LastUsedAt string
	}
}

// collectDocker levanta volumes, imagens e build cache.
func collectDocker(ctx context.Context, r runner.Runner, kube KubeState, now time.Time) ([]m.Item, []string) {
	out, err := r.Run(ctx, "docker", "system", "df", "-v", "--format", "json")
	if err != nil {
		return nil, []string{"Docker indisponível; volumes, imagens e build cache não foram analisados: " + err.Error()}
	}
	var df dockerDF
	if err := json.Unmarshal([]byte(out), &df); err != nil {
		return nil, []string{"Resposta inesperada de docker system df: " + err.Error()}
	}
	var warnings []string
	items, w := dockerVolumes(ctx, r, df)
	warnings = append(warnings, w...)
	items = append(items, dockerImages(df, kube)...)
	items = append(items, dockerBuildCache(df, now)...)
	return items, warnings
}

func dockerVolumes(ctx context.Context, r runner.Runner, df dockerDF) ([]m.Item, []string) {
	created := map[string]time.Time{}
	var warnings []string
	if len(df.Volumes) > 0 {
		args := []string{"volume", "inspect"}
		for _, v := range df.Volumes {
			args = append(args, v.Name)
		}
		out, err := r.Run(ctx, "docker", args...)
		if err != nil {
			warnings = append(warnings, "Sem data de criação dos volumes; eles ficam como Candidatos: "+err.Error())
		} else {
			var info []struct{ Name, CreatedAt string }
			if err := json.Unmarshal([]byte(out), &info); err != nil {
				warnings = append(warnings, "Resposta inesperada de docker volume inspect: "+err.Error())
			}
			for _, v := range info {
				if t, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil {
					created[v.Name] = t
				}
			}
		}
	}
	var orphans []string
	for _, v := range df.Volumes {
		if v.Links == "0" && ValidVolumeName(v.Name) {
			orphans = append(orphans, v.Name)
		}
	}
	contents, w := inspectVolumes(ctx, r, orphans)
	warnings = append(warnings, w...)

	items := make([]m.Item, 0, len(df.Volumes))
	for _, v := range df.Volumes {
		links, _ := strconv.Atoi(v.Links)
		if v.Links == "" || v.Links == "N/A" {
			links = 1 // sem contagem confiável, assume em uso
		}
		anonymous := strings.Contains(v.Labels, "com.docker.volume.anonymous")
		content := contents[v.Name]
		name, detail := v.Name, "Volume nomeado. "+ContentDescription(content)
		if anonymous {
			name, detail = "Volume anônimo "+shortID(v.Name), ContentDescription(content)+" ID: "+v.Name
		}
		items = append(items, m.Item{
			ID:     m.NewID(m.SourceVolume, v.Name),
			Source: m.SourceVolume, Group: m.GroupVolumes,
			Name: name, Detail: detail, SizeBytes: parseDockerSize(v.Size),
			Facts: m.Facts{Label: v.Name, Containers: links, Anonymous: anonymous, Content: content, CreatedAt: created[v.Name]},
		})
	}
	return items, warnings
}

func dockerImages(df dockerDF, kube KubeState) []m.Item {
	newest := map[string]time.Time{}
	type parsed struct {
		created time.Time
		local   bool
	}
	info := make([]parsed, len(df.Images))
	for i, img := range df.Images {
		t := parseDockerTime(img.CreatedAt)
		local := isLocalRegistry(img.Repository)
		info[i] = parsed{t, local}
		if local && t.After(newest[img.Repository]) {
			newest[img.Repository] = t
		}
	}
	seen := map[string]bool{}
	items := make([]m.Item, 0, len(df.Images))
	for i, img := range df.Images {
		containers, _ := strconv.Atoi(img.Containers)
		tagged := img.Tag != "" && img.Tag != "<none>" && img.Repository != "<none>"
		ref, tag := img.ID, "<none>"
		name := "Imagem sem tag " + shortID(strings.TrimPrefix(img.ID, "sha256:"))
		if img.Repository != "<none>" && !tagged {
			name = img.Repository + " (sem tag)"
		}
		if tagged {
			ref, tag, name = img.Repository+":"+img.Tag, img.Tag, img.Repository+":"+img.Tag
		}
		size := parseDockerSize(img.UniqueSize)
		detail := fmt.Sprintf("Tamanho total %s; %s exclusivos desta imagem.", img.Size, img.UniqueSize)
		if seen[img.ID] {
			size, detail = 0, "Mesma imagem de outra tag: o espaço só é liberado ao remover todas as tags."
		}
		seen[img.ID] = true
		items = append(items, m.Item{
			ID:     m.NewID(m.SourceImage, ref),
			Source: m.SourceImage, Group: m.GroupImages,
			Name: name, Detail: detail, SizeBytes: size, SizeEstimated: true,
			Facts: m.Facts{
				Label: ref, Repo: img.Repository, Tag: tag, ImageID: img.ID, Containers: containers,
				CreatedAt:     info[i].created,
				LocalRegistry: info[i].local,
				NewestInRepo:  info[i].local && !info[i].created.Before(newest[img.Repository]),
				Infra:         classify.IsInfraImage(img.Repository),
				Deployed:      tagged && kube.Deployed[NormalizeImageRef(ref)],
				KubeUnknown:   kube.Unknown,
			},
		})
	}
	return items
}

func dockerBuildCache(df dockerDF, now time.Time) []m.Item {
	var used, old, recent, shared int64
	var nUsed, nOld, nRecent int
	for _, b := range df.BuildCache {
		size := parseDockerSize(b.Size)
		last := parseDockerTime(b.LastUsedAt)
		if last.IsZero() {
			last = parseDockerTime(b.CreatedAt)
		}
		switch {
		case b.InUse == "true":
			used += size
			nUsed++
		case b.Shared == "true":
			shared += size
		case !last.IsZero() && now.Sub(last) > 7*24*time.Hour:
			old += size
			nOld++
		default:
			recent += size
			nRecent++
		}
	}
	mk := func(src m.Source, name string, size int64, n int) m.Item {
		return m.Item{
			ID: m.NewID(src, "build-cache"), Source: src, Group: m.GroupBuildCache, Name: name,
			Detail:    fmt.Sprintf("%d entradas. Outros %s são compartilhados com imagens e não entram na conta.", n, FormatBytes(shared)),
			SizeBytes: size, SizeEstimated: true,
		}
	}
	var items []m.Item
	if nUsed > 0 {
		items = append(items, mk(m.SourceBuildCacheUsed, "Build cache em uso", used, nUsed))
	}
	if nOld > 0 {
		items = append(items, mk(m.SourceBuildCacheOld, "Build cache sem uso há mais de 7 dias", old, nOld))
	}
	if nRecent > 0 {
		items = append(items, mk(m.SourceBuildCacheNew, "Build cache usado nos últimos 7 dias", recent, nRecent))
	}
	return items
}

func isLocalRegistry(repo string) bool {
	return strings.HasPrefix(repo, "localhost:") || strings.HasPrefix(repo, "127.0.0.1:")
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

var dockerSizeRe = regexp.MustCompile(`^([0-9.]+)\s*([kKMGT]?)i?B$`)

// parseDockerSize interpreta tamanhos do Docker, que usam base 1000 ("4.35GB").
func parseDockerSize(s string) int64 {
	match := dockerSizeRe.FindStringSubmatch(strings.TrimSpace(s))
	if match == nil {
		return 0
	}
	v, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0
	}
	mult := map[string]float64{"": 1, "k": 1e3, "K": 1e3, "M": 1e6, "G": 1e9, "T": 1e12}[match[2]]
	return int64(v * mult)
}

func parseDockerTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04:05 -0700 MST", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// FormatBytes formata em base 1024 para mensagens.
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
