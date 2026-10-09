package gen4

import (
	"os"
	"testing"
)

// hgg-location.bin is bytes 0x1400–0x14FF of a live HeartGold Generations
// general block, taken just after walking from Cherrygrove City back onto
// Route 30. The door records still say Cherrygrove; the current one doesn't.
func TestFindLocationLive(t *testing.T) {
	b, err := os.ReadFile("testdata/hgg-location.bin")
	if err != nil {
		t.Fatal(err)
	}
	if o := FindLocation(b); o != 0x24 {
		t.Fatalf("FindLocation = %#x, want 0x24", o)
	}
	if mp := le.Uint32(b[0x24:]); mp != 34 {
		t.Errorf("current map = %d, want 34 (Route 30)", mp)
	}
}
