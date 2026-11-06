package scan

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	m "faxina/internal/model"
	"faxina/internal/runner"
)

var (
	paccacheSavedRe = regexp.MustCompile(`disk space saved: ([0-9.]+) ([KMGT]?i?B)`)
	pacmanSizeRe    = regexp.MustCompile(`(?m)^Installed Size\s*:\s*([0-9.]+) ([KMGT]?i?B)`)
	journalSizeRe   = regexp.MustCompile(`take up ([0-9.]+)([KMGT]?)`)
)

func collectSystem(ctx context.Context, r runner.Runner) ([]m.Item, []string) {
	var items []m.Item
	var warnings []string

	var saved int64
	paccacheOK := true
	for _, args := range [][]string{{"-dk2"}, {"-duk0"}} {
		out, err := r.Run(ctx, "env", append([]string{"LC_ALL=C", "paccache"}, args...)...)
		if err != nil {
			paccacheOK = false
			warnings = append(warnings, "Não foi possível estimar o cache do pacman: "+err.Error())
			break
		}
		saved += parsePaccacheSaved(out)
	}
	if paccacheOK && saved > 0 {
		items = append(items, m.Item{
			ID: m.NewID(m.SourcePacmanCache, "pkg"), Source: m.SourcePacmanCache, Group: m.GroupSystem,
			Name: "Pacotes antigos do pacman", Path: "/var/cache/pacman/pkg",
			Detail:    "Estimativa do próprio paccache. Pede a senha do sistema ao limpar.",
			SizeBytes: saved, SizeEstimated: true,
		})
	}

	if out, err := r.Run(ctx, "pacman", "-Qdtq"); err == nil {
		if pkgs := strings.Fields(out); len(pkgs) > 0 {
			var size int64
			args := append([]string{"LC_ALL=C", "pacman", "-Qi"}, pkgs...)
			if info, err := r.Run(ctx, "env", args...); err == nil {
				for _, match := range pacmanSizeRe.FindAllStringSubmatch(info, -1) {
					size += parseBinarySize(match[1], match[2])
				}
			}
			items = append(items, m.Item{
				ID: m.NewID(m.SourceOrphanPkgs, "orphans"), Source: m.SourceOrphanPkgs, Group: m.GroupSystem,
				Name:      fmt.Sprintf("%d pacotes órfãos", len(pkgs)),
				Detail:    strings.Join(pkgs, ", "),
				SizeBytes: size, SizeEstimated: true,
				Facts: m.Facts{ManualHint: "sudo pacman -Rns " + strings.Join(pkgs, " ")},
			})
		}
	}

	if out, err := r.Run(ctx, "env", "LC_ALL=C", "journalctl", "--disk-usage"); err == nil {
		if match := journalSizeRe.FindStringSubmatch(out); match != nil {
			items = append(items, m.Item{
				ID: m.NewID(m.SourceJournal, "journal"), Source: m.SourceJournal, Group: m.GroupSystem,
				Name: "Journal do sistema", Path: "/var/log/journal",
				SizeBytes: parseBinarySize(match[1], match[2]+"iB"), SizeEstimated: true,
				Facts: m.Facts{ManualHint: "sudo journalctl --vacuum-size=200M"},
			})
		}
	}

	detail := "Listar e medir snapshots exige root; use o comando sugerido."
	if _, err := os.Stat("/.snapshots"); os.IsNotExist(err) {
		detail = "/.snapshots não existe neste sistema; confira a configuração do snapper com o comando sugerido."
	}
	if out, err := r.Run(ctx, "snapper", "list-configs"); err == nil && strings.Contains(out, "root") {
		items = append(items, m.Item{
			ID: m.NewID(m.SourceSnapshots, "root"), Source: m.SourceSnapshots, Group: m.GroupSystem,
			Name: "Snapshots do snapper", Detail: detail, SizeEstimated: true,
			Facts: m.Facts{ManualHint: "sudo snapper -c root list"},
		})
	}
	return items, warnings
}

func parsePaccacheSaved(out string) int64 {
	match := paccacheSavedRe.FindStringSubmatch(out)
	if match == nil {
		return 0
	}
	return parseBinarySize(match[1], match[2])
}

func parseBinarySize(value, unit string) int64 {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	mult := map[string]float64{"B": 1, "iB": 1, "KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40}[unit]
	return int64(v * mult)
}

// ReadDisk lê o uso do sistema de arquivos, com a mesma conta do df.
func ReadDisk(mount string) (m.Disk, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(mount, &st); err != nil {
		return m.Disk{Mount: mount}, err
	}
	bs := int64(st.Bsize)
	total := int64(st.Blocks) * bs
	free := int64(st.Bavail) * bs
	used := int64(st.Blocks-st.Bfree) * bs
	d := m.Disk{Mount: mount, TotalBytes: total, FreeBytes: free, UsedBytes: used}
	if used+free > 0 {
		d.UsedPercent = float64(used) / float64(used+free) * 100
	}
	return d, nil
}
