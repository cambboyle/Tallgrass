package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"0.2.0", "0.1.9", true}, {"1.2.10", "1.2.9", true}, {"0.1.0", "0.1.0", false}, {"0.1.0", "0.2.0", false}, {"9.9.9", "dev", false}} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	sums := []byte("abc123  tallgrass-linux-armv7\n")
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sums))
	if _, err := Verify(sums, []byte(sig), "tallgrass-linux-armv7"); err == nil {
		t.Error("a signature from another key was accepted")
	}
}

// Apply installs a signed release and refuses a tampered one.
func TestApply(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	old := PublicKey
	PublicKey = base64.StdEncoding.EncodeToString(pub)
	defer func() { PublicKey = old }()

	name := AssetName()
	bin := []byte("new tallgrass binary")
	sum := sha256.Sum256(bin)
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sums))
	files := map[string][]byte{"/" + name: bin, "/SHA256SUMS": sums, "/SHA256SUMS.sig": []byte(sig)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(files[r.URL.Path]) }))
	defer srv.Close()
	rel := &Release{Version: "9.9.9", assets: map[string]string{
		name: srv.URL + "/" + name, "SHA256SUMS": srv.URL + "/SHA256SUMS", "SHA256SUMS.sig": srv.URL + "/SHA256SUMS.sig"}}

	exe := filepath.Join(t.TempDir(), "tallgrass")
	os.WriteFile(exe, []byte("old"), 0o755)
	if err := Apply(rel, exe); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != string(bin) {
		t.Fatalf("installed %q", got)
	}

	files["/"+name] = []byte("tampered")
	if err := Apply(rel, exe); err == nil {
		t.Error("a binary that doesn't match SHA256SUMS was installed")
	}
	files["/"+name] = bin
	files["/SHA256SUMS.sig"] = []byte(base64.StdEncoding.EncodeToString(make([]byte, 64)))
	if err := Apply(rel, exe); err == nil {
		t.Error("a bad signature was accepted")
	}
}
