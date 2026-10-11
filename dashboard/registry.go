package dashboard

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"tallgrass/cloud"
	"tallgrass/profile"
)

// The profile registry: profiles published from the repo (profiles/*.json)
// that the agent downloads, so new and fixed profiles reach players without
// a new release. Priority: local profiles folder, then the registry, then
// the profiles built into this binary. The last download is kept on disk,
// so the registry's profiles also work offline and from the first poll.

// SetProfiles replaces the profile list and makes everything that depends
// on a profile pick again (the running game on the next poll, ROM data on
// next use).
func (s *Server) SetProfiles(all []*profile.Profile) {
	s.mu.Lock()
	s.profiles = all
	s.running = nil // the poller re-picks the running game's profile
	s.mu.Unlock()
	s.romCache.mu.Lock()
	s.romCache.byID = nil
	s.romCache.mu.Unlock()
}

func (s *Server) profileList() []*profile.Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profiles
}

// EnableRegistry adds registry profiles in front of bundled ones (behind
// local ones), from cacheFile at once and from the registry now and every
// 6 hours. version is this build's, to skip profiles needing newer code.
func (s *Server) EnableRegistry(c *cloud.Client, cacheFile, version string, local, bundled []*profile.Profile) {
	apply := func(entries []cloud.GameProfile) int {
		var reg []*profile.Profile
		for _, e := range entries {
			p, err := profile.Parse(e.Profile, "registry profile "+e.Slug)
			if err != nil {
				log.Printf("skipping %v", err)
				continue
			}
			if !p.Supports(version) {
				log.Printf("skipping registry profile %s: needs Tallgrass %s or newer", e.Slug, p.MinTallgrass)
				continue
			}
			reg = append(reg, p)
		}
		all := append(append(append([]*profile.Profile{}, local...), reg...), bundled...)
		s.SetProfiles(all)
		return len(reg)
	}
	var cached []cloud.GameProfile
	if b, err := os.ReadFile(cacheFile); err == nil && json.Unmarshal(b, &cached) == nil {
		apply(cached)
	}
	go func() {
		for {
			entries, err := c.GameProfiles()
			switch {
			case err != nil:
				log.Printf("profile registry: %v", err)
			case !sameEntries(entries, cached):
				n := apply(entries)
				log.Printf("profile registry: using %d profiles", n)
				if b, err := json.Marshal(entries); err == nil {
					if err := os.WriteFile(cacheFile, b, 0o644); err != nil {
						log.Printf("saving the profile registry: %v", err)
					}
				}
				cached = entries
			}
			time.Sleep(6 * time.Hour)
		}
	}()
}

// sameEntries reports whether two registry downloads have the same
// profiles at the same versions.
func sameEntries(a, b []cloud.GameProfile) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, e := range a {
		seen[e.Slug] = e.Version
	}
	for _, e := range b {
		if v, ok := seen[e.Slug]; !ok || v != e.Version {
			return false
		}
	}
	return true
}
