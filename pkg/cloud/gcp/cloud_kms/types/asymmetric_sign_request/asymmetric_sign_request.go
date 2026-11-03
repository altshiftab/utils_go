package asymmetric_sign_request

// Digest is the digest of what is signed, base64-encoded, under the name of the
// algorithm that produced it. It must be the one the key version's algorithm
// names; a key that signs with SHA-256 refuses a SHA-384 digest.
type Digest struct {
	Sha256 string `json:"sha256,omitzero"`
}

type Request struct {
	Digest *Digest `json:"digest,omitzero"`
}
