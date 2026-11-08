package asymmetric_decrypt_response

type Response struct {
	// Plaintext is base64-encoded.
	Plaintext string `json:"plaintext"`
	// PlaintextCrc32c is the CRC32C of the plaintext as a decimal string.
	PlaintextCrc32c string `json:"plaintextCrc32c,omitzero"`
	// VerifiedCiphertextCrc32c says whether KMS checked the ciphertext against
	// the CRC32C the request carried.
	VerifiedCiphertextCrc32c bool   `json:"verifiedCiphertextCrc32c,omitzero"`
	ProtectionLevel          string `json:"protectionLevel,omitzero"`
}
