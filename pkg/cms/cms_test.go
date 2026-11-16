package cms

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"math/big"
	"testing"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func selfSigned(t *testing.T, key crypto.Signer) *x509.Certificate {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "cms test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatalf("x509 create certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509 parse certificate: %v", err)
	}
	return certificate
}

func TestSignAndVerify(t *testing.T) {
	t.Parallel()

	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa generate key: %v", err)
	}

	content := []byte("the byte ranges of a PDF")
	contentDigest := sha256.Sum256(content)

	testCases := []struct {
		name        string
		key         crypto.Signer
		contentType asn1.ObjectIdentifier
		// encapsulated carries the content inside the SignedData rather than beside it.
		encapsulated  bool
		verifyContent []byte
		wantErr       error
	}{
		{name: "ecdsa detached", key: ecdsaKey, contentType: OidData, verifyContent: content},
		{name: "rsa detached", key: rsaKey, contentType: OidData, verifyContent: content},
		{name: "ecdsa encapsulated tst info", key: ecdsaKey, contentType: OidTstInfo, encapsulated: true},
		{name: "changed content", key: ecdsaKey, contentType: OidData, verifyContent: []byte("the byte ranges of a PDF!"), wantErr: altshiftErrors.ErrVerificationError},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			certificate := selfSigned(t, testCase.key)
			signerInfo, err := NewSignerInfo(testCase.key, certificate, testCase.contentType, contentDigest[:])
			if err != nil {
				t.Fatalf("%s: new signer info: %v", testCase.name, err)
			}
			signerInfo.UnsignedAttributes = append(signerInfo.UnsignedAttributes, &Attribute{Type: OidAttributeTimeStampToken, Values: []asn1.RawValue{{FullBytes: []byte{0x05, 0x00}}}})

			var encapsulated []byte
			if testCase.encapsulated {
				encapsulated = content
			}
			der, err := MarshalSignedData(testCase.contentType, encapsulated, []*x509.Certificate{certificate}, signerInfo)
			if err != nil {
				t.Fatalf("%s: marshal signed data: %v", testCase.name, err)
			}

			parsed, err := ParseSignedData(der)
			if err != nil {
				t.Fatalf("%s: parse signed data: %v", testCase.name, err)
			}
			if !parsed.ContentType.Equal(testCase.contentType) {
				t.Errorf("%s: content type %s, want %s", testCase.name, parsed.ContentType, testCase.contentType)
			}
			if len(parsed.SignerInfos) != 1 || len(parsed.SignerInfos[0].UnsignedAttributes) != 1 {
				t.Fatalf("%s: unsigned attribute did not survive the round trip", testCase.name)
			}

			signer, err := parsed.Verify(testCase.verifyContent)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("%s: verify = %v, want %v", testCase.name, err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: verify: %v", testCase.name, err)
			}
			if !signer.Equal(certificate) {
				t.Errorf("%s: verify returned another certificate", testCase.name)
			}
		})
	}
}

// TestContentInfoWrapping holds the [0] EXPLICIT around the SignedData, which encoding/asn1 drops silently from a RawValue.
func TestContentInfoWrapping(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	certificate := selfSigned(t, key)
	digest := sha256.Sum256(nil)
	signerInfo, err := NewSignerInfo(key, certificate, OidData, digest[:])
	if err != nil {
		t.Fatalf("new signer info: %v", err)
	}
	der, err := MarshalSignedData(OidData, nil, nil, signerInfo)
	if err != nil {
		t.Fatalf("marshal signed data: %v", err)
	}

	var outer struct {
		ContentType asn1.ObjectIdentifier
		Content     asn1.RawValue
	}
	if _, err := asn1.Unmarshal(der, &outer); err != nil {
		t.Fatalf("asn1 unmarshal: %v", err)
	}
	if outer.Content.Class != asn1.ClassContextSpecific || outer.Content.Tag != 0 || !outer.Content.IsCompound {
		t.Errorf("content is class %d tag %d, want context-specific [0]", outer.Content.Class, outer.Content.Tag)
	}
}

func TestSigningCertificateV2(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	certificate := selfSigned(t, key)
	digest := sha256.Sum256(nil)
	signerInfo, err := NewSignerInfo(key, certificate, OidData, digest[:])
	if err != nil {
		t.Fatalf("new signer info: %v", err)
	}
	der, err := MarshalSignedData(OidData, nil, nil, signerInfo)
	if err != nil {
		t.Fatalf("marshal signed data: %v", err)
	}
	parsed, err := ParseSignedData(der)
	if err != nil {
		t.Fatalf("parse signed data: %v", err)
	}

	value, ok := parsed.SignerInfos[0].Attribute(OidAttributeSigningCertificateV2)
	if !ok {
		t.Fatal("no signing-certificate-v2 attribute")
	}
	var signingCertificate signingCertificateV2
	if _, err := asn1.Unmarshal(value.FullBytes, &signingCertificate); err != nil {
		t.Fatalf("asn1 unmarshal: %v", err)
	}
	certificateHash := sha256.Sum256(certificate.Raw)
	if len(signingCertificate.Certs) != 1 || !bytes.Equal(signingCertificate.Certs[0].CertHash, certificateHash[:]) {
		t.Error("the certificate hash is not the signer's")
	}
	if signingCertificate.Certs[0].IssuerSerial.SerialNumber.Cmp(certificate.SerialNumber) != 0 {
		t.Error("the issuer serial is not the signer's")
	}
}

func TestNewSignerInfoRejects(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	certificate := selfSigned(t, key)
	digest := sha256.Sum256(nil)

	testCases := []struct {
		name        string
		key         crypto.Signer
		certificate *x509.Certificate
		contentType asn1.ObjectIdentifier
		digest      []byte
	}{
		{name: "no key", certificate: certificate, contentType: OidData, digest: digest[:]},
		{name: "no certificate", key: key, contentType: OidData, digest: digest[:]},
		{name: "no content type", key: key, certificate: certificate, digest: digest[:]},
		{name: "short digest", key: key, certificate: certificate, contentType: OidData, digest: digest[:20]},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewSignerInfo(testCase.key, testCase.certificate, testCase.contentType, testCase.digest); err == nil {
				t.Errorf("%s: new signer info succeeded", testCase.name)
			}
		})
	}
}

// TestVerifyContentSelection holds Verify to exactly one source of content, so a caller's content is never passed over for content the signature brought along.
func TestVerifyContentSelection(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	certificate := selfSigned(t, key)
	content := []byte("what was signed")
	digest := sha256.Sum256(content)

	build := func(encapsulated []byte) *ParsedSignedData {
		signerInfo, err := NewSignerInfo(key, certificate, OidData, digest[:])
		if err != nil {
			t.Fatalf("new signer info: %v", err)
		}
		der, err := MarshalSignedData(OidData, encapsulated, []*x509.Certificate{certificate}, signerInfo)
		if err != nil {
			t.Fatalf("marshal signed data: %v", err)
		}
		parsed, err := ParseSignedData(der)
		if err != nil {
			t.Fatalf("parse signed data: %v", err)
		}
		return parsed
	}

	testCases := []struct {
		name     string
		parsed   *ParsedSignedData
		detached []byte
		wantErr  bool
	}{
		{name: "encapsulated", parsed: build(content)},
		{name: "detached", parsed: build(nil), detached: content},
		{name: "encapsulated, and other content given", parsed: build(content), detached: []byte("a completely different document"), wantErr: true},
		{name: "detached, and nothing given", parsed: build(nil), wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := testCase.parsed.Verify(testCase.detached)
			if (err != nil) != testCase.wantErr {
				t.Errorf("%s: verify = %v", testCase.name, err)
			}
			if err != nil && !errors.Is(err, altshiftErrors.ErrVerificationError) {
				t.Errorf("%s: %v is not a verification error", testCase.name, err)
			}
		})
	}
}

// TestSubjectKeyIdentifierSigner reads and verifies a signer identified by its subject key identifier, as some timestamping authorities do.
func TestSubjectKeyIdentifierSigner(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: "key id signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		SubjectKeyId: []byte{1, 2, 3, 4},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatalf("x509 create certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509 parse certificate: %v", err)
	}

	content := []byte("stamped")
	digest := sha256.Sum256(content)
	signerInfo, err := NewSignerInfo(key, certificate, OidData, digest[:])
	if err != nil {
		t.Fatalf("new signer info: %v", err)
	}
	// The same signer info, with the signer named by key identifier: [0] IMPLICIT, version 3.
	raw, err := asn1.Marshal(parsedSignerInfo{
		Version:            3,
		Sid:                asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, Bytes: certificate.SubjectKeyId},
		DigestAlgorithm:    pkix.AlgorithmIdentifier{Algorithm: OidSha256},
		SignedAttributes:   asn1.RawValue{FullBytes: implicitTag(signerInfo.signedAttributes, 0)},
		SignatureAlgorithm: signerInfo.signatureAlgorithm,
		Signature:          signerInfo.signature,
	})
	if err != nil {
		t.Fatalf("asn1 marshal: %v", err)
	}
	parsedInfo, err := parseSignerInfo(raw)
	if err != nil {
		t.Fatalf("parse signer info: %v", err)
	}
	if !bytes.Equal(parsedInfo.SubjectKeyId, certificate.SubjectKeyId) || parsedInfo.SerialNumber != nil {
		t.Fatalf("parsed sid: key id %x, serial %v", parsedInfo.SubjectKeyId, parsedInfo.SerialNumber)
	}

	signedData := &ParsedSignedData{ContentType: OidData, Certificates: []*x509.Certificate{certificate}, SignerInfos: []*ParsedSignerInfo{parsedInfo}}
	signer, err := signedData.Verify(content)
	if err != nil || !signer.Equal(certificate) {
		t.Errorf("verify = %v, %v", signer, err)
	}
}

func TestHashFor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		algorithm asn1.ObjectIdentifier
		want      crypto.Hash
		wantErr   bool
	}{
		{name: "sha256", algorithm: OidSha256, want: crypto.SHA256},
		{name: "sha384", algorithm: OidSha384, want: crypto.SHA384},
		{name: "sha512", algorithm: OidSha512, want: crypto.SHA512},
		{name: "sha1 is refused", algorithm: asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}, wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := HashFor(testCase.algorithm)
			if (err != nil) != testCase.wantErr || got != testCase.want {
				t.Errorf("%s: hash for = %v, %v", testCase.name, got, err)
			}
		})
	}
}
