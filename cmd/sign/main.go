// sign makes and uses the release signing key.
//
//	go run ./cmd/sign -genkey ~/.config/tallgrass/release.key   # prints the public key
//	go run ./cmd/sign -key release.key SHA256SUMS > SHA256SUMS.sig
//
// The key file holds a base64 Ed25519 seed. In CI it comes from the
// RELEASE_SIGNING_KEY secret (-key-env RELEASE_SIGNING_KEY).
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	genkey := flag.String("genkey", "", "write a new private key to this file and print its public key")
	keyFile := flag.String("key", "", "private key file")
	keyEnv := flag.String("key-env", "", "environment variable holding the private key")
	flag.Parse()

	if *genkey != "" {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*genkey, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
			log.Fatal(err)
		}
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
		return
	}

	var encoded string
	switch {
	case *keyEnv != "":
		encoded = os.Getenv(*keyEnv)
	case *keyFile != "":
		b, err := os.ReadFile(*keyFile)
		if err != nil {
			log.Fatal(err)
		}
		encoded = string(b)
	default:
		log.Fatal("pass -key or -key-env")
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(seed) != ed25519.SeedSize {
		log.Fatal("the key isn't a base64 Ed25519 seed")
	}
	if flag.NArg() != 1 {
		log.Fatal("give the file to sign")
	}
	msg, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed), msg)))
}
