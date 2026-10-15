package catalog

import (
	"strings"
	"testing"
)

func TestBuiltInCatalogLoads(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Hacks) < 4 || len(c.Bases) < 3 {
		t.Fatalf("%d hacks, %d bases", len(c.Hacks), len(c.Bases))
	}
	rr := c.Hack("radical-red")
	if rr == nil || c.BaseOf(rr) == nil || c.BaseOf(rr).Slug != "firered-us" {
		t.Fatalf("radical-red = %+v", rr)
	}
	if h := c.HackBySHA1("964F951A0FDAF209E4EA1344883EF0D557BB3A80"); h == nil || h.Slug != "radical-red" {
		t.Errorf("lookup by SHA-1 (any case) = %+v", h)
	}
	if c.HackBySHA1("0000000000000000000000000000000000000000") != nil {
		t.Error("an unknown SHA-1 matched a hack")
	}
	if got := c.HacksFor("emerald-us"); len(got) != 1 || got[0].Slug != "rowe" {
		t.Errorf("hacks for Emerald = %+v", got)
	}
}

func TestParseRejectsBadData(t *testing.T) {
	good := `{"schema":"tallgrass.catalog","version":1,
	 "bases":[{"slug":"b","name":"B","system":"gba"}],
	 "hacks":[{"slug":"h","name":"H","author":"A","system":"gba","base":"b",
	  "source":{"label":"L","url":"https://example.com/x"},
	  "result":{"sha1":["964f951a0fdaf209e4ea1344883ef0d557bb3a80"]},"support":"s"}]}`
	if _, err := Parse([]byte(good)); err != nil {
		t.Fatalf("a good catalogue was rejected: %v", err)
	}
	for name, bad := range map[string]string{
		"wrong schema":     strings.Replace(good, "tallgrass.catalog", "other", 1),
		"unknown base":     strings.Replace(good, `"base":"b"`, `"base":"nope"`, 1),
		"system mismatch":  strings.Replace(good, `"system":"gba","base"`, `"system":"nds","base"`, 1),
		"plain http link":  strings.Replace(good, "https://example.com/x", "http://example.com/x", 1),
		"upper-case SHA-1": strings.Replace(good, "964f951a0fdaf209e4ea1344883ef0d557bb3a80", "964F951A0FDAF209E4EA1344883EF0D557BB3A80", 1),
		"short SHA-1":      strings.Replace(good, "964f951a0fdaf209e4ea1344883ef0d557bb3a80", "964f951a", 1),
		"no result":        strings.Replace(good, `"sha1":["964f951a0fdaf209e4ea1344883ef0d557bb3a80"]`, `"sha1":[]`, 1),
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
