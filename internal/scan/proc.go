package scan

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// OpenPaths guarda os caminhos abertos por processos visíveis ao usuário.
type OpenPaths struct{ paths []string }

// CollectOpenPaths lê descritores, diretório de trabalho e mapas de memória em /proc.
func CollectOpenPaths(procRoot string) OpenPaths {
	set := map[string]bool{}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return OpenPaths{}
	}
	self := strconv.Itoa(os.Getpid())
	add := func(p string) {
		p = strings.TrimSuffix(p, " (deleted)")
		if strings.HasPrefix(p, "/") {
			set[p] = true
		}
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil || e.Name() == self {
			continue
		}
		base := filepath.Join(procRoot, e.Name())
		if cwd, err := os.Readlink(filepath.Join(base, "cwd")); err == nil {
			add(cwd)
		}
		fds, err := os.ReadDir(filepath.Join(base, "fd"))
		if err == nil {
			for _, fd := range fds {
				if target, err := os.Readlink(filepath.Join(base, "fd", fd.Name())); err == nil {
					add(target)
				}
			}
		}
		if maps, err := os.ReadFile(filepath.Join(base, "maps")); err == nil {
			for _, line := range strings.Split(string(maps), "\n") {
				if i := strings.IndexByte(line, '/'); i >= 0 {
					add(line[i:])
				}
			}
		}
	}
	out := OpenPaths{paths: make([]string, 0, len(set))}
	for p := range set {
		out.paths = append(out.paths, p)
	}
	sort.Strings(out.paths)
	return out
}

// Contains diz se o caminho, ou algo dentro dele, está aberto.
func (o OpenPaths) Contains(path string) bool {
	i := sort.SearchStrings(o.paths, path)
	if i >= len(o.paths) {
		return false
	}
	return o.paths[i] == path || strings.HasPrefix(o.paths[i], path+"/") ||
		containsChild(o.paths[i:], path)
}

// containsChild cobre nomes que ordenam entre "path" e "path/" (como "path-x").
func containsChild(paths []string, path string) bool {
	i := sort.SearchStrings(paths, path+"/")
	return i < len(paths) && strings.HasPrefix(paths[i], path+"/")
}
