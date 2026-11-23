// Signs a release artifact with the ed25519 key in RELEASE_SIGNING_KEY
// (hex seed). Invoked by GoReleaser to sign the checksum manifest. With
// -version the signature also covers the release version, which is what
// current clients verify.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"log"
	"os"

	"github.com/ugzv/ublockdnsclient/internal/update"
)

func main() {
	in := flag.String("in", "", "file to sign")
	out := flag.String("out", "", "signature output file")
	version := flag.String("version", "", "release version to bind the signature to")
	flag.Parse()
	if *in == "" || *out == "" {
		log.Fatal("usage: sign -in <file> -out <sig>")
	}

	seed, err := hex.DecodeString(os.Getenv("RELEASE_SIGNING_KEY"))
	if err != nil || len(seed) != ed25519.SeedSize {
		log.Fatal("RELEASE_SIGNING_KEY must be a hex-encoded ed25519 seed")
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		log.Fatal(err)
	}
	if *version != "" {
		data = update.VersionedManifest(*version, data)
	}
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), data)
	if err := os.WriteFile(*out, []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		log.Fatal(err)
	}
}
