package asymmetric_decrypt_request

type Request struct {
	// Ciphertext is base64-encoded.
	Ciphertext string `json:"ciphertext"`
	// CiphertextCrc32c is the CRC32C of the ciphertext as a decimal string, which
	// KMS checks before decrypting and reports having checked.
	CiphertextCrc32c string `json:"ciphertextCrc32c,omitzero"`
}
