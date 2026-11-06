// Package clean monta a prévia, revalida cada item e executa a limpeza com auditoria.
package clean

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	m "faxina/internal/model"
	"faxina/internal/runner"
	"faxina/internal/scan"
)

// Script fixo do pacman: nunca recebe dados da varredura.
const (
	paccacheScript = "/usr/bin/paccache -rk2 && /usr/bin/paccache -ruk0"
	maxLargeFile   = 1 << 30
)

// Entry é uma linha da prévia.
type Entry struct {
	ItemID        string   `json:"itemId"`
	Name          string   `json:"name"`
	Status        m.Status `json:"status"`
	Command       string   `json:"command"`
	SizeBytes     int64    `json:"sizeBytes"`
	SizeEstimated bool     `json:"sizeEstimated"`
	NeedsRoot     bool     `json:"needsRoot"`
	Refused       string   `json:"refused"`
}

// Plan é uma prévia confirmável. Só o backend conhece os itens por trás dos IDs.
type Plan struct {
	ID           string  `json:"id"`
	Entries      []Entry `json:"entries"`
	TotalBytes   int64   `json:"totalBytes"`
	Accepted     int     `json:"accepted"`
	Refused      int     `json:"refused"`
	NeedsRoot    bool    `json:"needsRoot"`
	HasCandidate bool    `json:"hasCandidate"`
}

// Outcome é o resultado de um item.
type Outcome struct {
	ItemID    string `json:"itemId"`
	Name      string `json:"name"`
	Command   string `json:"command"`
	OK        bool   `json:"ok"`
	Skipped   bool   `json:"skipped"`
	Error     string `json:"error"`
	SizeBytes int64  `json:"sizeBytes"`
}

// Report resume a execução e compara estimado com real.
type Report struct {
	PlanID         string    `json:"planId"`
	DryRun         bool      `json:"dryRun"`
	Canceled       bool      `json:"canceled"`
	Outcomes       []Outcome `json:"outcomes"`
	EstimatedBytes int64     `json:"estimatedBytes"`
	FreedBytes     int64     `json:"freedBytes"`
	Before         m.Disk    `json:"before"`
	After          m.Disk    `json:"after"`
	Note           string    `json:"note"`
	AuditPath      string    `json:"auditPath"`
	AuditError     string    `json:"auditError"`
}

// Options configura o serviço.
type Options struct {
	Home      string
	Runner    runner.Runner
	ProcRoot  string
	AuditPath string
	ReadDisk  func() (m.Disk, error)
	Now       func() time.Time
}

// Service guarda a última varredura e as prévias válidas.
type Service struct {
	opts  Options
	mu    sync.Mutex
	items map[string]m.Item
	plans map[string]*Plan
}

// New cria o serviço com padrões seguros.
func New(o Options) *Service {
	if o.ProcRoot == "" {
		o.ProcRoot = "/proc"
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.ReadDisk == nil {
		o.ReadDisk = func() (m.Disk, error) { return scan.ReadDisk("/") }
	}
	return &Service{opts: o, plans: map[string]*Plan{}}
}

// SetScan troca a varredura de referência e invalida as prévias anteriores.
func (s *Service) SetScan(r m.ScanResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]m.Item, len(r.Items))
	for _, it := range r.Items {
		s.items[it.ID] = it
	}
	s.plans = map[string]*Plan{}
}

// DefaultIDs devolve a seleção padrão: apenas Removíveis com ação.
func DefaultIDs(r m.ScanResult) []string {
	ids := []string{}
	for _, it := range r.Items {
		if it.Status == m.StatusRemovable && it.Action.Executable() {
			ids = append(ids, it.ID)
		}
	}
	return ids
}

// Preview valida os IDs e gera a prévia. Nada é alterado.
func (s *Service) Preview(ctx context.Context, ids []string) (*Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		return nil, errors.New("nenhuma varredura disponível")
	}
	v := newValidator(ctx, s.opts)
	plan := &Plan{ID: newPlanID(), Entries: []Entry{}}
	seen := map[string]bool{}
	ids = s.withImpliedItems(ids)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		it, ok := s.items[id]
		if !ok {
			plan.Entries = append(plan.Entries, Entry{ItemID: id, Name: id, Refused: "Item desconhecido nesta varredura."})
			plan.Refused++
			continue
		}
		entry := Entry{
			ItemID: id, Name: it.Name, Status: it.Status, Command: it.Action.Command,
			SizeBytes: it.SizeBytes, SizeEstimated: it.SizeEstimated, NeedsRoot: it.Action.NeedsRoot,
		}
		if err := v.check(it); err != nil {
			entry.Refused = err.Error()
			plan.Refused++
		} else {
			plan.Accepted++
			plan.TotalBytes += it.SizeBytes
			plan.NeedsRoot = plan.NeedsRoot || it.Action.NeedsRoot
			plan.HasCandidate = plan.HasCandidate || it.Status == m.StatusCandidate
		}
		plan.Entries = append(plan.Entries, entry)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.New("prévia cancelada")
	}
	sortEntries(plan.Entries, s.items)
	s.plans[plan.ID] = plan
	return plan, nil
}

// withImpliedItems torna visível o que um comando apaga além do item marcado:
// o prune sem filtro também remove o build cache antigo.
func (s *Service) withImpliedItems(ids []string) []string {
	out := append([]string{}, ids...)
	for _, id := range ids {
		if s.items[id].Source != m.SourceBuildCacheNew {
			continue
		}
		for otherID, other := range s.items {
			if other.Source == m.SourceBuildCacheOld {
				out = append(out, otherID)
			}
		}
	}
	return out
}

// Execute roda uma prévia uma única vez, revalidando item a item.
// Com dryRun, só valida: não executa nem grava auditoria.
func (s *Service) Execute(ctx context.Context, planID string, dryRun bool, progress func(done, total int, name string)) (*Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, ok := s.plans[planID]
	if !ok {
		return nil, errors.New("prévia inválida ou já usada: gere uma nova prévia")
	}
	delete(s.plans, planID)

	report := &Report{PlanID: planID, DryRun: dryRun, Outcomes: []Outcome{}}
	report.Before, _ = s.opts.ReadDisk()
	if !dryRun {
		report.AuditPath = s.opts.AuditPath
	}
	// Uma ação em andamento sempre termina: cancelar só impede os próximos itens.
	work := context.WithoutCancel(ctx)
	v := newValidator(work, s.opts)
	total := plan.Accepted
	done := 0
	auditFailed := func(err error) {
		if err != nil && report.AuditError == "" {
			report.AuditError = err.Error()
		}
	}
	for _, entry := range plan.Entries {
		if entry.Refused != "" {
			if !dryRun {
				auditFailed(s.audit(map[string]any{"event": "refused", "plan": planID, "item": entry.ItemID, "name": entry.Name, "error": entry.Refused}))
			}
			continue
		}
		it := s.items[entry.ItemID]
		out := Outcome{ItemID: it.ID, Name: it.Name, Command: it.Action.Command, SizeBytes: it.SizeBytes}
		done++
		if progress != nil {
			progress(done, total, it.Name)
		}
		switch {
		case ctx.Err() != nil:
			out.Skipped, out.Error = true, "Limpeza cancelada antes deste item."
		default:
			if err := v.check(it); err != nil {
				out.Skipped, out.Error = true, "Recusado na revalidação: "+err.Error()
			} else if !dryRun {
				// O registro vem antes: uma queda no meio ainda deixa rastro.
				auditFailed(s.audit(map[string]any{
					"event": "start", "plan": planID, "item": it.ID, "name": it.Name, "status": it.Status,
					"command": it.Action.Command, "path": it.Path, "sizeBytes": it.SizeBytes,
				}))
				if err := s.run(work, it); err != nil {
					out.Error = err.Error()
				} else {
					out.OK = true
					report.EstimatedBytes += it.SizeBytes
				}
			} else {
				out.OK = true
				report.EstimatedBytes += it.SizeBytes
			}
		}
		report.Outcomes = append(report.Outcomes, out)
		if !dryRun {
			auditFailed(s.audit(map[string]any{
				"event": "result", "plan": planID, "item": it.ID, "name": it.Name,
				"ok": out.OK, "skipped": out.Skipped, "error": out.Error,
			}))
		}
	}
	report.Canceled = ctx.Err() != nil
	if !dryRun {
		syscall.Sync()
	}
	report.After, _ = s.opts.ReadDisk()
	report.FreedBytes = report.After.FreeBytes - report.Before.FreeBytes
	if !dryRun {
		report.Note = explainGap(report.EstimatedBytes, report.FreedBytes)
	}
	return report, nil
}

func explainGap(estimated, freed int64) string {
	if estimated >= 1<<30 && freed < estimated/2 {
		return "Foi liberado bem menos que o estimado. No btrfs, isso pode vir de compressão, de snapshots que ainda guardam os dados ou da liberação em segundo plano. Meça de novo em alguns minutos."
	}
	return ""
}

func (s *Service) run(ctx context.Context, it m.Item) error {
	r := s.opts.Runner
	var err error
	switch it.Action.Kind {
	case m.ActionVolumeRm:
		_, err = r.Run(ctx, "docker", "volume", "rm", it.Facts.Label)
	case m.ActionImageRm:
		_, err = r.Run(ctx, "docker", "rmi", it.Facts.Label)
	case m.ActionBuilderPruneOld:
		_, err = r.Run(ctx, "docker", "builder", "prune", "-a", "-f", "--filter", "until=168h")
	case m.ActionBuilderPruneAll:
		_, err = r.Run(ctx, "docker", "builder", "prune", "-a", "-f")
	case m.ActionPaccache:
		_, err = r.Run(ctx, "pkexec", "/usr/bin/sh", "-c", paccacheScript)
	case m.ActionRemovePath:
		err = removePath(it)
	default:
		err = fmt.Errorf("ação não permitida: %s", it.Action.Kind)
	}
	return err
}

// removePath apaga sem seguir symlinks. Permissões são ajustadas só nas pastas do alvo,
// pois o cache de módulos Go é somente leitura.
func removePath(it m.Item) error {
	info, err := os.Lstat(it.Path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.Remove(it.Path)
	}
	_ = filepath.WalkDir(it.Path, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})
	if it.Source != m.SourceTrash {
		return os.RemoveAll(it.Path)
	}
	children, err := os.ReadDir(it.Path)
	if err != nil {
		return err
	}
	for _, c := range children {
		if err := os.RemoveAll(filepath.Join(it.Path, c.Name())); err != nil {
			return err
		}
	}
	return nil
}

// audit grava um evento. A falha não interrompe a limpeza, pois disco cheio é justamente
// o caso de uso, mas é informada na tela final.
func (s *Service) audit(event map[string]any) error {
	if s.opts.AuditPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.opts.AuditPath), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.opts.AuditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	event["time"] = s.opts.Now().Format(time.RFC3339)
	if err := json.NewEncoder(f).Encode(event); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// sortEntries executa imagens antes de volumes, build cache e arquivos; root por último.
func sortEntries(entries []Entry, items map[string]m.Item) {
	rank := map[m.ActionKind]int{
		m.ActionImageRm: 0, m.ActionVolumeRm: 1, m.ActionBuilderPruneOld: 2, m.ActionBuilderPruneAll: 3,
		m.ActionRemovePath: 4, m.ActionPaccache: 5,
	}
	sort.SliceStable(entries, func(a, b int) bool {
		ra, rb := rank[items[entries[a].ItemID].Action.Kind], rank[items[entries[b].ItemID].Action.Kind]
		if ra != rb {
			return ra < rb
		}
		return entries[a].SizeBytes > entries[b].SizeBytes
	})
}

func newPlanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// validator recheca o estado real; cache de kube e /proc valem por lote.
type validator struct {
	ctx  context.Context
	opts Options
	once sync.Once
	kube scan.KubeState
	open scan.OpenPaths
}

func newValidator(ctx context.Context, o Options) *validator { return &validator{ctx: ctx, opts: o} }

func (v *validator) load() {
	v.once.Do(func() {
		v.kube, _ = scan.CollectKube(v.ctx, v.opts.Runner)
		v.open = scan.CollectOpenPaths(v.opts.ProcRoot)
	})
}

var allowedActions = map[m.Source]m.ActionKind{
	m.SourceVolume: m.ActionVolumeRm, m.SourceImage: m.ActionImageRm,
	m.SourceBuildCacheOld: m.ActionBuilderPruneOld, m.SourceBuildCacheNew: m.ActionBuilderPruneAll,
	m.SourceTrash: m.ActionRemovePath, m.SourceRegenCache: m.ActionRemovePath, m.SourceAppCache: m.ActionRemovePath,
	m.SourceToolCache: m.ActionRemovePath, m.SourceNodeModules: m.ActionRemovePath, m.SourceGoMod: m.ActionRemovePath,
	m.SourceBackup: m.ActionRemovePath, m.SourceLargeFile: m.ActionRemovePath, m.SourcePacmanCache: m.ActionPaccache,
}

func (v *validator) check(it m.Item) error {
	switch it.Status {
	case m.StatusRemovable, m.StatusCandidate:
	case m.StatusInUse:
		return errors.New("O item está em uso.")
	case m.StatusProtected:
		return errors.New("O item está protegido.")
	default:
		return errors.New("O item não tem classificação válida.")
	}
	if !it.Action.Executable() {
		return errors.New("Este item exige ação manual.")
	}
	if allowed, ok := allowedActions[it.Source]; !ok || allowed != it.Action.Kind {
		return errors.New("Ação incompatível com o tipo do item.")
	}
	switch it.Action.Kind {
	case m.ActionVolumeRm:
		return v.checkVolume(it)
	case m.ActionImageRm:
		return v.checkImage(it)
	case m.ActionRemovePath:
		return v.checkPath(it)
	}
	return nil
}

func (v *validator) checkVolume(it m.Item) error {
	name := it.Facts.Label
	if !scan.ValidVolumeName(name) {
		return errors.New("Nome de volume inválido.")
	}
	if _, err := v.opts.Runner.Run(v.ctx, "docker", "volume", "inspect", name); err != nil {
		return errors.New("O volume não existe mais.")
	}
	out, err := v.opts.Runner.Run(v.ctx, "docker", "ps", "-a", "-q", "--filter", "volume="+name)
	if err != nil {
		return fmt.Errorf("Não foi possível confirmar o uso do volume: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		return errors.New("Um container passou a usar este volume.")
	}
	return nil
}

func (v *validator) checkImage(it m.Item) error {
	ref := it.Facts.Label
	if ref == "" || strings.HasPrefix(ref, "-") {
		return errors.New("Referência de imagem inválida.")
	}
	id, err := v.opts.Runner.Run(v.ctx, "docker", "image", "inspect", "--format", "{{.Id}}", ref)
	if err != nil {
		return errors.New("A imagem não existe mais.")
	}
	if strings.TrimSpace(id) != it.Facts.ImageID {
		return errors.New("A imagem mudou depois da varredura.")
	}
	out, err := v.opts.Runner.Run(v.ctx, "docker", "ps", "-a", "-q", "--filter", "ancestor="+ref)
	if err != nil {
		return fmt.Errorf("Não foi possível confirmar o uso da imagem: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		return errors.New("Um container passou a usar esta imagem.")
	}
	if !it.Facts.LocalRegistry {
		return nil
	}
	v.load()
	if v.kube.Unknown {
		return errors.New("Não foi possível confirmar se o cluster kind usa esta imagem.")
	}
	if v.kube.Uses(ref) {
		return errors.New("Um pod do cluster kind passou a usar esta imagem.")
	}
	return nil
}

func (v *validator) checkPath(it m.Item) error {
	if err := PathAllowed(it, v.opts.Home); err != nil {
		return err
	}
	v.load()
	if v.open.Contains(it.Path) {
		return errors.New("Há processo com arquivo aberto neste caminho.")
	}
	newest, err := inspectTree(it.Path)
	if err != nil {
		return err
	}
	if !it.Facts.LastActivity.IsZero() && newest.After(it.Facts.LastActivity.Add(time.Second)) {
		return errors.New("O conteúdo mudou depois da varredura.")
	}
	return nil
}

// PathAllowed é a allowlist: cada tipo de item só pode apagar o caminho previsto para ele.
func PathAllowed(it m.Item, home string) error {
	path := it.Path
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("Caminho inválido.")
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return fmt.Errorf("Pasta pessoal inválida: %v", err)
	}
	if path == "/" || path == home || path == realHome || !strings.HasPrefix(path, home+"/") {
		return errors.New("Caminho fora da área permitida.")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return errors.New("O caminho não existe mais.")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("O caminho é um symlink.")
	}
	rel := strings.TrimPrefix(path, home+"/")
	parts := strings.Split(rel, "/")
	cur := realHome
	for _, part := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if err != nil || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("Há symlink no caminho.")
		}
	}
	for _, reserved := range []string{"tmp", "projects", "platform"} {
		root := "meus_projetos/dev/work/gamersclub/gc-stack/" + reserved
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return errors.New("Dados do gc-stack são protegidos.")
		}
	}
	denied := errors.New("Caminho fora da allowlist para este tipo de item.")
	switch it.Source {
	case m.SourceTrash:
		if rel != ".local/share/Trash" {
			return denied
		}
	case m.SourceRegenCache:
		if rel != ".cache/go-build" && rel != ".npm/_cacache" {
			return denied
		}
	case m.SourceAppCache, m.SourceToolCache:
		if len(parts) != 2 || parts[0] != ".cache" || !info.IsDir() {
			return denied
		}
	case m.SourceGoMod:
		if rel != "go/pkg/mod" {
			return denied
		}
	case m.SourceNodeModules:
		if filepath.Base(path) != "node_modules" || !info.IsDir() || !scan.HasLockfile(filepath.Dir(path)) {
			return denied
		}
	case m.SourceBackup:
		if !info.Mode().IsRegular() || !scan.BackupName(filepath.Base(path)) || strings.Contains("/"+rel, "/.git/") {
			return denied
		}
	case m.SourceLargeFile:
		if !info.Mode().IsRegular() || info.Size() < maxLargeFile/2 || strings.Contains("/"+rel, "/.git/") {
			return denied
		}
	default:
		return denied
	}
	return nil
}

// inspectTree devolve a alteração mais recente e recusa árvores que cruzam dispositivos:
// a varredura não mede montagens nem subvolumes internos, então a limpeza não pode apagá-los.
func inspectTree(path string) (time.Time, error) {
	var newest time.Time
	root, err := os.Lstat(path)
	if err != nil {
		return newest, fmt.Errorf("Não foi possível ler o caminho: %v", err)
	}
	dev, err := deviceOf(root)
	if err != nil {
		return newest, err
	}
	err = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if d.IsDir() {
			other, err := deviceOf(info)
			if err != nil {
				return err
			}
			if other != dev {
				return errors.New("Há ponto de montagem ou subvolume neste caminho.")
			}
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest, err
}

func deviceOf(info os.FileInfo) (uint64, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("Não foi possível identificar o dispositivo do caminho.")
	}
	return uint64(st.Dev), nil
}
