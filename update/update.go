// Package update finds new Tallgrass releases on GitHub and installs them.
//
// A release has one binary per platform (tallgrass-linux-armv7 and so on),
// SHA256SUMS listing their hashes, and SHA256SUMS.sig: an Ed25519 signature
// of SHA256SUMS by the release key, whose public half is built in here. An
// update is installed only if the signature and the binary's hash check out.
package update

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// PublicKey verifies releases (the private half lives with the maintainer
// and in the release workflow's RELEASE_SIGNING_KEY secret). A variable only
// so tests can use their own key.
var PublicKey = "Vi+Iyin0vBx2GDbHCLKHGDWLjP6DRODtXxpRh026Ga0="

// Repo is where releases are published.
const Repo = "cambboyle/Tallgrass"

type Release struct {
	Version string `json:"version"` // e.g. "0.2.0"
	Notes   string `json:"notes"`
	URL     string `json:"url"` // the release page
	assets  map[string]string
}

var client = &http.Client{Timeout: 2 * time.Minute}

// Latest asks GitHub for the newest release.
func Latest() (*Release, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+Repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, errors.New("no public releases yet")
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub: HTTP %d", res.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	r := &Release{Version: strings.TrimPrefix(body.TagName, "v"), Notes: body.Body, URL: body.HTMLURL, assets: map[string]string{}}
	for _, a := range body.Assets {
		r.assets[a.Name] = a.URL
	}
	return r, nil
}

// AssetName is this platform's binary in a release.
func AssetName() string {
	arch := runtime.GOARCH
	if arch == "arm" {
		arch = "armv7"
	}
	return "tallgrass-" + runtime.GOOS + "-" + arch
}

// Newer reports whether version a is newer than b ("1.2.10" > "1.2.9").
// A development build ("dev", "dev-<commit>") never has a newer release.
func Newer(a, b string) bool {
	if b == "" || strings.HasPrefix(b, "dev") {
		return false
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func download(url string, limit int64) ([]byte, error) {
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: HTTP %d", filepath.Base(url), res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, limit))
}

// Verify checks a signed SHA256SUMS and returns the hash listed for name.
func Verify(sums, sig []byte, name string) (string, error) {
	pub, err := base64.StdEncoding.DecodeString(PublicKey)
	if err != nil {
		return "", err
	}
	s, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || !ed25519.Verify(pub, sums, s) {
		return "", errors.New("the release's signature doesn't match: not installing it")
	}
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("the release has no %s", name)
}

// Apply downloads this platform's binary from a release, checks it and
// replaces the running executable. The caller then exits so the service
// manager restarts it.
func Apply(r *Release, exe string) error {
	name := AssetName()
	binURL, sumsURL, sigURL := r.assets[name], r.assets["SHA256SUMS"], r.assets["SHA256SUMS.sig"]
	if binURL == "" || sumsURL == "" || sigURL == "" {
		return fmt.Errorf("release %s has no signed build for this device (%s)", r.Version, name)
	}
	sums, err := download(sumsURL, 1<<16)
	if err != nil {
		return err
	}
	sig, err := download(sigURL, 1<<10)
	if err != nil {
		return err
	}
	want, err := Verify(sums, sig, name)
	if err != nil {
		return err
	}
	bin, err := download(binURL, 200<<20)
	if err != nil {
		return err
	}
	got := sha256.Sum256(bin)
	if hex.EncodeToString(got[:]) != want {
		return errors.New("the download doesn't match the release's checksum: not installing it")
	}
	// Write next to the executable and rename over it: Linux keeps the
	// running copy until the process exits.
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, exe)
}
