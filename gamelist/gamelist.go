// Package gamelist updates EmulationStation's gamelist.xml files: it gives
// games Tallgrass recognises a proper name and description, keeping every
// other game and every other field (play counts, ratings, artwork paths)
// as it was.
package gamelist

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Game is one entry to apply. Path is relative to the system's ROM folder,
// as EmulationStation writes it ("./Game.gba").
type Game struct {
	Path string
	Name string
	Desc string
}

type gameList struct {
	XMLName xml.Name  `xml:"gameList"`
	Games   []entry   `xml:"game"`
	Other   []element `xml:",any"` // anything besides games (folders, …)
}

type entry struct {
	Attrs []xml.Attr `xml:",any,attr"`
	Path  string     `xml:"path"`
	Name  string     `xml:"name"`
	Desc  string     `xml:"desc,omitempty"`
	Other []element  `xml:",any"` // every other field, kept as is
}

type element struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Inner   []byte     `xml:",innerxml"`
}

// Result says what Apply changed.
type Result struct {
	File    string   `json:"file"`
	Renamed []string `json:"renamed"` // new names
	Added   []string `json:"added"`   // games that weren't listed yet
}

// Apply updates a system's gamelist.xml (created if missing): a listed
// game gets the given name when it still has EmulationStation's default
// (its file name) or no name, and the description when it has none. Games
// not listed are added. The first time it changes a file, the original is
// kept next to it as gamelist.xml.bak-tallgrass.
func Apply(file string, games []Game) (Result, error) {
	res := Result{File: file, Renamed: []string{}, Added: []string{}}
	var list gameList
	orig, err := os.ReadFile(file)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return res, err
	default:
		if err := xml.Unmarshal(orig, &list); err != nil {
			return res, fmt.Errorf("%s isn't a gamelist we can read: %w", file, err)
		}
	}

	byPath := map[string]int{}
	for i, g := range list.Games {
		byPath[cleanPath(g.Path)] = i
	}
	changed := false
	for _, g := range games {
		i, ok := byPath[cleanPath(g.Path)]
		if !ok {
			list.Games = append(list.Games, entry{Path: g.Path, Name: g.Name, Desc: g.Desc})
			res.Added = append(res.Added, g.Name)
			changed = true
			continue
		}
		e := &list.Games[i]
		if e.Name == "" || e.Name == defaultName(e.Path) {
			if e.Name != g.Name {
				e.Name = g.Name
				res.Renamed = append(res.Renamed, g.Name)
				changed = true
			}
		}
		if e.Desc == "" && g.Desc != "" {
			e.Desc = g.Desc
			changed = true
		}
	}
	if !changed {
		return res, nil
	}

	out, err := xml.MarshalIndent(list, "", "\t")
	if err != nil {
		return res, err
	}
	out = append([]byte(xml.Header), append(out, '\n')...)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return res, err
	}
	if orig != nil {
		bak := file + ".bak-tallgrass"
		if _, err := os.Stat(bak); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(bak, orig, 0o644); err != nil {
				return res, err
			}
		}
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return res, err
	}
	return res, os.Rename(tmp, file)
}

// defaultName is the name EmulationStation gives a game: its file name
// without the extension.
func defaultName(path string) string {
	base := filepath.Base(cleanPath(path))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func cleanPath(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(p), "./")
}

// Running reports whether EmulationStation is running (it rewrites
// gamelists when it quits, which would undo changes made meanwhile).
func Running(proc string) bool {
	dirs, _ := os.ReadDir(proc)
	for _, d := range dirs {
		if d.Name()[0] < '0' || d.Name()[0] > '9' {
			continue
		}
		cmd, err := os.ReadFile(filepath.Join(proc, d.Name(), "cmdline"))
		if err != nil {
			continue
		}
		exe, _, _ := bytes.Cut(cmd, []byte{0})
		if filepath.Base(string(exe)) == "emulationstation" {
			return true
		}
	}
	return false
}
