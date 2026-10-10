package saves

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrGameRunning is returned by Restore while that save's game is open,
// because the emulator rewrites the save file from memory when it exits.
var ErrGameRunning = errors.New("close this game first: the emulator would overwrite the restored save when it exits")

// Source is one system's folders. RetroArch keeps saves next to the ROMs;
// DraStic keeps them in its own backup folder.
type Source struct {
	System  string // e.g. "gba", "nds"; also the first part of save paths
	SaveDir string
	ROMDir  string
}

// SaveFile is a watched save and its backups.
type SaveFile struct {
	Path     string    `json:"path"` // "<system>/<file name>", e.g. "gba/Game.srm"
	System   string    `json:"system"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Versions []Version `json:"versions"` // newest first
}

type fileStat struct {
	size int64
	mod  time.Time
}

// Watcher polls save folders and backs up each new version.
type Watcher struct {
	sources map[string]Source // by system
	exts    []string
	backups *Backups

	mu       sync.Mutex
	known    map[string]bool     // relative paths of save files seen
	pending  map[string]fileStat // changed on the last scan, not yet stable
	backedUp map[string]fileStat // stat at the last backup (or check)
}

func NewWatcher(sources []Source, backups *Backups) *Watcher {
	w := &Watcher{
		sources: map[string]Source{}, exts: []string{".srm", ".sav", ".dsv"}, backups: backups,
		known: map[string]bool{}, pending: map[string]fileStat{}, backedUp: map[string]fileStat{},
	}
	for _, src := range sources {
		w.sources[src.System] = src
	}
	return w
}

// RetroPieSources is the standard RetroPie layout under a ROMs folder, plus
// DraStic's save folder for DS games.
func RetroPieSources(roms, drasticSaves string) []Source {
	var sources []Source
	for _, sys := range []string{"gb", "gbc", "gba"} {
		dir := filepath.Join(roms, sys)
		sources = append(sources, Source{System: sys, SaveDir: dir, ROMDir: dir})
	}
	return append(sources, Source{System: "nds", SaveDir: drasticSaves, ROMDir: filepath.Join(roms, "nds")})
}

// Run scans every interval until the process exits.
func (w *Watcher) Run(interval time.Duration) {
	for {
		if err := w.Scan(); err != nil {
			log.Printf("save watcher: %v", err)
		}
		time.Sleep(interval)
	}
}

// Scan checks every save file once. A changed file is backed up once its
// size and modification time match on two scans in a row, so we never copy
// a save while it's being written.
func (w *Watcher) Scan() error {
	stats, err := w.stat()
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	for rel, st := range stats {
		w.known[rel] = true
		if w.backedUp[rel] == st {
			delete(w.pending, rel)
			continue
		}
		if prev, ok := w.pending[rel]; !ok || prev != st {
			w.pending[rel] = st
			continue
		}
		delete(w.pending, rel)

		data, err := os.ReadFile(w.abs(rel))
		if err != nil {
			log.Printf("save watcher: %v", err)
			continue
		}
		saved, err := w.backups.Save(rel, data, st.mod)
		if err != nil {
			log.Printf("save watcher: backing up %s: %v", rel, err)
			continue
		}
		w.backedUp[rel] = st
		if saved {
			log.Printf("backed up %s", rel)
		}
	}
	return nil
}

// abs is where a save path ("nds/Game.dsv") lives on disk.
func (w *Watcher) abs(rel string) string {
	sys, name, _ := strings.Cut(rel, "/")
	return filepath.Join(w.sources[sys].SaveDir, name)
}

func (w *Watcher) stat() (map[string]fileStat, error) {
	stats := map[string]fileStat{}
	for sys, src := range w.sources {
		entries, err := os.ReadDir(src.SaveDir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !w.isSave(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue // deleted since ReadDir
			}
			stats[path.Join(sys, e.Name())] = fileStat{info.Size(), info.ModTime()}
		}
	}
	return stats, nil
}

func (w *Watcher) isSave(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range w.exts {
		if ext == e {
			return true
		}
	}
	return false
}

// Files lists the watched saves with their versions, most recently
// modified first.
func (w *Watcher) Files() ([]SaveFile, error) {
	stats, err := w.stat()
	if err != nil {
		return nil, err
	}
	files := []SaveFile{}
	for rel, st := range stats {
		versions, err := w.backups.Versions(rel)
		if err != nil {
			return nil, err
		}
		if versions == nil {
			versions = []Version{}
		}
		files = append(files, SaveFile{
			Path: rel, System: path.Dir(rel), Name: path.Base(rel),
			Size: st.size, Modified: st.mod, Versions: versions,
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Modified.After(files[j].Modified) })
	return files, nil
}

// Read returns a backed-up version of a watched save.
func (w *Watcher) Read(rel, id string) ([]byte, error) {
	if !w.isKnown(rel) {
		return nil, fmt.Errorf("unknown save %q", rel)
	}
	return w.backups.Read(rel, id)
}

func (w *Watcher) isKnown(rel string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.known[rel]
}

// Restore replaces a save file with one of its versions. The current file
// is backed up first, so a restore can itself be undone.
func (w *Watcher) Restore(rel, id string, gameRunning bool) error {
	if gameRunning {
		return ErrGameRunning
	}
	data, err := w.Read(rel, id)
	if err != nil {
		return err
	}
	if err := w.replace(rel, data); err != nil {
		return err
	}
	log.Printf("restored %s to %s", rel, id)
	return nil
}

// Replace overwrites a save file with new contents (an edited save coming
// back from a save editor, say), backing up the current file first.
func (w *Watcher) Replace(rel string, data []byte, gameRunning bool) error {
	if gameRunning {
		return ErrGameRunning
	}
	if !w.isKnown(rel) {
		return fmt.Errorf("unknown save %q", rel)
	}
	if err := w.replace(rel, data); err != nil {
		return err
	}
	log.Printf("replaced %s with an uploaded save (%d bytes)", rel, len(data))
	return nil
}

func (w *Watcher) replace(rel string, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	current, err := os.ReadFile(w.abs(rel))
	if err != nil {
		return err
	}
	if _, err := w.backups.Save(rel, current, time.Now()); err != nil {
		return fmt.Errorf("backing up the current save first: %w", err)
	}
	if err := writeAtomic(w.abs(rel), data); err != nil {
		return err
	}
	return os.Chmod(w.abs(rel), 0o644)
}

var romExts = []string{".gba", ".gbc", ".gb", ".nds"}

// IsROM reports whether a file name has a ROM extension we handle.
func IsROM(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, r := range romExts {
		if ext == r {
			return true
		}
	}
	return false
}

// ROMFor finds a watched save's ROM: same name in the system's ROM folder,
// as emulators name saves after their ROM.
func (w *Watcher) ROMFor(rel string) (string, error) {
	if !w.isKnown(rel) {
		return "", fmt.Errorf("unknown save %q", rel)
	}
	sys, name, _ := strings.Cut(rel, "/")
	stem := filepath.Join(w.sources[sys].ROMDir, strings.TrimSuffix(name, filepath.Ext(name)))
	for _, ext := range romExts {
		if _, err := os.Stat(stem + ext); err == nil {
			return stem + ext, nil
		}
	}
	return "", fmt.Errorf("no ROM found for %s", rel)
}

// SaveFor is the reverse of ROMFor: the watched save paths belonging to a
// ROM file (a game can have both .srm and .sav).
func (w *Watcher) SaveFor(rom string) []string {
	stem := strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom))
	var saves []string
	w.mu.Lock()
	defer w.mu.Unlock()
	for rel := range w.known {
		sys, name, _ := strings.Cut(rel, "/")
		if strings.TrimSuffix(name, filepath.Ext(name)) == stem && filepath.Clean(w.sources[sys].ROMDir) == filepath.Dir(rom) {
			saves = append(saves, rel)
		}
	}
	sort.Strings(saves)
	return saves
}

// ReadCurrent returns the save file as it is on disk now.
func (w *Watcher) ReadCurrent(rel string) ([]byte, error) {
	if !w.isKnown(rel) {
		return nil, fmt.Errorf("unknown save %q", rel)
	}
	return os.ReadFile(w.abs(rel))
}

// ROMFile is a ROM in one of the watched systems.
type ROMFile struct {
	System string
	Path   string
}

// ROMs lists the ROM files in the watched systems' ROM folders.
func (w *Watcher) ROMs() []ROMFile {
	var roms []ROMFile
	for sys, src := range w.sources {
		entries, _ := os.ReadDir(src.ROMDir)
		for _, e := range entries {
			if !e.IsDir() && IsROM(e.Name()) {
				roms = append(roms, ROMFile{System: sys, Path: filepath.Join(src.ROMDir, e.Name())})
			}
		}
	}
	sort.Slice(roms, func(i, j int) bool { return roms[i].Path < roms[j].Path })
	return roms
}
