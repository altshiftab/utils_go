package interfaces

type Signer interface {
	Sign(message []byte) (signature []byte, err error)
}

type NamedSigner interface {
	Signer
	GetName() string
}

type Verifier interface {
	Verify(message []byte, signature []byte) error
}

type NamedVerifier interface {
	Verifier
	GetName() string
}

type Method interface {
	Signer
	Verifier
	GetName() string
}
