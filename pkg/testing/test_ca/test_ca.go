// Package test_ca makes a throwaway certificate hierarchy shaped like a real e-seal's: a root, an issuing CA, a sealing certificate and a timestamping unit, with a CRL from each CA.
package test_ca

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// The CRL addresses are under .invalid, so a validator can only use the CRLs the document carries.
const testOrganization = "Signals Seal Test"

const (
	rootCrlUrl    = "http://signals-seal-test.invalid/root.crl"
	issuingCrlUrl = "http://signals-seal-test.invalid/issuing.crl"
)

// Authority is the hierarchy and the sealing key.
type Authority struct {
	Root       *x509.Certificate
	Issuing    *x509.Certificate
	Seal       *x509.Certificate
	SealKey    crypto.Signer
	Tsa        *x509.Certificate
	tsaKey     crypto.Signer
	rootKey    crypto.Signer
	issuingKey crypto.Signer
}

func newKey() (*ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("ecdsa generate key: %w", err))
	}
	return key, nil
}

func serialNumber() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("rand int: %w", err))
	}
	return serial, nil
}

func issue(template *x509.Certificate, publicKey crypto.PublicKey, issuer *x509.Certificate, issuerKey crypto.Signer) (*x509.Certificate, error) {
	serial, err := serialNumber()
	if err != nil {
		return nil, fmt.Errorf("serial number: %w", err)
	}
	template.SerialNumber = serial
	if issuer == nil {
		issuer = template
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, publicKey, issuerKey)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("x509 create certificate: %w", err), template.Subject.String())
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("x509 parse certificate: %w", err))
	}
	return certificate, nil
}

// New makes a hierarchy valid from an hour before now for a year, sealing in the name of organization.
func New(now time.Time, organization string) (*Authority, error) {
	if organization == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("organization"))
	}

	rootKey, err := newKey()
	if err != nil {
		return nil, fmt.Errorf("new key (root): %w", err)
	}
	issuingKey, err := newKey()
	if err != nil {
		return nil, fmt.Errorf("new key (issuing): %w", err)
	}
	sealKey, err := newKey()
	if err != nil {
		return nil, fmt.Errorf("new key (seal): %w", err)
	}

	notBefore := now.Add(-time.Hour)
	notAfter := now.AddDate(1, 0, 0)

	root, err := issue(
		&x509.Certificate{
			Subject:               pkix.Name{Country: []string{"SE"}, Organization: []string{testOrganization}, CommonName: "Signals Seal Test Root"},
			NotBefore:             notBefore,
			NotAfter:              notAfter,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		},
		rootKey.Public(), nil, rootKey,
	)
	if err != nil {
		return nil, fmt.Errorf("issue (root): %w", err)
	}

	issuing, err := issue(
		&x509.Certificate{
			Subject:               pkix.Name{Country: []string{"SE"}, Organization: []string{testOrganization}, CommonName: "Signals Seal Test Issuing CA"},
			NotBefore:             notBefore,
			NotAfter:              notAfter,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
			MaxPathLenZero:        true,
			CRLDistributionPoints: []string{rootCrlUrl},
		},
		issuingKey.Public(), root, rootKey,
	)
	if err != nil {
		return nil, fmt.Errorf("issue (issuing): %w", err)
	}

	seal, err := issue(sealTemplate(organization, notBefore, notAfter), sealKey.Public(), issuing, issuingKey)
	if err != nil {
		return nil, fmt.Errorf("issue (seal): %w", err)
	}

	tsaKey, err := newKey()
	if err != nil {
		return nil, fmt.Errorf("new key (tsa): %w", err)
	}
	tsa, err := issue(
		&x509.Certificate{
			Subject:               pkix.Name{Country: []string{"SE"}, Organization: []string{testOrganization}, CommonName: "Signals Seal Test Timestamping Unit"},
			NotBefore:             notBefore,
			NotAfter:              notAfter,
			KeyUsage:              x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
			BasicConstraintsValid: true,
			CRLDistributionPoints: []string{issuingCrlUrl},
		},
		tsaKey.Public(), issuing, issuingKey,
	)
	if err != nil {
		return nil, fmt.Errorf("issue (tsa): %w", err)
	}

	return &Authority{Root: root, Issuing: issuing, Seal: seal, SealKey: sealKey, Tsa: tsa, tsaKey: tsaKey, rootKey: rootKey, issuingKey: issuingKey}, nil
}

func sealTemplate(organization string, notBefore time.Time, notAfter time.Time) *x509.Certificate {
	return &x509.Certificate{
		Subject:               pkix.Name{Country: []string{"SE"}, Organization: []string{organization}, CommonName: organization + " e-seal (test)"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		BasicConstraintsValid: true,
		CRLDistributionPoints: []string{issuingCrlUrl},
	}
}

// IssueSeal issues a sealing certificate for a key the authority does not hold, such as one in an HSM, from its issuing CA, valid as long as the authority's own.
func (a *Authority) IssueSeal(publicKey crypto.PublicKey) (*x509.Certificate, error) {
	if a == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("authority"))
	}
	if publicKey == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("public key"))
	}
	certificate, err := issue(sealTemplate(a.Seal.Subject.Organization[0], a.Seal.NotBefore, a.Seal.NotAfter), publicKey, a.Issuing, a.issuingKey)
	if err != nil {
		return nil, fmt.Errorf("issue (seal): %w", err)
	}
	return certificate, nil
}

// Chain is the sealing certificate and its issuing CA, as a signature carries them.
func (a *Authority) Chain() []*x509.Certificate {
	return []*x509.Certificate{a.Seal, a.Issuing}
}

// Crls issues an empty CRL from each CA, current at now.
func (a *Authority) Crls(now time.Time) ([][]byte, error) {
	var crls [][]byte
	for _, ca := range []struct {
		certificate *x509.Certificate
		key         crypto.Signer
	}{{a.Root, a.rootKey}, {a.Issuing, a.issuingKey}} {
		der, err := x509.CreateRevocationList(
			rand.Reader,
			&x509.RevocationList{Number: big.NewInt(now.Unix()), ThisUpdate: now, NextUpdate: now.AddDate(0, 0, 7)},
			ca.certificate,
			ca.key,
		)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("x509 create revocation list: %w", err), ca.certificate.Subject.String())
		}
		crls = append(crls, der)
	}
	return crls, nil
}

// RootPem is the root certificate in PEM, for a validator to trust.
func (a *Authority) RootPem() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: a.Root.Raw})
}
