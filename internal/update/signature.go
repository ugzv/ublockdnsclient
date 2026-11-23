package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// Public half of the release signing key; the private half lives only in CI
// secrets. Updates are refused unless the checksum manifest is signed by it.
const releasePublicKeyHex = "a84f76066f0ae0f5d352f7600e78e0755776bfb920190d0f965618afaa46f182"

var releasePublicKey = mustDecodePublicKey(releasePublicKeyHex)

func mustDecodePublicKey(h string) ed25519.PublicKey {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic("invalid release public key")
	}
	return ed25519.PublicKey(b)
}

// manifestSignatureName is the release asset holding the signature over
// VersionedManifest. "SHA256SUMS.sig", over the bare manifest, is still
// published for clients released before the version was signed.
const manifestSignatureName = "SHA256SUMS.vsig"

// VersionedManifest is the signed payload: the checksum manifest prefixed with
// the release it belongs to. Asset names repeat across releases, so a
// signature over the manifest alone would stay valid under any other tag.
func VersionedManifest(version string, sums []byte) []byte {
	return append([]byte("ublockdns v"+strings.TrimPrefix(version, "v")+"\n"), sums...)
}

func verifySignature(payload, sigB64 []byte) error {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil {
		return err
	}
	if len(sig) != ed25519.SignatureSize || !ed25519.Verify(releasePublicKey, payload, sig) {
		return errors.New("invalid signature")
	}
	return nil
}
