package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	m "faxina/internal/model"
)

const (
	minBackupSize    = 10 << 20
	minLargeFileSize = 1 << 30
	largeFileAge     = 90 * 24 * time.Hour
	minOwnerSize     = 50 << 20
	minCacheSize     = 1 << 20
	projectActive    = 30 * 24 * time.Hour
)

// Caches de ferramentas cujo conteúdo exige novo download.
var toolCaches = map[string]bool{
	"paru": true, "yay": true, "ms-playwright": true, "puppeteer": true,
	"huggingface": true, "pip": true, "uv": true, "electron": true, "Cypress": true,
	"pnpm": true, "torch": true, "JetBrains": true,
}

var backupNameRe = regexp.MustCompile(`(?i)(\.(bak|backup|dump|old)$|\.sql\.(gz|zst|xz|bz2)$|\.sql$|[-_.]backup[-_.].*\.(tar|tgz|zip|gz|zst|7z)$)`)

var lockfiles = []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lock", "bun.lockb", "npm-shrinkwrap.json"}

// bucket acumula tamanho e atividade de uma árvore, com segurança entre goroutines.
type bucket struct {
	item   m.Item
	size   atomic.Int64
	newest atomic.Int64
}

func (b *bucket) add(size, mtime int64) {
	b.size.Add(size)
	for {
		cur := b.newest.Load()
		if mtime <= cur || b.newest.CompareAndSwap(cur, mtime) {
			return
		}
	}
}

type fsScan struct {
	ctx      context.Context
	home     string
	gcStack  string
	now      time.Time
	progress func(string)
	steam    map[string]steamGame

	sem     chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	buckets map[string]*bucket
	files   []m.Item
	inodes  map[[2]uint64]bool
	denied  atomic.Int64
	skipped atomic.Int64
	dirs    atomic.Int64
}

// ScanHome percorre a home e devolve itens agrupados. Não segue symlinks nem cruza dispositivos.
func ScanHome(ctx context.Context, home string, now time.Time, progress func(string)) ([]m.Item, []string) {
	if progress == nil {
		progress = func(string) {}
	}
	s := newFSScan(ctx, now, progress)
	s.home = home
	s.gcStack = filepath.Join(home, "meus_projetos/dev/work/gamersclub/gc-stack")
	s.steam = readSteamLibrary(filepath.Join(home, ".local/share/Steam/steamapps"))
	s.walkRoot(home, nil)
	return s.finish()
}

// ScanSystem mede diretórios fora da home como itens protegidos.
func ScanSystem(ctx context.Context, roots []string, now time.Time) ([]m.Item, []string) {
	s := newFSScan(ctx, now, func(string) {})
	for _, root := range roots {
		if _, err := os.Lstat(root); err != nil {
			continue
		}
		b := s.bucketFor("system:"+root, m.Item{
			ID: m.NewID(m.SourceUnknown, root), Source: m.SourceUnknown, Group: m.GroupSystem,
			Name: root, Path: root, Detail: "Diretório do sistema.",
		})
		s.walkRoot(root, b)
	}
	return s.finish()
}

func newFSScan(ctx context.Context, now time.Time, progress func(string)) *fsScan {
	n := runtime.NumCPU() * 2
	if n > 16 {
		n = 16
	}
	return &fsScan{
		ctx: ctx, now: now, progress: progress,
		sem:     make(chan struct{}, n),
		buckets: map[string]*bucket{},
		inodes:  map[[2]uint64]bool{},
	}
}

func (s *fsScan) walkRoot(root string, b *bucket) {
	info, err := os.Lstat(root)
	if err != nil {
		return
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	s.walk(root, uint64(st.Dev), b)
	s.wg.Wait()
}

// walk visita um diretório. Com bucket definido, tudo abaixo pertence a ele.
func (s *fsScan) walk(dir string, dev uint64, claimed *bucket) {
	if s.ctx.Err() != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		s.denied.Add(1)
		return
	}
	if n := s.dirs.Add(1); n%2000 == 0 {
		s.progress(dir)
	}
	var fallback *bucket
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			continue
		}
		if mode.IsDir() {
			if uint64(st.Dev) != dev {
				s.skipped.Add(1)
				continue
			}
			target := claimed
			if target == nil {
				target = s.claim(path, e.Name(), dir)
			}
			s.descend(path, dev, target)
			continue
		}
		if !mode.IsRegular() {
			continue
		}
		size := st.Blocks * 512
		if st.Nlink > 1 && !s.firstLink(uint64(st.Dev), st.Ino) {
			continue
		}
		mtime := info.ModTime()
		if claimed != nil {
			claimed.add(size, mtime.UnixNano())
			continue
		}
		if s.fileItem(path, e.Name(), size, mtime) {
			continue
		}
		if fallback == nil {
			fallback = s.ownerBucket(dir)
		}
		fallback.add(size, mtime.UnixNano())
	}
}

func (s *fsScan) descend(path string, dev uint64, b *bucket) {
	select {
	case s.sem <- struct{}{}:
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() { <-s.sem }()
			s.walk(path, dev, b)
		}()
	default:
		s.walk(path, dev, b)
	}
}

func (s *fsScan) firstLink(dev, ino uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := [2]uint64{dev, ino}
	if s.inodes[key] {
		return false
	}
	s.inodes[key] = true
	return true
}

func (s *fsScan) bucketFor(key string, item m.Item) *bucket {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.buckets[key]; ok {
		return b
	}
	b := &bucket{item: item}
	s.buckets[key] = b
	return b
}

// claim reconhece diretórios com regra própria; nil significa continuar a descida.
func (s *fsScan) claim(path, name, parent string) *bucket {
	if s.home == "" {
		return nil
	}
	rel, err := filepath.Rel(s.home, path)
	if err != nil {
		return nil
	}
	mk := func(src m.Source, g m.Group, display, detail string) *bucket {
		return s.bucketFor(path, m.Item{ID: m.NewID(src, path), Source: src, Group: g, Name: display, Path: path, Detail: detail})
	}
	switch rel {
	case ".local/share/Trash":
		return mk(m.SourceTrash, m.GroupCaches, "Lixeira", "")
	case ".cache/go-build":
		return mk(m.SourceRegenCache, m.GroupCaches, "Cache de build do Go", "")
	case ".npm/_cacache":
		return mk(m.SourceRegenCache, m.GroupCaches, "Cache do npm", "")
	case "go/pkg/mod":
		return mk(m.SourceGoMod, m.GroupDeps, "Módulos Go", "")
	case ".config/Claude/vm_bundles":
		return mk(m.SourceUnknown, m.GroupOther, "VM do app Claude", "Origem e reuso não confirmados; avalie manualmente.")
	}
	if parent == filepath.Join(s.home, ".cache") {
		if toolCaches[name] {
			return mk(m.SourceToolCache, m.GroupCaches, "Cache: "+name, "")
		}
		return mk(m.SourceAppCache, m.GroupCaches, "Cache: "+name, "")
	}
	if parent == s.gcStack && (name == "tmp" || name == "projects" || name == "platform") {
		return mk(m.SourceGCStackData, m.GroupGCStack, "gc-stack/"+name, "")
	}
	if parent == filepath.Join(s.home, ".local/share/Steam/steamapps/common") {
		if game, ok := s.steam[name]; ok {
			b := mk(m.SourceSteamGame, m.GroupGames, game.Name, "Steam, appid "+game.AppID)
			b.item.Facts.LastActivity = game.LastPlayed
			return b
		}
	}
	if parent == filepath.Join(s.home, "Unity/Hub/Editor") {
		return mk(m.SourceUnity, m.GroupGames, "Unity Editor "+name, "")
	}
	if name == "Library" && exists(filepath.Join(parent, "ProjectSettings")) {
		return mk(m.SourceUnity, m.GroupGames, "Cache Unity: "+filepath.Base(parent), "Regenerado ao abrir o projeto.")
	}
	if name == "node_modules" {
		rp, _ := filepath.Rel(s.home, parent)
		b := mk(m.SourceNodeModules, m.GroupDeps, rp, "")
		b.item.Facts.HasLockfile = HasLockfile(parent)
		b.item.Facts.ProjectActive = isProjectActive(parent, s.home, s.now)
		return b
	}
	return nil
}

// fileItem separa backups e arquivos grandes antigos da contagem geral.
func (s *fsScan) fileItem(path, name string, size int64, mtime time.Time) bool {
	if s.home == "" {
		return false
	}
	rel, err := filepath.Rel(s.home, path)
	if err != nil {
		return false
	}
	var src m.Source
	var group m.Group
	switch {
	case size >= minBackupSize && backupNameRe.MatchString(name) && !insideGit(rel):
		src, group = m.SourceBackup, m.GroupBackups
	case size >= minLargeFileSize && s.now.Sub(mtime) > largeFileAge && !hiddenPath(rel):
		src, group = m.SourceLargeFile, m.GroupBackups
	default:
		return false
	}
	it := m.Item{
		ID: m.NewID(src, path), Source: src, Group: group, Name: rel, Path: path,
		SizeBytes: size, Facts: m.Facts{LastActivity: mtime},
	}
	s.mu.Lock()
	s.files = append(s.files, it)
	s.mu.Unlock()
	return true
}

func (s *fsScan) ownerBucket(dir string) *bucket {
	key := ownerKey(s.home, dir)
	path := filepath.Join(s.home, key)
	name := "~/" + key
	if key == "." {
		name = "~ (arquivos soltos)"
		path = s.home
	}
	return s.bucketFor("owner:"+key, m.Item{
		ID: m.NewID(m.SourceUnknown, path), Source: m.SourceUnknown, Group: m.GroupOther, Name: name, Path: path,
	})
}

// ownerKey escolhe a profundidade que deixa o agrupamento útil.
func ownerKey(home, dir string) string {
	rel, err := filepath.Rel(home, dir)
	if err != nil || rel == "." {
		return "."
	}
	parts := strings.Split(rel, "/")
	depth := 1
	switch {
	case parts[0] == ".local" && len(parts) > 1 && parts[1] == "share":
		depth = 3
	case parts[0] == ".config":
		depth = 2
	case parts[0] == "meus_projetos" && len(parts) > 2 && parts[2] == "work":
		depth = 5
	case parts[0] == "meus_projetos":
		depth = 4
	}
	if len(parts) < depth {
		depth = len(parts)
	}
	return filepath.Join(parts[:depth]...)
}

func (s *fsScan) finish() ([]m.Item, []string) {
	s.wg.Wait()
	items := s.files
	var rest, restCount int64
	for _, b := range s.buckets {
		it := b.item
		it.SizeBytes = b.size.Load()
		if n := b.newest.Load(); n > 0 && it.Facts.LastActivity.IsZero() {
			it.Facts.LastActivity = time.Unix(0, n)
		}
		switch {
		case it.SizeBytes == 0:
			continue
		case it.Source == m.SourceUnknown && it.SizeBytes < minOwnerSize && it.Group == m.GroupOther:
			rest += it.SizeBytes
			restCount++
			continue
		case (it.Source == m.SourceAppCache || it.Source == m.SourceToolCache) && it.SizeBytes < minCacheSize:
			rest += it.SizeBytes
			restCount++
			continue
		}
		items = append(items, it)
	}
	if rest > 0 {
		items = append(items, m.Item{
			ID: m.NewID(m.SourceUnknown, "rest"), Source: m.SourceUnknown, Group: m.GroupOther,
			Name: "Demais pastas pequenas", SizeBytes: rest,
			Detail: fmt.Sprintf("%d pastas abaixo do limite de exibição.", restCount),
		})
	}
	var warnings []string
	if n := s.denied.Load(); n > 0 {
		warnings = append(warnings, fmt.Sprintf("%d diretórios sem permissão de leitura não entraram na conta.", n))
	}
	if n := s.skipped.Load(); n > 0 {
		warnings = append(warnings, fmt.Sprintf("%d pontos de montagem ou subvolumes foram ignorados.", n))
	}
	return items, warnings
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// HasLockfile diz se o projeto tem como restaurar as dependências.
func HasLockfile(dir string) bool {
	for _, name := range lockfiles {
		if exists(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

// isProjectActive olha os arquivos do projeto e o índice do git mais próximo.
func isProjectActive(dir, home string, now time.Time) bool {
	recent := func(t time.Time) bool { return now.Sub(t) < projectActive }
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true // sem como confirmar, assume ativo
	}
	for _, e := range entries {
		if e.Name() == "node_modules" {
			continue
		}
		if info, err := e.Info(); err == nil && recent(info.ModTime()) {
			return true
		}
	}
	for d := dir; strings.HasPrefix(d, home) && d != home; d = filepath.Dir(d) {
		if info, err := os.Stat(filepath.Join(d, ".git/index")); err == nil {
			return recent(info.ModTime())
		}
	}
	return false
}

func hiddenPath(rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// BackupName diz se o nome do arquivo parece um backup.
func BackupName(name string) bool { return backupNameRe.MatchString(name) }

func insideGit(rel string) bool {
	return strings.Contains("/"+rel, "/.git/")
}

// steamGame descreve um jogo instalado.
type steamGame struct {
	AppID, Name string
	LastPlayed  time.Time
}

var acfFieldRe = regexp.MustCompile(`"(appid|name|installdir|LastPlayed)"\s+"([^"]*)"`)

// readSteamLibrary lê os manifestos, indexados pela pasta de instalação.
func readSteamLibrary(steamapps string) map[string]steamGame {
	games := map[string]steamGame{}
	paths, _ := filepath.Glob(filepath.Join(steamapps, "appmanifest_*.acf"))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var game steamGame
		var dir string
		for _, match := range acfFieldRe.FindAllStringSubmatch(string(data), -1) {
			switch match[1] {
			case "appid":
				game.AppID = match[2]
			case "name":
				game.Name = match[2]
			case "installdir":
				dir = match[2]
			case "LastPlayed":
				if ts, err := strconv.ParseInt(match[2], 10, 64); err == nil && ts > 0 {
					game.LastPlayed = time.Unix(ts, 0)
				}
			}
		}
		if dir != "" {
			if game.Name == "" {
				game.Name = dir
			}
			games[dir] = game
		}
	}
	return games
}
