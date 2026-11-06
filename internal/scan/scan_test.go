package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	m "faxina/internal/model"
	"faxina/internal/runner"
)

func write(t *testing.T, path string, size int, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

func bySource(items []m.Item, src m.Source) []m.Item {
	var out []m.Item
	for _, it := range items {
		if it.Source == src {
			out = append(out, it)
		}
	}
	return out
}

func TestScanHome(t *testing.T) {
	home := t.TempDir()
	now := time.Now()
	old := now.Add(-200 * 24 * time.Hour)
	write(t, filepath.Join(home, ".local/share/Trash/files/a"), 4096, old)
	write(t, filepath.Join(home, ".cache/go-build/x"), 4096, old)
	write(t, filepath.Join(home, ".cache/someapp/big"), 2<<20, old)
	write(t, filepath.Join(home, ".cache/paru/pkg"), 2<<20, old)
	write(t, filepath.Join(home, "meus_projetos/dev/pessoal/a/node_modules/x/i.js"), 4096, old)
	write(t, filepath.Join(home, "meus_projetos/dev/pessoal/a/package-lock.json"), 10, old)
	write(t, filepath.Join(home, "meus_projetos/dev/pessoal/b/node_modules/x/i.js"), 4096, old)
	write(t, filepath.Join(home, "meus_projetos/dev/pessoal/b/package.json"), 10, now)
	write(t, filepath.Join(home, "meus_projetos/dev/work/gamersclub/gc-stack/tmp/files/db"), 4096, old)
	write(t, filepath.Join(home, "Documentos/banco.sql.gz"), minBackupSize+1, old)
	write(t, filepath.Join(home, "Documentos/foto.png"), 4096, old)
	write(t, filepath.Join(home, ".local/share/Steam/steamapps/common/Jogo/d"), 4096, old)
	manifest := `"AppState" { "appid" "42" "name" "Jogo Teste" "installdir" "Jogo" "LastPlayed" "1700000000" }`
	write(t, filepath.Join(home, ".local/share/Steam/steamapps/appmanifest_42.acf"), 0, time.Time{})
	if err := os.WriteFile(filepath.Join(home, ".local/share/Steam/steamapps/appmanifest_42.acf"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc", filepath.Join(home, "link-externo")); err != nil {
		t.Fatal(err)
	}

	items, _ := ScanHome(context.Background(), home, now, nil)

	for src, want := range map[m.Source]int{
		m.SourceTrash: 1, m.SourceRegenCache: 1, m.SourceAppCache: 1, m.SourceToolCache: 1,
		m.SourceNodeModules: 2, m.SourceGCStackData: 1, m.SourceBackup: 1, m.SourceSteamGame: 1,
	} {
		if got := len(bySource(items, src)); got != want {
			t.Errorf("%s: %d itens, quer %d", src, got, want)
		}
	}
	for _, it := range bySource(items, m.SourceNodeModules) {
		switch {
		case strings.HasSuffix(it.Name, "/a"):
			if !it.Facts.HasLockfile || it.Facts.ProjectActive {
				t.Errorf("projeto a: %+v", it.Facts)
			}
		case strings.HasSuffix(it.Name, "/b"):
			if it.Facts.HasLockfile || !it.Facts.ProjectActive {
				t.Errorf("projeto b: %+v", it.Facts)
			}
		}
	}
	if g := bySource(items, m.SourceSteamGame); len(g) == 1 && (g[0].Name != "Jogo Teste" || g[0].Facts.LastActivity.Unix() != 1700000000) {
		t.Errorf("jogo: %+v", g[0])
	}
	for _, it := range items {
		if strings.Contains(it.Path, "link-externo") {
			t.Errorf("seguiu symlink: %s", it.Path)
		}
	}
}

func TestHardlinkContadoUmaVez(t *testing.T) {
	home := t.TempDir()
	a := filepath.Join(home, ".cache/app/a")
	write(t, a, 2<<20, time.Now())
	if err := os.Link(a, filepath.Join(home, ".cache/app/b")); err != nil {
		t.Fatal(err)
	}
	items, _ := ScanHome(context.Background(), home, time.Now(), nil)
	got := bySource(items, m.SourceAppCache)
	if len(got) != 1 || got[0].SizeBytes > 3<<20 {
		t.Fatalf("hardlink contado duas vezes: %+v", got)
	}
}

func TestDocker(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	df := `{"Images":[
	 {"ID":"sha256:aaa","Repository":"localhost:5000/svc","Tag":"old","CreatedAt":"2026-09-01 10:00:00 -0300 -03","Size":"2GB","UniqueSize":"100MB","Containers":"0"},
	 {"ID":"sha256:bbb","Repository":"localhost:5000/svc","Tag":"new","CreatedAt":"2026-10-01 10:00:00 -0300 -03","Size":"2GB","UniqueSize":"200MB","Containers":"0"},
	 {"ID":"sha256:ccc","Repository":"kindest/node","Tag":"<none>","CreatedAt":"2026-09-01 10:00:00 -0300 -03","Size":"1GB","UniqueSize":"1GB","Containers":"0"},
	 {"ID":"sha256:ddd","Repository":"registry","Tag":"3","CreatedAt":"2026-09-01 10:00:00 -0300 -03","Size":"85MB","UniqueSize":"85MB","Containers":"0"},
	 {"ID":"sha256:bbb","Repository":"localhost:5000/alias","Tag":"x","CreatedAt":"2026-10-01 10:00:00 -0300 -03","Size":"2GB","UniqueSize":"200MB","Containers":"0"}],
	"Volumes":[
	 {"Name":"anon1","Labels":"com.docker.volume.anonymous=","Links":"0","Size":"4.35GB"},
	 {"Name":"dados","Labels":"","Links":"0","Size":"1GB"},
	 {"Name":"usado","Labels":"com.docker.volume.anonymous=","Links":"1","Size":"1GB"}],
	"BuildCache":[
	 {"InUse":"false","Shared":"false","Size":"1GB","CreatedAt":"2026-09-01 10:00:00.5 +0000 UTC","LastUsedAt":"2026-09-02 10:00:00.5 +0000 UTC"},
	 {"InUse":"false","Shared":"true","Size":"9GB","CreatedAt":"2026-09-01 10:00:00.5 +0000 UTC","LastUsedAt":"2026-09-02 10:00:00.5 +0000 UTC"},
	 {"InUse":"false","Shared":"false","Size":"2GB","CreatedAt":"2026-11-05 10:00:00.5 +0000 UTC","LastUsedAt":"2026-11-05 11:00:00.5 +0000 UTC"}]}`
	r := &runner.Fake{Outputs: map[string]string{
		"docker ps -a -q --filter label=" + inspectLabel:                             "",
		"docker image inspect " + inspectImage:                                       "[]",
		"docker " + strings.Join(inspectCreateArgs([]string{"anon1", "dados"}), " "): "cid\n",
		"docker start -a cid":                                                        "anon1|yes|docker,\ndados|no|auto.cnf,ibdata1,\n",
		"docker rm cid":                                                              "cid\n",
		"docker system df -v --format json":                                          df,
		"docker volume inspect anon1 dados usado":                                    `[{"Name":"anon1","CreatedAt":"2026-09-01T10:00:00-03:00"},{"Name":"dados","CreatedAt":"2026-09-01T10:00:00-03:00"},{"Name":"usado","CreatedAt":"2026-09-01T10:00:00-03:00"}]`,
	}}
	items, warnings := collectDocker(context.Background(), r, KubeState{Deployed: map[string]bool{}}, now)
	if len(warnings) > 0 {
		t.Fatalf("avisos: %v", warnings)
	}
	find := func(label string) m.Facts {
		for _, it := range items {
			if it.Facts.Label == label {
				return it.Facts
			}
		}
		t.Fatalf("item %s não encontrado", label)
		return m.Facts{}
	}
	if f := find("localhost:5000/svc:new"); !f.NewestInRepo || !f.LocalRegistry {
		t.Errorf("new: %+v", f)
	}
	if f := find("localhost:5000/svc:old"); f.NewestInRepo || f.CreatedAt.IsZero() {
		t.Errorf("old: %+v", f)
	}
	if f := find("sha256:ccc"); f.Tag != "<none>" || !f.Infra {
		t.Errorf("kindest sem tag: %+v", f)
	}
	if f := find("anon1"); !f.Anonymous || f.Containers != 0 || f.CreatedAt.IsZero() || f.Content != m.VolumeRegistry {
		t.Errorf("anon1: %+v", f)
	}
	if f := find("dados"); f.Anonymous || f.Content != m.VolumeMySQL {
		t.Errorf("dados: %+v", f)
	}
	if f := find("usado"); f.Containers != 1 || f.Content != m.VolumeUnknown {
		t.Errorf("usado: %+v", f)
	}
	for _, it := range items {
		if it.Facts.Label == "localhost:5000/alias:x" && it.SizeBytes != 0 {
			t.Errorf("tag duplicada somou tamanho: %d", it.SizeBytes)
		}
	}
	old := bySource(items, m.SourceBuildCacheOld)
	if len(old) != 1 || old[0].SizeBytes != 1e9 {
		t.Errorf("cache antigo: %+v", old)
	}
	if rec := bySource(items, m.SourceBuildCacheNew); len(rec) != 1 || rec[0].SizeBytes != 2e9 {
		t.Errorf("cache recente: %+v", rec)
	}
}

func TestImagensDuplicadas(t *testing.T) {
	img := func(repo, tag, created string) string {
		return `{"ID":"sha256:` + repo + tag + `","Repository":"` + repo + `","Tag":"` + tag + `","CreatedAt":"` + created + `","Size":"1GB","UniqueSize":"1GB","Containers":"0"}`
	}
	const older, newer, newest = "2026-11-03 10:00:00 -0300 -03", "2026-11-04 10:00:00 -0300 -03", "2026-11-05 10:00:00 -0300 -03"
	df := `{"Images":[` + strings.Join([]string{
		img("localhost:5000/svc", "a1", older),
		img("localhost:5000/svc", "b2", newer),
		img("localhost:5000/svc", "<none>", newest),
		img("localhost:5000/unica", "x", older),
		img("gamersclub/app", "abc123", older),
		img("gamersclub/app", "def456", newer),
		img("mysql", "5.7", older),
		img("mysql", "8.0", newer),
	}, ",") + `],"Volumes":[],"BuildCache":[]}`
	r := &runner.Fake{Outputs: map[string]string{
		"docker ps -a -q --filter label=" + inspectLabel: "",
		"docker system df -v --format json":              df,
	}}
	items, warnings := collectDocker(context.Background(), r, KubeState{Deployed: map[string]bool{}}, time.Now())
	if len(warnings) > 0 {
		t.Fatalf("avisos: %v", warnings)
	}
	got := map[string]bool{}
	for _, it := range items {
		got[it.Facts.Repo+":"+it.Facts.Tag] = it.Facts.OlderDuplicate
	}
	for ref, want := range map[string]bool{
		"localhost:5000/svc:a1":     true,  // antiga, mesmo com menos de 7 dias
		"localhost:5000/svc:b2":     false, // a imagem sem tag mais nova não conta
		"localhost:5000/svc:<none>": false,
		"localhost:5000/unica:x":    false,
		"gamersclub/app:abc123":     true,
		"gamersclub/app:def456":     false,
		"mysql:5.7":                 false, // versões diferentes de propósito
		"mysql:8.0":                 false,
	} {
		if got[ref] != want {
			t.Errorf("%s: OlderDuplicate = %v, quer %v", ref, got[ref], want)
		}
	}
}

func TestEmpateDeixaUmaTagETamanhoNela(t *testing.T) {
	const when = "2026-11-03 10:00:00 -0300 -03"
	img := func(tag string) string {
		return `{"ID":"sha256:mesma","Repository":"localhost:5000/nginx","Tag":"` + tag + `","CreatedAt":"` + when + `","Size":"2GB","UniqueSize":"1GB","Containers":"0"}`
	}
	df := `{"Images":[` + img("aaa") + `,` + img("ccc") + `,` + img("bbb") + `],"Volumes":[],"BuildCache":[]}`
	r := &runner.Fake{Outputs: map[string]string{
		"docker ps -a -q --filter label=" + inspectLabel: "",
		"docker system df -v --format json":              df,
	}}
	items, _ := collectDocker(context.Background(), r, KubeState{Deployed: map[string]bool{}}, time.Now())
	kept := 0
	for _, it := range items {
		if it.Facts.OlderDuplicate {
			if it.SizeBytes != 0 {
				t.Errorf("%s: apelido removido não libera espaço, tamanho = %d", it.Name, it.SizeBytes)
			}
			continue
		}
		kept++
		if it.Facts.Tag != "ccc" || it.SizeBytes != 1e9 || !it.Facts.NewestInRepo {
			t.Errorf("tag que fica: %+v tamanho %d", it.Facts, it.SizeBytes)
		}
	}
	if kept != 1 {
		t.Fatalf("deveria sobrar 1 tag, sobraram %d", kept)
	}

	deployed := KubeState{Deployed: map[string]bool{"nginx:aaa": true}, DigestRepos: map[string]bool{}}
	items, _ = collectDocker(context.Background(), r, deployed, time.Now())
	for _, it := range items {
		if it.Facts.Tag == "aaa" && (it.Facts.OlderDuplicate || !it.Facts.Deployed) {
			t.Errorf("no empate, a tag usada por um pod deve ser a que fica: %+v", it.Facts)
		}
	}
}

func TestDockerIndisponivel(t *testing.T) {
	items, warnings := collectDocker(context.Background(), &runner.Fake{}, KubeState{}, time.Now())
	if len(items) != 0 || len(warnings) != 1 {
		t.Fatalf("items=%d warnings=%v", len(items), warnings)
	}
}

func TestKube(t *testing.T) {
	r := &runner.Fake{Outputs: map[string]string{
		"kind get clusters":                             "gc\n",
		"kubectl --context kind-gc get pods -A -o json": `{"items":[{"spec":{"containers":[{"image":"registry.local:5000/svc:abc"}],"initContainers":[{"image":"localhost:5000/init:1"}]}}]}`,
	}}
	state, _ := CollectKube(context.Background(), r)
	if !state.Deployed["svc:abc"] || !state.Deployed["init:1"] || state.Unknown {
		t.Fatalf("%+v", state)
	}
	if !state.Deployed[NormalizeImageRef("localhost:5000/svc:abc")] {
		t.Fatal("referência local não bate com a do cluster")
	}
	failing := &runner.Fake{Outputs: map[string]string{"kind get clusters": "gc\n"}}
	if state, _ := CollectKube(context.Background(), failing); !state.Unknown {
		t.Fatal("falha na consulta precisa marcar Unknown")
	}
	if state, _ := CollectKube(context.Background(), &runner.Fake{}); !state.Unknown {
		t.Fatal("kind ausente precisa marcar Unknown")
	}
}

func TestParsers(t *testing.T) {
	for in, want := range map[string]int64{"4.35GB": 4350000000, "117.3MB": 117300000, "0B": 0, "10kB": 10000, "lixo": 0} {
		if got := parseDockerSize(in); got != want {
			t.Errorf("parseDockerSize(%q) = %d, quer %d", in, got, want)
		}
	}
	if got := parsePaccacheSaved("==> finished dry run: 100 candidates (disk space saved: 3.88 GiB)"); got < 4100000000 || got > 4200000000 {
		t.Errorf("paccache = %d", got)
	}
	if parseDockerTime("2026-11-05 01:57:09 -0300 -03").IsZero() {
		t.Error("data da imagem não interpretada")
	}
	if parseDockerTime("2026-09-15 12:17:52.217141904 +0000 UTC").IsZero() {
		t.Error("data do build cache não interpretada")
	}
}

func TestOpenPaths(t *testing.T) {
	o := OpenPaths{paths: []string{"/a/b-x/f", "/a/b/c", "/z"}}
	for path, want := range map[string]bool{"/a/b": true, "/a/b/c": true, "/a/b-x": true, "/a/bb": false, "/q": false, "/z": true} {
		if got := o.Contains(path); got != want {
			t.Errorf("Contains(%s) = %v, quer %v", path, got, want)
		}
	}
}

func TestVolumeInspectionNuncaUsaRmComV(t *testing.T) {
	r := &runner.Fake{Outputs: map[string]string{
		"docker image inspect " + inspectImage:                           "[]",
		"docker " + strings.Join(inspectCreateArgs([]string{"v1"}), " "): "cid\n",
		"docker start -a cid":                                            "v1|no|\n",
		"docker rm cid":                                                  "",
	}}
	got, warnings := inspectVolumes(context.Background(), r, []string{"v1"})
	if got["v1"] != m.VolumeEmpty || len(warnings) != 0 {
		t.Fatalf("got=%v warnings=%v", got, warnings)
	}
	for _, call := range r.Calls {
		if strings.Contains(call, "rm -v") || strings.Contains(call, " --rm") || strings.Contains(call, "volume rm") {
			t.Errorf("chamada perigosa: %s", call)
		}
	}
}

func TestVolumeInspectionFalhaViraDesconhecido(t *testing.T) {
	got, warnings := inspectVolumes(context.Background(), &runner.Fake{}, []string{"v1"})
	if len(got) != 0 || len(warnings) == 0 {
		t.Fatalf("got=%v warnings=%v", got, warnings)
	}
}

func TestDetectVolumeContent(t *testing.T) {
	for _, tt := range []struct {
		entries  string
		registry bool
		want     m.VolumeContent
	}{
		{"", false, m.VolumeEmpty},
		{"docker,", true, m.VolumeRegistry},
		{"docker,", false, m.VolumeOther},
		{"docker,extra,", true, m.VolumeOther},
		{"auto.cnf,ca.pem,", false, m.VolumeMySQL},
		{"PG_VERSION,base,", false, m.VolumePostgres},
		{"dump.rdb,", false, m.VolumeRedis},
		{"foo,", false, m.VolumeOther},
	} {
		if got := detectVolumeContent(tt.entries, tt.registry); got != tt.want {
			t.Errorf("%q: %s, quer %s", tt.entries, got, tt.want)
		}
	}
}

func TestFalhaAoListarVolumeNaoPareceVazio(t *testing.T) {
	r := &runner.Fake{Outputs: map[string]string{
		"docker image inspect " + inspectImage:                                 "[]",
		"docker " + strings.Join(inspectCreateArgs([]string{"v1", "v2"}), " "): "cid\n",
		"docker start -a cid": "v1|err|\nv2|no|a|b,\n",
		"docker rm cid":       "",
	}}
	got, _ := inspectVolumes(context.Background(), r, []string{"v1", "v2"})
	if c, ok := got["v1"]; ok {
		t.Errorf("falha na listagem virou %q", c)
	}
	if got["v2"] != m.VolumeOther {
		t.Errorf("nome com barra vertical: %q", got["v2"])
	}
}

func TestInspetorAntigoSaiAntesDoDF(t *testing.T) {
	r := &runner.Fake{Outputs: map[string]string{
		"docker ps -a -q --filter label=" + inspectLabel: "velho\n",
		"docker rm velho":                   "",
		"docker system df -v --format json": `{"Images":[],"Volumes":[],"BuildCache":[]}`,
	}}
	collectDocker(context.Background(), r, KubeState{}, time.Now())
	if len(r.Calls) < 3 || r.Calls[1] != "docker rm velho" || !strings.HasPrefix(r.Calls[2], "docker system df") {
		t.Fatalf("ordem: %v", r.Calls)
	}
}

func TestKubeReferencias(t *testing.T) {
	r := &runner.Fake{Outputs: map[string]string{
		"kind get clusters": "gc\n",
		"kubectl --context kind-gc get pods -A -o json": `{"items":[{"spec":{"containers":[
		 {"image":"localhost:5000/implicita"},
		 {"image":"localhost:5000/comtag:abc@sha256:111"},
		 {"image":"localhost:5000/sotag@sha256:222"}]}}]}`,
	}}
	state, _ := CollectKube(context.Background(), r)
	for ref, want := range map[string]bool{
		"localhost:5000/implicita:latest": true,
		"localhost:5000/implicita:velha":  false,
		"localhost:5000/comtag:abc":       true,
		"localhost:5000/comtag:def":       false,
		"localhost:5000/sotag:qualquer":   true,
		"localhost:5000/outra:1":          false,
	} {
		if got := state.Uses(ref); got != want {
			t.Errorf("Uses(%s) = %v, quer %v", ref, got, want)
		}
	}
}

func TestLinkRepetidoContaComoAtividade(t *testing.T) {
	home := t.TempDir()
	old := time.Now().Add(-240 * time.Hour)
	recent := time.Now().Add(-time.Hour)
	first := filepath.Join(home, ".cache/a/f")
	write(t, first, 2<<20, old)
	write(t, filepath.Join(home, ".cache/a/novo"), 2<<20, recent)
	write(t, filepath.Join(home, ".cache/b/base"), 2<<20, old)
	if err := os.Link(filepath.Join(home, ".cache/a/novo"), filepath.Join(home, ".cache/b/novo")); err != nil {
		t.Fatal(err)
	}
	items, _ := ScanHome(context.Background(), home, time.Now(), nil)
	got := bySource(items, m.SourceAppCache)
	if len(got) != 2 {
		t.Fatalf("itens: %+v", got)
	}
	for _, it := range got {
		if it.Facts.LastActivity.Before(recent.Add(-time.Minute)) {
			t.Errorf("%s ignorou o link recente: %v", it.Name, it.Facts.LastActivity)
		}
	}
}
