// Package model define os tipos compartilhados entre coletores, regras, limpeza e UI.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Status diz o que pode ser feito com um item.
type Status string

const (
	StatusInUse     Status = "in_use"
	StatusProtected Status = "protected"
	StatusCandidate Status = "candidate"
	StatusRemovable Status = "removable"
)

// Group agrupa itens na UI.
type Group string

const (
	GroupVolumes    Group = "docker_volumes"
	GroupImages     Group = "docker_images"
	GroupBuildCache Group = "docker_build_cache"
	GroupCaches     Group = "caches"
	GroupDeps       Group = "dependencies"
	GroupSystem     Group = "system"
	GroupGames      Group = "games_editors"
	GroupBackups    Group = "backups"
	GroupGCStack    Group = "gc_stack"
	GroupOther      Group = "other"
)

// Source identifica o tipo do fato coletado; as regras decidem a partir dele.
type Source string

const (
	SourceVolume         Source = "docker_volume"
	SourceImage          Source = "docker_image"
	SourceBuildCacheUsed Source = "build_cache_in_use"
	SourceBuildCacheOld  Source = "build_cache_old"
	SourceBuildCacheNew  Source = "build_cache_recent"
	SourceTrash          Source = "trash"
	SourceRegenCache     Source = "regenerable_cache"
	SourceAppCache       Source = "app_cache"
	SourceToolCache      Source = "tool_cache"
	SourceNodeModules    Source = "node_modules"
	SourceGoMod          Source = "go_mod"
	SourceBackup         Source = "backup"
	SourceLargeFile      Source = "large_file"
	SourceSteamGame      Source = "steam_game"
	SourceUnity          Source = "unity"
	SourceGCStackData    Source = "gc_stack_data"
	SourceUnknown        Source = "unknown"
	SourcePacmanCache    Source = "pacman_cache"
	SourceOrphanPkgs     Source = "orphan_packages"
	SourceJournal        Source = "journal"
	SourceSnapshots      Source = "snapshots"
)

// ActionKind é a operação de limpeza. O conjunto é fechado de propósito.
type ActionKind string

const (
	ActionNone            ActionKind = "none"
	ActionVolumeRm        ActionKind = "docker_volume_rm"
	ActionImageRm         ActionKind = "docker_image_rm"
	ActionBuilderPruneOld ActionKind = "docker_builder_prune_old"
	ActionBuilderPruneAll ActionKind = "docker_builder_prune_all"
	ActionRemovePath      ActionKind = "remove_path"
	ActionPaccache        ActionKind = "paccache"
)

// Action descreve a limpeza de um item. Command serve só para exibição.
type Action struct {
	Kind      ActionKind `json:"kind"`
	Target    string     `json:"target"`
	Command   string     `json:"command"`
	NeedsRoot bool       `json:"needsRoot"`
}

// Executable indica se a ação pode ser executada pelo app.
func (a Action) Executable() bool { return a.Kind != "" && a.Kind != ActionNone }

// VolumeContent é o tipo de dado encontrado num volume Docker.
type VolumeContent string

const (
	VolumeUnknown  VolumeContent = ""
	VolumeEmpty    VolumeContent = "empty"
	VolumeRegistry VolumeContent = "registry"
	VolumeMySQL    VolumeContent = "mysql"
	VolumePostgres VolumeContent = "postgres"
	VolumeRedis    VolumeContent = "redis"
	VolumeOther    VolumeContent = "other"
)

// Facts guarda o que os coletores observaram. Não é enviado à UI.
type Facts struct {
	Containers    int
	Anonymous     bool
	Content       VolumeContent
	CreatedAt     time.Time
	LastActivity  time.Time
	Repo          string
	Tag           string
	ImageID       string
	NewestInRepo  bool
	LocalRegistry bool
	Infra         bool
	Deployed      bool
	KubeUnknown   bool
	OpenByProcess bool
	HasLockfile   bool
	ProjectActive bool
	Label         string
	ManualHint    string
}

// Item é uma unidade exibida e, quando permitido, limpável.
type Item struct {
	ID            string `json:"id"`
	Source        Source `json:"source"`
	Group         Group  `json:"group"`
	Name          string `json:"name"`
	Detail        string `json:"detail"`
	Path          string `json:"path"`
	SizeBytes     int64  `json:"sizeBytes"`
	SizeEstimated bool   `json:"sizeEstimated"`
	AgeDays       int    `json:"ageDays"`
	Status        Status `json:"status"`
	Reason        string `json:"reason"`
	ManualHint    string `json:"manualHint"`
	Action        Action `json:"action"`
	Facts         Facts  `json:"-"`
}

// NewID gera um ID estável para a fonte e o alvo.
func NewID(source Source, key string) string {
	sum := sha256.Sum256([]byte(string(source) + "\x00" + key))
	return hex.EncodeToString(sum[:8])
}

// Disk é o uso do sistema de arquivos raiz.
type Disk struct {
	Mount       string  `json:"mount"`
	TotalBytes  int64   `json:"totalBytes"`
	FreeBytes   int64   `json:"freeBytes"`
	UsedBytes   int64   `json:"usedBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

// ScanResult é o resultado completo de uma varredura.
type ScanResult struct {
	ScannedAt  time.Time `json:"scannedAt"`
	DurationMs int64     `json:"durationMs"`
	Disk       Disk      `json:"disk"`
	Items      []Item    `json:"items"`
	Warnings   []string  `json:"warnings"`
}
