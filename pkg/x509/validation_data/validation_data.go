// Package validation_data gathers what a validator needs to check a certificate long after the fact: every certificate up to a root, and a CRL for each one below it.
package validation_data

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

// Errors about the certificates being collected for; they are the sealer's own and its authority's, so none is classed as client input.
var (
	ErrNoIssuer     = errors.New("no issuer found")
	ErrNoCurrentCrl = errors.New("no current crl found")
	ErrRevoked      = errors.New("certificate revoked")
	ErrChainTooLong = errors.New("certificate chain too long")
)

// The most read from a URL a certificate names: a certificate is a few kilobytes, while a large CA's CRL runs to megabytes.
const (
	maxCertificateResponseSize = 1 << 20
	maxCrlResponseSize         = 32 << 20
)

// maxChainLength bounds the walk up a chain, so that an AIA loop cannot keep it going.
const maxChainLength = 8

// clockSkew is how far ahead of this clock a CA's may run before its fresh CRL is taken for one from the future.
const clockSkew = 5 * time.Minute

// ValidationData is the certificates and CRLs that go into a PDF's DSS.
type ValidationData struct {
	Certificates []*x509.Certificate
	Crls         [][]byte
}

func (v *ValidationData) addCertificate(certificate *x509.Certificate) {
	if !slices.ContainsFunc(v.Certificates, func(known *x509.Certificate) bool { return known.Equal(certificate) }) {
		v.Certificates = append(v.Certificates, certificate)
	}
}

func (v *ValidationData) addCrl(crl []byte) {
	if !slices.ContainsFunc(v.Crls, func(known []byte) bool { return bytes.Equal(known, crl) }) {
		v.Crls = append(v.Crls, crl)
	}
}

// Collector finds issuers and CRLs among what it is given first, and fetches the rest from the URLs the certificates name.
type Collector struct {
	Certificates []*x509.Certificate
	Crls         [][]byte
	// CrlSource, when set, issues further CRLs at collection time, as a CA under test can.
	CrlSource    func(now time.Time) ([][]byte, error)
	FetchOptions []fetch_config.Option
	Now          func() time.Time
}

func (c *Collector) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func isSelfSigned(certificate *x509.Certificate) bool {
	return bytes.Equal(certificate.RawIssuer, certificate.RawSubject) && certificate.CheckSignatureFrom(certificate) == nil
}

func (c *Collector) fetch(ctx context.Context, address string, maxSize int64) ([]byte, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("url parse: %w", err), address)
	}
	// AIA and CRL distribution points are plain HTTP by convention; anything else (ldap:) is skipped.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, nil
	}
	// The cap goes first, so a caller's own options can raise it.
	options := append([]fetch_config.Option{fetch_config.WithMaxResponseBodySize(maxSize)}, c.FetchOptions...)
	_, data, err := altshiftHttpUtils.Fetch(ctx, address, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch: %w", err), address)
	}
	return data, nil
}

func parseIssuerCertificates(data []byte) []*x509.Certificate {
	if certificate, err := x509.ParseCertificate(data); err == nil {
		return []*x509.Certificate{certificate}
	}
	var certificates []*x509.Certificate
	for rest := data; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if certificate, err := x509.ParseCertificate(block.Bytes); err == nil {
			certificates = append(certificates, certificate)
		}
	}
	return certificates
}

// issuer is the certificate that issued certificate: one of the collector's, a self-signed one first, else one its AIA
// names. A root may also come cross-signed by an older root, as DigiCert's timestamping tokens carry theirs: both verify
// the certificate, and only the self-signed one ends the chain at the root that is trusted.
func (c *Collector) issuer(ctx context.Context, certificate *x509.Certificate) (*x509.Certificate, error) {
	var found *x509.Certificate
	for _, candidate := range c.Certificates {
		if candidate == nil || !bytes.Equal(candidate.RawSubject, certificate.RawIssuer) || certificate.CheckSignatureFrom(candidate) != nil {
			continue
		}
		if isSelfSigned(candidate) {
			return candidate, nil
		}
		if found == nil {
			found = candidate
		}
	}
	if found != nil {
		return found, nil
	}

	// A failed fetch is one source less, not the end: another URL may still answer.
	var fetchErrs []error
	for _, address := range certificate.IssuingCertificateURL {
		data, err := c.fetch(ctx, address, maxCertificateResponseSize)
		if err != nil {
			fetchErrs = append(fetchErrs, fmt.Errorf("fetch (issuer): %w", err))
			continue
		}
		for _, candidate := range parseIssuerCertificates(data) {
			if bytes.Equal(candidate.RawSubject, certificate.RawIssuer) && certificate.CheckSignatureFrom(candidate) == nil {
				return candidate, nil
			}
		}
	}

	return nil, altshiftErrors.NewWithTrace(errors.Join(append([]error{fmt.Errorf("%w for %q", ErrNoIssuer, certificate.Subject.String())}, fetchErrs...)...))
}

// checkCrl accepts a CRL from the issuer that is current and does not list the certificate.
func (c *Collector) checkCrl(data []byte, certificate *x509.Certificate, issuer *x509.Certificate) (bool, error) {
	crl, err := x509.ParseRevocationList(data)
	if err != nil {
		return false, nil
	}
	if !bytes.Equal(crl.RawIssuer, issuer.RawSubject) || crl.CheckSignatureFrom(issuer) != nil {
		return false, nil
	}
	if now := c.now(); now.Add(clockSkew).Before(crl.ThisUpdate) || (!crl.NextUpdate.IsZero() && now.After(crl.NextUpdate)) {
		return false, nil
	}
	for _, entry := range crl.RevokedCertificateEntries {
		if entry.SerialNumber != nil && entry.SerialNumber.Cmp(certificate.SerialNumber) == 0 {
			return false, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %q", ErrRevoked, certificate.Subject.String()))
		}
	}
	return true, nil
}

func (c *Collector) crl(ctx context.Context, given [][]byte, certificate *x509.Certificate, issuer *x509.Certificate) ([]byte, error) {
	for _, candidate := range given {
		ok, err := c.checkCrl(candidate, certificate, issuer)
		if err != nil {
			return nil, fmt.Errorf("check crl (given): %w", err)
		}
		if ok {
			return candidate, nil
		}
	}

	var fetchErrs []error
	for _, address := range certificate.CRLDistributionPoints {
		data, err := c.fetch(ctx, address, maxCrlResponseSize)
		if err != nil {
			fetchErrs = append(fetchErrs, fmt.Errorf("fetch (crl): %w", err))
			continue
		}
		ok, err := c.checkCrl(data, certificate, issuer)
		if err != nil {
			return nil, fmt.Errorf("check crl (fetched): %w", err)
		}
		if ok {
			return data, nil
		}
	}

	return nil, altshiftErrors.NewWithTrace(errors.Join(append([]error{fmt.Errorf("%w for %q", ErrNoCurrentCrl, certificate.Subject.String())}, fetchErrs...)...))
}

// Collect walks each certificate up to a self-signed root, gathering every certificate on the way and a current CRL for each one that is not a root.
func (c *Collector) Collect(ctx context.Context, certificates ...*x509.Certificate) (*ValidationData, error) {
	if c == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("collector"))
	}

	given := c.Crls
	if c.CrlSource != nil {
		sourced, err := c.CrlSource(c.now())
		if err != nil {
			return nil, fmt.Errorf("crl source: %w", err)
		}
		given = slices.Concat(c.Crls, sourced)
	}

	data := &ValidationData{}
	for _, certificate := range certificates {
		if certificate == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("certificate"))
		}

		for current, length := certificate, 0; ; length++ {
			if length == maxChainLength {
				return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %q, past %d", ErrChainTooLong, certificate.Subject.String(), maxChainLength))
			}
			data.addCertificate(current)
			if isSelfSigned(current) {
				break
			}

			issuer, err := c.issuer(ctx, current)
			if err != nil {
				return nil, fmt.Errorf("issuer: %w", err)
			}
			crl, err := c.crl(ctx, given, current, issuer)
			if err != nil {
				return nil, fmt.Errorf("crl: %w", err)
			}
			data.addCrl(crl)
			current = issuer
		}
	}

	return data, nil
}
