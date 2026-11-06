package clean

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"faxina/internal/classify"
	m "faxina/internal/model"
	"faxina/internal/runner"
)

func fakeDisk() (m.Disk, error) { return m.Disk{FreeBytes: 100}, nil }

func writeFile(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func item(src m.Source, path string, last time.Time) m.Item {
	it := m.Item{ID: string(src) + path, Source: src, Name: path, Path: path, SizeBytes: 10, Facts: m.Facts{LastActivity: last}}
	classify.Apply(&it, last.Add(48*time.Hour))
	return it
}

func newService(t *testing.T, home string, r runner.Runner, items ...m.Item) *Service {
	t.Helper()
	proc := t.TempDir() // /proc falso e vazio: nenhum processo real interfere
	s := New(Options{Home: home, Runner: r, ProcRoot: proc, ReadDisk: fakeDisk, AuditPath: filepath.Join(t.TempDir(), "audit.jsonl")})
	s.SetScan(m.ScanResult{Items: items})
	return s
}

func TestProtegidoEEmUsoRecusados(t *testing.T) {
	home := t.TempDir()
	old := time.Now().Add(-240 * time.Hour)
	path := filepath.Join(home, ".cache/app/f")
	writeFile(t, path, old)
	r := &runner.Fake{}
	base := item(m.SourceAppCache, filepath.Dir(path), old)

	protected := base
	protected.ID, protected.Status = "p", m.StatusProtected
	inUse := base
	inUse.ID, inUse.Status = "u", m.StatusInUse
	s := newService(t, home, r, protected, inUse)

	plan, err := s.Preview(context.Background(), []string{"p", "u", "inexistente"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Accepted != 0 || plan.Refused != 3 {
		t.Fatalf("plan: %+v", plan)
	}
	report, err := s.Execute(context.Background(), plan.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 0 {
		t.Fatalf("nada deveria executar: %+v", report.Outcomes)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("arquivo protegido foi apagado")
	}
}

func TestDefaultIDsSoRemoviveis(t *testing.T) {
	r := m.ScanResult{Items: []m.Item{
		{ID: "r", Status: m.StatusRemovable, Action: m.Action{Kind: m.ActionVolumeRm}},
		{ID: "c", Status: m.StatusCandidate, Action: m.Action{Kind: m.ActionVolumeRm}},
		{ID: "p", Status: m.StatusProtected},
		{ID: "u", Status: m.StatusInUse},
		{ID: "m", Status: m.StatusRemovable, Action: m.Action{Kind: m.ActionNone}},
	}}
	got := DefaultIDs(r)
	if len(got) != 1 || got[0] != "r" {
		t.Fatalf("seleção padrão: %v", got)
	}
}

func TestAllowlist(t *testing.T) {
	home := t.TempDir()
	old := time.Now()
	writeFile(t, filepath.Join(home, ".cache/app/f"), old)
	writeFile(t, filepath.Join(home, "Documentos/nota.txt"), old)
	writeFile(t, filepath.Join(home, "p/node_modules/x"), old)
	writeFile(t, filepath.Join(home, "semlock/node_modules/x"), old)
	writeFile(t, filepath.Join(home, "p/package-lock.json"), old)
	writeFile(t, filepath.Join(home, "banco.sql.gz"), old)
	writeFile(t, filepath.Join(home, ".git/x.bak"), old)
	writeFile(t, filepath.Join(home, "meus_projetos/dev/work/gamersclub/gc-stack/tmp/a.bak"), old)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "valioso"), old)
	if err := os.Symlink(outside, filepath.Join(home, ".cache/link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "linkdir")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		src  m.Source
		path string
		ok   bool
	}{
		{"cache", m.SourceAppCache, ".cache/app", true},
		{"raiz da home", m.SourceAppCache, "", false},
		{"pasta .cache inteira", m.SourceAppCache, ".cache", false},
		{"cache como documento", m.SourceAppCache, "Documentos", false},
		{"symlink de cache", m.SourceAppCache, ".cache/link", false},
		{"symlink no meio", m.SourceBackup, "linkdir/valioso", false},
		{"lixeira errada", m.SourceTrash, "Documentos", false},
		{"node_modules com lock", m.SourceNodeModules, "p/node_modules", true},
		{"node_modules sem lock", m.SourceNodeModules, "semlock/node_modules", false},
		{"pasta qualquer como node_modules", m.SourceNodeModules, "Documentos", false},
		{"backup", m.SourceBackup, "banco.sql.gz", true},
		{"arquivo comum como backup", m.SourceBackup, "Documentos/nota.txt", false},
		{"backup dentro de .git", m.SourceBackup, ".git/x.bak", false},
		{"dados do gc-stack", m.SourceBackup, "meus_projetos/dev/work/gamersclub/gc-stack/tmp/a.bak", false},
		{"fonte sem regra", m.SourceUnknown, "Documentos", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(home, tt.path)
			err := PathAllowed(m.Item{Source: tt.src, Path: path}, home)
			if (err == nil) != tt.ok {
				t.Errorf("err = %v, quer ok=%v", err, tt.ok)
			}
		})
	}
	for _, bad := range []string{"/", "/etc", outside, filepath.Join(home, "x/../../fora")} {
		if err := PathAllowed(m.Item{Source: m.SourceBackup, Path: bad}, home); err == nil {
			t.Errorf("%s deveria ser recusado", bad)
		}
	}
}

func TestExecutorArvoreFalsa(t *testing.T) {
	home := t.TempDir()
	old := time.Now().Add(-240 * time.Hour)
	writeFile(t, filepath.Join(home, ".cache/app/sub/f"), old)
	writeFile(t, filepath.Join(home, ".local/share/Trash/files/lixo"), old)
	writeFile(t, filepath.Join(home, "go/pkg/mod/x/f"), old)
	if err := os.Chmod(filepath.Join(home, "go/pkg/mod/x"), 0o500); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, "Documentos/nota.txt"), old)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "valioso"), old)
	if err := os.Symlink(outside, filepath.Join(home, ".cache/app/link")); err != nil {
		t.Fatal(err)
	}

	app := item(m.SourceAppCache, filepath.Join(home, ".cache/app"), old)
	trash := item(m.SourceTrash, filepath.Join(home, ".local/share/Trash"), old)
	mod := item(m.SourceGoMod, filepath.Join(home, "go/pkg/mod"), old)
	s := newService(t, home, &runner.Fake{Outputs: map[string]string{"kind get clusters": ""}}, app, trash, mod)

	plan, err := s.Preview(context.Background(), []string{app.ID, trash.ID, mod.ID})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Accepted != 3 {
		t.Fatalf("plan: %+v", plan)
	}
	report, err := s.Execute(context.Background(), plan.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range report.Outcomes {
		if !o.OK {
			t.Errorf("%s falhou: %s", o.Name, o.Error)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".cache/app")); !os.IsNotExist(err) {
		t.Error("cache não removido")
	}
	if _, err := os.Stat(filepath.Join(home, "go/pkg/mod")); !os.IsNotExist(err) {
		t.Error("módulos não removidos")
	}
	if _, err := os.Stat(filepath.Join(home, ".local/share/Trash/files")); !os.IsNotExist(err) {
		t.Error("conteúdo da lixeira não removido")
	}
	if _, err := os.Stat(filepath.Join(home, ".local/share/Trash")); err != nil {
		t.Error("a pasta da lixeira deve continuar existindo")
	}
	if _, err := os.Stat(filepath.Join(outside, "valioso")); err != nil {
		t.Error("seguiu symlink e apagou arquivo externo")
	}
	if _, err := os.Stat(filepath.Join(home, "Documentos/nota.txt")); err != nil {
		t.Error("apagou arquivo fora do plano")
	}
	data, err := os.ReadFile(s.opts.AuditPath)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 3 {
		t.Errorf("auditoria: %d linhas", lines)
	}
}

func TestItemAlteradoAposVarredura(t *testing.T) {
	home := t.TempDir()
	old := time.Now().Add(-240 * time.Hour)
	path := filepath.Join(home, ".cache/app/f")
	writeFile(t, path, old)
	it := item(m.SourceAppCache, filepath.Dir(path), old)
	s := newService(t, home, &runner.Fake{Outputs: map[string]string{"kind get clusters": ""}}, it)
	plan, err := s.Preview(context.Background(), []string{it.ID})
	if err != nil || plan.Accepted != 1 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	writeFile(t, path, time.Now()) // mudou entre a prévia e a execução
	report, err := s.Execute(context.Background(), plan.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 || !report.Outcomes[0].Skipped {
		t.Fatalf("deveria recusar: %+v", report.Outcomes)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("arquivo alterado foi apagado")
	}
}

func TestDryRunNaoAltera(t *testing.T) {
	home := t.TempDir()
	old := time.Now().Add(-240 * time.Hour)
	path := filepath.Join(home, ".cache/app/f")
	writeFile(t, path, old)
	it := item(m.SourceAppCache, filepath.Dir(path), old)
	s := newService(t, home, &runner.Fake{Outputs: map[string]string{"kind get clusters": ""}}, it)
	plan, _ := s.Preview(context.Background(), []string{it.ID})
	report, err := s.Execute(context.Background(), plan.ID, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.EstimatedBytes != it.SizeBytes {
		t.Errorf("estimado: %d", report.EstimatedBytes)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("dry-run apagou o arquivo")
	}
	if _, err := os.Stat(s.opts.AuditPath); !os.IsNotExist(err) {
		t.Fatal("dry-run gravou auditoria")
	}
}

func TestPreviaUsoUnico(t *testing.T) {
	s := newService(t, t.TempDir(), &runner.Fake{})
	plan, _ := s.Preview(context.Background(), nil)
	if _, err := s.Execute(context.Background(), plan.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(context.Background(), plan.ID, true, nil); err == nil {
		t.Fatal("prévia reutilizada")
	}
	if _, err := s.Execute(context.Background(), "inventada", false, nil); err == nil {
		t.Fatal("prévia inventada aceita")
	}
}

func TestComandosDocker(t *testing.T) {
	vol := m.Item{ID: "v", Source: m.SourceVolume, Name: "v", Facts: m.Facts{Label: "vol1", Anonymous: true, CreatedAt: time.Now().Add(-240 * time.Hour), Content: m.VolumeRegistry}}
	classify.Apply(&vol, time.Now())
	img := m.Item{ID: "i", Source: m.SourceImage, Name: "i", Facts: m.Facts{Label: "localhost:5000/a:1", ImageID: "sha256:a", Tag: "1", LocalRegistry: true, CreatedAt: time.Now().Add(-240 * time.Hour)}}
	classify.Apply(&img, time.Now())
	r := &runner.Fake{Outputs: map[string]string{
		"docker volume inspect vol1":                               "[]",
		"docker ps -a -q --filter volume=vol1":                     "",
		"docker image inspect --format {{.Id}} localhost:5000/a:1": "sha256:a\n",
		"docker ps -a -q --filter ancestor=localhost:5000/a:1":     "",
		"kind get clusters":                                        "",
		"docker volume rm vol1":                                    "",
		"docker rmi localhost:5000/a:1":                            "",
	}}
	s := newService(t, t.TempDir(), r, vol, img)
	plan, err := s.Preview(context.Background(), []string{"v", "i"})
	if err != nil || plan.Accepted != 2 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if _, err := s.Execute(context.Background(), plan.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.Calls, "\n")
	for _, forbidden := range []string{"system prune", "volume prune", "image prune", " -f localhost", "rmi -f", "volume rm -f"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("comando proibido %q em:\n%s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "docker volume rm vol1") || !strings.Contains(joined, "docker rmi localhost:5000/a:1") {
		t.Errorf("comandos esperados ausentes:\n%s", joined)
	}
}

func TestVolumeQuePassouASerUsado(t *testing.T) {
	vol := m.Item{ID: "v", Source: m.SourceVolume, Name: "v", Status: m.StatusRemovable, Action: m.Action{Kind: m.ActionVolumeRm}, Facts: m.Facts{Label: "vol1"}}
	r := &runner.Fake{Outputs: map[string]string{
		"docker volume inspect vol1":           "[]",
		"docker ps -a -q --filter volume=vol1": "abc\n",
	}}
	s := newService(t, t.TempDir(), r, vol)
	plan, _ := s.Preview(context.Background(), []string{"v"})
	if plan.Accepted != 0 {
		t.Fatalf("volume em uso aceito: %+v", plan)
	}
}

func TestImagemKubeDesconhecido(t *testing.T) {
	img := m.Item{ID: "i", Source: m.SourceImage, Name: "i", Status: m.StatusRemovable, Action: m.Action{Kind: m.ActionImageRm}, Facts: m.Facts{Label: "localhost:5000/a:1", ImageID: "sha256:a", LocalRegistry: true}}
	r := &runner.Fake{
		Outputs: map[string]string{
			"docker image inspect --format {{.Id}} localhost:5000/a:1": "sha256:a",
			"docker ps -a -q --filter ancestor=localhost:5000/a:1":     "",
		},
		Errors: map[string]error{"kind get clusters": errors.New("sem kind")},
	}
	s := newService(t, t.TempDir(), r, img)
	plan, _ := s.Preview(context.Background(), []string{"i"})
	if plan.Accepted != 0 {
		t.Fatalf("deveria recusar sem confirmar o kube: %+v", plan)
	}
}

func TestPaccacheScriptFixo(t *testing.T) {
	it := m.Item{ID: "p", Source: m.SourcePacmanCache, Name: "p"}
	classify.Apply(&it, time.Now())
	r := &runner.Fake{Outputs: map[string]string{"pkexec /usr/bin/sh -c " + paccacheScript: ""}}
	s := newService(t, t.TempDir(), r, it)
	plan, _ := s.Preview(context.Background(), []string{"p"})
	if !plan.NeedsRoot || plan.Accepted != 1 {
		t.Fatalf("plan: %+v", plan)
	}
	report, err := s.Execute(context.Background(), plan.ID, false, nil)
	if err != nil || !report.Outcomes[0].OK {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestExplainGap(t *testing.T) {
	if explainGap(10<<30, 1<<30) == "" {
		t.Error("diferença grande precisa de explicação")
	}
	if explainGap(10<<30, 9<<30) != "" || explainGap(1<<20, 0) != "" {
		t.Error("não deveria explicar")
	}
}

// Teste de integração: usa só recursos próprios, marcados faxina-test.
func TestIntegracaoDocker(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker ausente")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker indisponível")
	}
	suffix := time.Now().Format("150405.000")
	suffix = strings.ReplaceAll(suffix, ".", "")
	orphan, attached, ctr := "faxina-test-orphan-"+suffix, "faxina-test-attached-"+suffix, "faxina-test-ctr-"+suffix
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	imageOut, err := exec.Command("docker", "images", "--filter", "reference=registry:*", "--format", "{{.Repository}}:{{.Tag}}").Output()
	if err != nil {
		t.Skip("sem lista de imagens")
	}
	scanner := bufio.NewScanner(strings.NewReader(string(imageOut)))
	if !scanner.Scan() {
		t.Skip("sem imagem local para o teste")
	}
	image := scanner.Text()

	run("volume", "create", "--label", "faxina-test=true", orphan)
	run("volume", "create", "--label", "faxina-test=true", attached)
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "-v", ctr).Run()
		_ = exec.Command("docker", "volume", "rm", orphan).Run()
		_ = exec.Command("docker", "volume", "rm", attached).Run()
	})
	run("create", "--name", ctr, "--label", "faxina-test=true", "-v", attached+":/data", "--tmpfs", "/var/lib/registry", image)

	old := time.Now().Add(-240 * time.Hour)
	mk := func(name string) m.Item {
		it := m.Item{ID: name, Source: m.SourceVolume, Name: name, Facts: m.Facts{Label: name, Anonymous: true, Content: m.VolumeRegistry, CreatedAt: old}}
		classify.Apply(&it, time.Now())
		return it
	}
	s := newService(t, t.TempDir(), runner.Exec{}, mk(orphan), mk(attached))
	plan, err := s.Preview(context.Background(), []string{orphan, attached})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Accepted != 1 || plan.Refused != 1 {
		t.Fatalf("plan: %+v", plan)
	}
	report, err := s.Execute(context.Background(), plan.ID, false, nil)
	if err != nil || len(report.Outcomes) != 1 || !report.Outcomes[0].OK {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if exec.Command("docker", "volume", "inspect", orphan).Run() == nil {
		t.Error("o volume órfão deveria ter sido removido")
	}
	if err := exec.Command("docker", "volume", "inspect", attached).Run(); err != nil {
		t.Error("o volume com container deveria continuar")
	}
}
