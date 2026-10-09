// Package saves watches emulator save files and keeps a gzipped copy of
// every distinct version, as plain files you can restore by hand:
//
//	<backups>/<system>/<save file name>/<UTC time>-<sha256 prefix><ext>.gz
package saves

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	stampFormat = "20060102T150405Z"
	hashLen     = 12 // hex chars of SHA-256 kept in file names
)

// Version is one backed-up copy of a save file.
type Version struct {
	ID   string    `json:"id"` // backup file name
	At   time.Time `json:"at"`
	Hash string    `json:"hash"`
	Size int64     `json:"size"` // compressed size on disk
}

// Backups is the backup folder.
type Backups struct {
	dir string
}

func NewBackups(dir string) *Backups { return &Backups{dir: dir} }

// folder holds the versions of the save at rel ("gba/Game.srm").
func (b *Backups) folder(rel string) string {
	return filepath.Join(b.dir, filepath.FromSlash(rel))
}

// Versions lists a save's backups, newest first.
func (b *Backups) Versions(rel string) ([]Version, error) {
	entries, err := os.ReadDir(b.folder(rel))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var versions []Version
	for _, e := range entries {
		v, ok := parseVersion(e.Name())
		if !ok {
			continue
		}
		if info, err := e.Info(); err == nil {
			v.Size = info.Size()
		}
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].ID > versions[j].ID })
	return versions, nil
}

// parseVersion reads "20261009T142530Z-0123456789ab.srm.gz".
func parseVersion(name string) (Version, bool) {
	if !strings.HasSuffix(name, ".gz") || len(name) < len(stampFormat)+1+hashLen {
		return Version{}, false
	}
	at, err := time.Parse(stampFormat, name[:len(stampFormat)])
	if err != nil || name[len(stampFormat)] != '-' {
		return Version{}, false
	}
	hash := name[len(stampFormat)+1 : len(stampFormat)+1+hashLen]
	return Version{ID: name, At: at, Hash: hash}, true
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:hashLen]
}

// Save stores data as a new version of rel unless it matches the newest
// one. It reports whether a version was written.
func (b *Backups) Save(rel string, data []byte, at time.Time) (bool, error) {
	hash := hashOf(data)
	versions, err := b.Versions(rel)
	if err != nil {
		return false, err
	}
	if len(versions) > 0 && versions[0].Hash == hash {
		return false, nil
	}

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Name = filepath.Base(rel)
	zw.ModTime = at
	if _, err := zw.Write(data); err != nil {
		return false, err
	}
	if err := zw.Close(); err != nil {
		return false, err
	}

	folder := b.folder(rel)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return false, err
	}
	name := fmt.Sprintf("%s-%s%s.gz", at.UTC().Format(stampFormat), hash, filepath.Ext(rel))
	return true, writeAtomic(filepath.Join(folder, name), buf.Bytes())
}

// Read returns the uncompressed contents of one version.
func (b *Backups) Read(rel, id string) ([]byte, error) {
	if _, ok := parseVersion(id); !ok || filepath.Base(id) != id {
		return nil, fmt.Errorf("bad version %q", id)
	}
	f, err := os.Open(filepath.Join(b.folder(rel), id))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}

// writeAtomic writes via a temp file and rename, so a crash or power cut
// never leaves a half-written file behind.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
