// Package cms builds and reads the CMS SignedData (RFC 5652) that PAdES signatures and RFC 3161 timestamp tokens are made of.
package cms

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

var (
	OidData                          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	OidSignedData                    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	OidTstInfo                       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 4}
	OidAttributeContentType          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	OidAttributeMessageDigest        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	OidAttributeSigningCertificateV2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}
	OidAttributeTimeStampToken       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}

	OidSha256 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	OidSha384 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	OidSha512 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}

	oidEcdsaWithSha256  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidEcdsaWithSha384  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 3}
	oidEcdsaWithSha512  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 4}
	oidRsaEncryption    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidSha256WithRsa    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	oidSha384WithRsa    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 12}
	oidSha512WithRsa    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 13}
	errUnsupportedKey   = errors.New("unsupported key type")
	errUnsupportedHash  = errors.New("unsupported digest algorithm")
	errUnsupportedSigAl = errors.New("unsupported signature algorithm")
)

// Attribute is a CMS attribute: a type and its set of values.
type Attribute struct {
	Type   asn1.ObjectIdentifier
	Values []asn1.RawValue `asn1:"set"`
}

type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,tag:0"`
}

type encapsulatedContentInfo struct {
	EContentType asn1.ObjectIdentifier
	EContent     []byte `asn1:"explicit,optional,tag:0"`
}

type issuerAndSerialNumber struct {
	Issuer       asn1.RawValue
	SerialNumber *big.Int
}

type signerInfo struct {
	Version            int
	Sid                issuerAndSerialNumber
	DigestAlgorithm    pkix.AlgorithmIdentifier
	SignedAttributes   asn1.RawValue `asn1:"optional,tag:0"`
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Signature          []byte
	UnsignedAttributes asn1.RawValue `asn1:"optional,tag:1"`
}

// parsedSignerInfo reads the sid raw: an IssuerAndSerialNumber, or a [0] SubjectKeyIdentifier.
type parsedSignerInfo struct {
	Version            int
	Sid                asn1.RawValue
	DigestAlgorithm    pkix.AlgorithmIdentifier
	SignedAttributes   asn1.RawValue `asn1:"optional,tag:0"`
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Signature          []byte
	UnsignedAttributes asn1.RawValue `asn1:"optional,tag:1"`
}

type signedData struct {
	Version          int
	DigestAlgorithms asn1.RawValue
	EncapContentInfo encapsulatedContentInfo
	Certificates     asn1.RawValue `asn1:"optional,tag:0"`
	Crls             asn1.RawValue `asn1:"optional,tag:1"`
	SignerInfos      asn1.RawValue
}

type issuerSerial struct {
	Issuer       []asn1.RawValue
	SerialNumber *big.Int
}

type essCertIdV2 struct {
	CertHash     []byte
	IssuerSerial issuerSerial
}

type signingCertificateV2 struct {
	Certs []essCertIdV2
}

// SignerInfo is one signer's signed part of a SignedData being built; unsigned attributes may be added until it is marshalled.
type SignerInfo struct {
	certificate        *x509.Certificate
	signedAttributes   []byte
	signatureAlgorithm pkix.AlgorithmIdentifier
	signature          []byte
	UnsignedAttributes []*Attribute
}

// Signature returns the signature value, which is what a signature timestamp covers.
func (s *SignerInfo) Signature() []byte {
	if s == nil {
		return nil
	}
	return s.signature
}

func signatureAlgorithmFor(publicKey crypto.PublicKey) (pkix.AlgorithmIdentifier, error) {
	switch publicKey.(type) {
	case *ecdsa.PublicKey:
		return pkix.AlgorithmIdentifier{Algorithm: oidEcdsaWithSha256}, nil
	case *rsa.PublicKey:
		return pkix.AlgorithmIdentifier{Algorithm: oidSha256WithRsa, Parameters: asn1.NullRawValue}, nil
	default:
		return pkix.AlgorithmIdentifier{}, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %T", errUnsupportedKey, publicKey))
	}
}

func marshalAttribute(attributeType asn1.ObjectIdentifier, value any) (*Attribute, error) {
	valueDer, err := asn1.Marshal(value)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal (attribute value): %w", err), attributeType)
	}
	return &Attribute{Type: attributeType, Values: []asn1.RawValue{{FullBytes: valueDer}}}, nil
}

func signingCertificateV2Attribute(certificate *x509.Certificate) (*Attribute, error) {
	certificateHash := crypto.SHA256.New()
	certificateHash.Write(certificate.Raw)

	value := signingCertificateV2{
		Certs: []essCertIdV2{
			{
				CertHash: certificateHash.Sum(nil),
				IssuerSerial: issuerSerial{
					// GeneralName directoryName is [4], explicit because Name is a CHOICE.
					Issuer:       []asn1.RawValue{{Class: asn1.ClassContextSpecific, Tag: 4, IsCompound: true, Bytes: certificate.RawIssuer}},
					SerialNumber: certificate.SerialNumber,
				},
			},
		},
	}
	return marshalAttribute(OidAttributeSigningCertificateV2, value)
}

func marshalAttributeSet(attributes []*Attribute) ([]byte, error) {
	values := make([]Attribute, 0, len(attributes))
	for _, attribute := range attributes {
		if attribute != nil {
			values = append(values, *attribute)
		}
	}
	// "set" makes encoding/asn1 sort the elements, as DER requires of a SET OF.
	der, err := asn1.MarshalWithParams(values, "set")
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal (attribute set): %w", err))
	}
	return der, nil
}

// NewSignerInfo signs the content-type, message-digest and signing-certificate-v2 attributes for content of the given type whose SHA-256 digest is contentDigest.
func NewSignerInfo(key crypto.Signer, certificate *x509.Certificate, contentType asn1.ObjectIdentifier, contentDigest []byte) (*SignerInfo, error) {
	if key == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("key"))
	}
	if certificate == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("certificate"))
	}
	if len(contentType) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("content type"))
	}
	if len(contentDigest) != crypto.SHA256.Size() {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: content digest is %d bytes, not a SHA-256", altshiftErrors.ErrValidationError, len(contentDigest)))
	}

	signatureAlgorithm, err := signatureAlgorithmFor(key.Public())
	if err != nil {
		return nil, fmt.Errorf("signature algorithm for: %w", err)
	}

	contentTypeAttribute, err := marshalAttribute(OidAttributeContentType, contentType)
	if err != nil {
		return nil, fmt.Errorf("marshal attribute (content type): %w", err)
	}
	messageDigestAttribute, err := marshalAttribute(OidAttributeMessageDigest, contentDigest)
	if err != nil {
		return nil, fmt.Errorf("marshal attribute (message digest): %w", err)
	}
	signingCertificateAttribute, err := signingCertificateV2Attribute(certificate)
	if err != nil {
		return nil, fmt.Errorf("signing certificate v2 attribute: %w", err)
	}

	signedAttributes, err := marshalAttributeSet([]*Attribute{contentTypeAttribute, messageDigestAttribute, signingCertificateAttribute})
	if err != nil {
		return nil, fmt.Errorf("marshal attribute set: %w", err)
	}

	attributesHash := crypto.SHA256.New()
	attributesHash.Write(signedAttributes)
	signature, err := key.Sign(rand.Reader, attributesHash.Sum(nil), crypto.SHA256)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("key sign: %w", err))
	}
	if len(signature) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("signature"))
	}

	return &SignerInfo{
		certificate:        certificate,
		signedAttributes:   signedAttributes,
		signatureAlgorithm: signatureAlgorithm,
		signature:          signature,
	}, nil
}

// implicitTag rewrites the outer tag of a DER SET as the context-specific constructed tag the SignerInfo carries it under.
func implicitTag(setDer []byte, tag byte) []byte {
	out := bytes.Clone(setDer)
	out[0] = 0xa0 | tag
	return out
}

func (s *SignerInfo) marshal() ([]byte, error) {
	info := signerInfo{
		Version:            1,
		Sid:                issuerAndSerialNumber{Issuer: asn1.RawValue{FullBytes: s.certificate.RawIssuer}, SerialNumber: s.certificate.SerialNumber},
		DigestAlgorithm:    pkix.AlgorithmIdentifier{Algorithm: OidSha256},
		SignedAttributes:   asn1.RawValue{FullBytes: implicitTag(s.signedAttributes, 0)},
		SignatureAlgorithm: s.signatureAlgorithm,
		Signature:          s.signature,
	}

	if len(s.UnsignedAttributes) != 0 {
		unsignedAttributes, err := marshalAttributeSet(s.UnsignedAttributes)
		if err != nil {
			return nil, fmt.Errorf("marshal attribute set (unsigned): %w", err)
		}
		info.UnsignedAttributes = asn1.RawValue{FullBytes: implicitTag(unsignedAttributes, 1)}
	}

	der, err := asn1.Marshal(info)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal (signer info): %w", err))
	}
	return der, nil
}

// MarshalSignedData encodes a ContentInfo holding a SignedData; a nil content makes it detached.
func MarshalSignedData(contentType asn1.ObjectIdentifier, content []byte, certificates []*x509.Certificate, signerInfos ...*SignerInfo) ([]byte, error) {
	if len(contentType) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("content type"))
	}
	if len(signerInfos) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("signer infos"))
	}

	digestAlgorithms, err := asn1.MarshalWithParams([]pkix.AlgorithmIdentifier{{Algorithm: OidSha256}}, "set")
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal (digest algorithms): %w", err))
	}

	var signerInfosDer [][]byte
	for _, info := range signerInfos {
		if info == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("signer info"))
		}
		der, err := info.marshal()
		if err != nil {
			return nil, fmt.Errorf("signer info marshal: %w", err)
		}
		signerInfosDer = append(signerInfosDer, der)
	}
	signerInfosRaw := make([]asn1.RawValue, 0, len(signerInfosDer))
	for _, der := range signerInfosDer {
		signerInfosRaw = append(signerInfosRaw, asn1.RawValue{FullBytes: der})
	}
	signerInfosSet, err := asn1.MarshalWithParams(signerInfosRaw, "set")
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal (signer infos): %w", err))
	}

	data := signedData{
		Version:          1,
		DigestAlgorithms: asn1.RawValue{FullBytes: digestAlgorithms},
		EncapContentInfo: encapsulatedContentInfo{EContentType: contentType, EContent: content},
		SignerInfos:      asn1.RawValue{FullBytes: signerInfosSet},
	}
	if !contentType.Equal(OidData) {
		// RFC 5652 5.1: version 3 when the encapsulated content is not id-data.
		data.Version = 3
	}

	var certificatesDer []byte
	for _, certificate := range certificates {
		if certificate != nil {
			certificatesDer = append(certificatesDer, certificate.Raw...)
		}
	}
	if len(certificatesDer) != 0 {
		data.Certificates = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: certificatesDer}
	}

	signedDataDer, err := asn1.Marshal(data)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal (signed data): %w", err))
	}

	// encoding/asn1 ignores "explicit" on a RawValue, so the [0] wrapper is built here.
	der, err := asn1.Marshal(contentInfo{ContentType: OidSignedData, Content: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: signedDataDer}})
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal (content info): %w", err))
	}
	return der, nil
}

// ParsedSignerInfo is a signer info as read from a SignedData.
type ParsedSignerInfo struct {
	// IssuerRaw and SerialNumber identify the signer, or SubjectKeyId does.
	IssuerRaw          []byte
	SerialNumber       *big.Int
	SubjectKeyId       []byte
	DigestAlgorithm    asn1.ObjectIdentifier
	SignedAttributes   []*Attribute
	SignatureAlgorithm asn1.ObjectIdentifier
	Signature          []byte
	UnsignedAttributes []*Attribute

	signedAttributesSet []byte
}

// Attribute returns the first value of the signed attribute of the given type.
func (p *ParsedSignerInfo) Attribute(attributeType asn1.ObjectIdentifier) (asn1.RawValue, bool) {
	if p == nil {
		return asn1.RawValue{}, false
	}
	for _, attribute := range p.SignedAttributes {
		if attribute != nil && attribute.Type.Equal(attributeType) && len(attribute.Values) != 0 {
			return attribute.Values[0], true
		}
	}
	return asn1.RawValue{}, false
}

// ParsedSignedData is a SignedData as read from its ContentInfo.
type ParsedSignedData struct {
	ContentType  asn1.ObjectIdentifier
	Content      []byte
	Certificates []*x509.Certificate
	SignerInfos  []*ParsedSignerInfo
}

func parseAttributeSet(contents []byte) ([]*Attribute, error) {
	var attributes []*Attribute
	for rest := contents; len(rest) != 0; {
		attribute := &Attribute{}
		var err error
		rest, err = asn1.Unmarshal(rest, attribute)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (attribute): %w", altshiftErrors.ErrParseError, err))
		}
		attributes = append(attributes, attribute)
	}
	return attributes, nil
}

func parseSignerInfo(der []byte) (*ParsedSignerInfo, error) {
	var info parsedSignerInfo
	rest, err := asn1.Unmarshal(der, &info)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (signer info): %w", altshiftErrors.ErrParseError, err))
	}
	if len(rest) != 0 {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: trailing data after signer info", altshiftErrors.ErrParseError))
	}
	if len(info.SignedAttributes.FullBytes) == 0 {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %w", altshiftErrors.ErrParseError, empty_error.New("signed attributes")))
	}

	parsed := &ParsedSignerInfo{}
	switch {
	case info.Sid.Class == asn1.ClassUniversal && info.Sid.Tag == asn1.TagSequence:
		var sid issuerAndSerialNumber
		if _, err := asn1.Unmarshal(info.Sid.FullBytes, &sid); err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (issuer and serial number): %w", altshiftErrors.ErrParseError, err))
		}
		parsed.IssuerRaw, parsed.SerialNumber = sid.Issuer.FullBytes, sid.SerialNumber
	case info.Sid.Class == asn1.ClassContextSpecific && info.Sid.Tag == 0:
		// [0] IMPLICIT SubjectKeyIdentifier: the octets themselves.
		parsed.SubjectKeyId = info.Sid.Bytes
	default:
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: signer identifier of class %d tag %d", altshiftErrors.ErrParseError, info.Sid.Class, info.Sid.Tag))
	}

	signedAttributes, err := parseAttributeSet(info.SignedAttributes.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse attribute set (signed): %w", err)
	}
	var unsignedAttributes []*Attribute
	if len(info.UnsignedAttributes.FullBytes) != 0 {
		unsignedAttributes, err = parseAttributeSet(info.UnsignedAttributes.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse attribute set (unsigned): %w", err)
		}
	}

	// What was signed is the DER SET, not the [0] it is carried under.
	signedAttributesSet := bytes.Clone(info.SignedAttributes.FullBytes)
	signedAttributesSet[0] = 0x31

	parsed.DigestAlgorithm = info.DigestAlgorithm.Algorithm
	parsed.SignedAttributes = signedAttributes
	parsed.SignatureAlgorithm = info.SignatureAlgorithm.Algorithm
	parsed.Signature = info.Signature
	parsed.UnsignedAttributes = unsignedAttributes
	parsed.signedAttributesSet = signedAttributesSet
	return parsed, nil
}

// ParseSignedData reads a ContentInfo holding a SignedData.
func ParseSignedData(der []byte) (*ParsedSignedData, error) {
	if len(der) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("der"))
	}

	var info contentInfo
	if _, err := asn1.Unmarshal(der, &info); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (content info): %w", altshiftErrors.ErrParseError, err))
	}
	if !info.ContentType.Equal(OidSignedData) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: content type %s is not signed data", altshiftErrors.ErrParseError, info.ContentType))
	}

	var data signedData
	if _, err := asn1.Unmarshal(info.Content.Bytes, &data); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (signed data): %w", altshiftErrors.ErrParseError, err))
	}

	parsed := &ParsedSignedData{ContentType: data.EncapContentInfo.EContentType, Content: data.EncapContentInfo.EContent}

	for rest := data.Certificates.Bytes; len(rest) != 0; {
		var element asn1.RawValue
		var err error
		rest, err = asn1.Unmarshal(rest, &element)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (certificate): %w", altshiftErrors.ErrParseError, err))
		}
		// Other certificate choices are context-tagged; only plain certificates are read.
		if element.Class != asn1.ClassUniversal {
			continue
		}
		certificate, err := x509.ParseCertificate(element.FullBytes)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: x509 parse certificate: %w", altshiftErrors.ErrParseError, err))
		}
		parsed.Certificates = append(parsed.Certificates, certificate)
	}

	for rest := data.SignerInfos.Bytes; len(rest) != 0; {
		var element asn1.RawValue
		var err error
		rest, err = asn1.Unmarshal(rest, &element)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (signer info element): %w", altshiftErrors.ErrParseError, err))
		}
		info, err := parseSignerInfo(element.FullBytes)
		if err != nil {
			return nil, fmt.Errorf("parse signer info: %w", err)
		}
		parsed.SignerInfos = append(parsed.SignerInfos, info)
	}

	return parsed, nil
}

// HashFor maps a digest algorithm identifier to its hash.
func HashFor(algorithm asn1.ObjectIdentifier) (crypto.Hash, error) {
	switch {
	case algorithm.Equal(OidSha256):
		return crypto.SHA256, nil
	case algorithm.Equal(OidSha384):
		return crypto.SHA384, nil
	case algorithm.Equal(OidSha512):
		return crypto.SHA512, nil
	default:
		return 0, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", errUnsupportedHash, algorithm))
	}
}

func verifySignature(publicKey crypto.PublicKey, signatureAlgorithm asn1.ObjectIdentifier, hash crypto.Hash, digest []byte, signature []byte) error {
	switch key := publicKey.(type) {
	case *ecdsa.PublicKey:
		if !signatureAlgorithm.Equal(oidEcdsaWithSha256) && !signatureAlgorithm.Equal(oidEcdsaWithSha384) && !signatureAlgorithm.Equal(oidEcdsaWithSha512) {
			return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s with an ECDSA key", errUnsupportedSigAl, signatureAlgorithm))
		}
		if !ecdsa.VerifyASN1(key, digest, signature) {
			return altshiftErrors.NewWithTrace(fmt.Errorf("%w: ecdsa signature", altshiftErrors.ErrVerificationError))
		}
		return nil
	case *rsa.PublicKey:
		if !signatureAlgorithm.Equal(oidRsaEncryption) && !signatureAlgorithm.Equal(oidSha256WithRsa) && !signatureAlgorithm.Equal(oidSha384WithRsa) && !signatureAlgorithm.Equal(oidSha512WithRsa) {
			return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s with an RSA key", errUnsupportedSigAl, signatureAlgorithm))
		}
		if err := rsa.VerifyPKCS1v15(key, hash, digest, signature); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("%w: rsa verify pkcs1v15: %w", altshiftErrors.ErrVerificationError, err))
		}
		return nil
	default:
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %T", errUnsupportedKey, publicKey))
	}
}

// Verify checks the single signer's signature over its signed attributes and their binding to the content, the encapsulated one or detachedContent, and returns the signer's certificate.
func (p *ParsedSignedData) Verify(detachedContent []byte) (*x509.Certificate, error) {
	if p == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("parsed signed data"))
	}
	if len(p.SignerInfos) != 1 {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %d signer infos, expected one", altshiftErrors.ErrVerificationError, len(p.SignerInfos)))
	}
	info := p.SignerInfos[0]

	var certificate *x509.Certificate
	for _, candidate := range p.Certificates {
		if candidate == nil {
			continue
		}
		byIssuer := info.SerialNumber != nil && bytes.Equal(candidate.RawIssuer, info.IssuerRaw) && candidate.SerialNumber.Cmp(info.SerialNumber) == 0
		byKeyId := len(info.SubjectKeyId) != 0 && bytes.Equal(candidate.SubjectKeyId, info.SubjectKeyId)
		if byIssuer || byKeyId {
			certificate = candidate
			break
		}
	}
	if certificate == nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: the signer's certificate is not in the signed data", altshiftErrors.ErrVerificationError))
	}

	// Exactly one of the two: a caller's content must never be passed over for content the signature brought along.
	var content []byte
	switch {
	case p.Content != nil && detachedContent != nil:
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: content given for a signature that carries its own", altshiftErrors.ErrVerificationError))
	case p.Content != nil:
		content = p.Content
	case detachedContent != nil:
		content = detachedContent
	default:
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: no content to verify the signature against", altshiftErrors.ErrVerificationError))
	}
	hash, err := HashFor(info.DigestAlgorithm)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: hash for: %w", altshiftErrors.ErrVerificationError, err))
	}
	contentHash := hash.New()
	contentHash.Write(content)

	messageDigestValue, ok := info.Attribute(OidAttributeMessageDigest)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: no message digest attribute", altshiftErrors.ErrVerificationError))
	}
	var messageDigest []byte
	if _, err := asn1.Unmarshal(messageDigestValue.FullBytes, &messageDigest); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (message digest): %w", altshiftErrors.ErrParseError, err))
	}
	if !bytes.Equal(messageDigest, contentHash.Sum(nil)) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: message digest does not match the content", altshiftErrors.ErrVerificationError))
	}

	contentTypeValue, ok := info.Attribute(OidAttributeContentType)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: no content type attribute", altshiftErrors.ErrVerificationError))
	}
	var contentType asn1.ObjectIdentifier
	if _, err := asn1.Unmarshal(contentTypeValue.FullBytes, &contentType); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (content type): %w", altshiftErrors.ErrParseError, err))
	}
	if !contentType.Equal(p.ContentType) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: content type attribute %s differs from %s", altshiftErrors.ErrVerificationError, contentType, p.ContentType))
	}

	attributesHash := hash.New()
	attributesHash.Write(info.signedAttributesSet)
	if err := verifySignature(certificate.PublicKey, info.SignatureAlgorithm, hash, attributesHash.Sum(nil), info.Signature); err != nil {
		if errors.Is(err, altshiftErrors.ErrVerificationError) {
			return nil, fmt.Errorf("verify signature: %w", err)
		}
		return nil, fmt.Errorf("%w: verify signature: %w", altshiftErrors.ErrVerificationError, err)
	}

	return certificate, nil
}
