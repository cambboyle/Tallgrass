// Package catalog is the list of romhacks Tallgrass knows: who made each one,
// which base game it needs, where its official patch is (we link, never
// host), and the SHA-1 of the build Tallgrass supports, so a player can check
// their patched ROM. The data is hacks.json (CC0); the website's Hacks page
// reads the same file from the repository.
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

//go:embed hacks.json
var data []byte

// Base is an original game a hack is made from. Hashes are only listed when
// they were checked against a real copy.
type Base struct {
	Slug     string   `json:"slug"`
	Name     string   `json:"name"`
	System   string   `json:"system"` // "gba" or "nds"
	GameCode string   `json:"gameCode,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	SHA1     []string `json:"sha1,omitempty"`
	CRC32    string   `json:"crc32,omitempty"`
}

// Link points at the hack's official page.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Hack is one romhack.
type Hack struct {
	Slug     string `json:"slug"` // the game's slug on the website and in the setup wizard
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Author   string `json:"author"`
	System   string `json:"system"`
	Base     string `json:"base"` // a Base slug
	BaseNote string `json:"baseNote,omitempty"`
	Source   Link   `json:"source"`
	Result   struct {
		SHA1 []string `json:"sha1"` // the supported build, once patched
	} `json:"result"`
	Support string `json:"support"` // what Tallgrass does with it
}

// Catalog is the whole file.
type Catalog struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Bases   []Base `json:"bases"`
	Hacks   []Hack `json:"hacks"`
}

// Load parses and checks the built-in catalogue.
func Load() (*Catalog, error) { return Parse(data) }

// Parse reads a catalogue and checks it hangs together.
func Parse(b []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.Schema != "tallgrass.catalog" || c.Version != 1 {
		return nil, fmt.Errorf("not a Tallgrass catalogue (schema %q version %d)", c.Schema, c.Version)
	}
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	crc := regexp.MustCompile(`^[0-9A-F]{8}$`)
	bases := map[string]Base{}
	for _, base := range c.Bases {
		if base.Slug == "" || base.Name == "" || (base.System != "gba" && base.System != "nds") {
			return nil, fmt.Errorf("base %+v needs a slug, name and system", base)
		}
		if _, dup := bases[base.Slug]; dup {
			return nil, fmt.Errorf("base %s is listed twice", base.Slug)
		}
		bases[base.Slug] = base
		for _, s := range base.SHA1 {
			if !sha.MatchString(s) {
				return nil, fmt.Errorf("base %s: %q isn't a lower-case SHA-1", base.Slug, s)
			}
		}
		if base.CRC32 != "" && !crc.MatchString(base.CRC32) {
			return nil, fmt.Errorf("base %s: %q isn't an upper-case CRC32", base.Slug, base.CRC32)
		}
	}
	seen := map[string]bool{}
	for _, h := range c.Hacks {
		if h.Slug == "" || h.Name == "" || h.Author == "" {
			return nil, fmt.Errorf("hack %+v needs a slug, name and author", h)
		}
		if seen[h.Slug] {
			return nil, fmt.Errorf("hack %s is listed twice", h.Slug)
		}
		seen[h.Slug] = true
		base, ok := bases[h.Base]
		if !ok {
			return nil, fmt.Errorf("hack %s: unknown base %q", h.Slug, h.Base)
		}
		if base.System != h.System {
			return nil, fmt.Errorf("hack %s is for %s but its base %s is for %s", h.Slug, h.System, base.Slug, base.System)
		}
		if u, err := url.Parse(h.Source.URL); err != nil || u.Scheme != "https" || u.Host == "" || h.Source.Label == "" {
			return nil, fmt.Errorf("hack %s: source needs a label and an https link", h.Slug)
		}
		if len(h.Result.SHA1) == 0 {
			return nil, fmt.Errorf("hack %s: no result SHA-1", h.Slug)
		}
		for _, s := range h.Result.SHA1 {
			if !sha.MatchString(s) {
				return nil, fmt.Errorf("hack %s: %q isn't a lower-case SHA-1", h.Slug, s)
			}
		}
	}
	return &c, nil
}

// Hack returns the hack with this slug, or nil.
func (c *Catalog) Hack(slug string) *Hack {
	for i := range c.Hacks {
		if c.Hacks[i].Slug == slug {
			return &c.Hacks[i]
		}
	}
	return nil
}

// BaseOf returns the base game a hack is made from, or nil.
func (c *Catalog) BaseOf(h *Hack) *Base {
	for i := range c.Bases {
		if c.Bases[i].Slug == h.Base {
			return &c.Bases[i]
		}
	}
	return nil
}

// HackBySHA1 finds the hack whose supported build has this SHA-1, or nil.
func (c *Catalog) HackBySHA1(sum string) *Hack {
	sum = strings.ToLower(sum)
	for i := range c.Hacks {
		for _, s := range c.Hacks[i].Result.SHA1 {
			if s == sum {
				return &c.Hacks[i]
			}
		}
	}
	return nil
}

// HacksFor lists the hacks made from a base game.
func (c *Catalog) HacksFor(base string) []Hack {
	var out []Hack
	for _, h := range c.Hacks {
		if h.Base == base {
			out = append(out, h)
		}
	}
	return out
}
