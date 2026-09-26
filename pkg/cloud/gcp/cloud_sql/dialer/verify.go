package dialer

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/connection_name"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// verifyConnectionFunc checks the server certificate against the instance's CA, then its name: the
// instance DNS name when the instance has one, else (and as a fallback) the legacy
// `project:instance` common name that certificates from the per-instance CA carry. It runs as
// VerifyConnection so that a resumed session is checked too.
func verifyConnectionFunc(
	serverName string,
	connectionName *connection_name.ConnectionName,
	roots *x509.CertPool,
) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		certificates := state.PeerCertificates
		if len(certificates) == 0 {
			return altshiftErrors.NewWithTrace(fmt.Errorf("%w: no server certificate", altshiftErrors.ErrVerificationError))
		}

		intermediates := x509.NewCertPool()
		for _, certificate := range certificates[1:] {
			intermediates.AddCert(certificate)
		}

		serverCertificate := certificates[0]
		if _, err := serverCertificate.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates}); err != nil {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: server certificate chain: %w", altshiftErrors.ErrVerificationError, err),
				connectionName.String(),
			)
		}

		serverName = strings.TrimSuffix(serverName, ".")
		if serverName != "" {
			if serverCertificate.VerifyHostname(serverName) == nil || serverCertificate.VerifyHostname(serverName+".") == nil {
				return nil
			}
		}

		return verifyCommonName(connectionName, serverCertificate)
	}
}

func verifyCommonName(connectionName *connection_name.ConnectionName, certificate *x509.Certificate) error {
	expected := connectionName.Project + ":" + connectionName.Instance
	if certificate.Subject.CommonName != expected {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: server certificate names neither the instance DNS name nor %q (CN %q)",
				altshiftErrors.ErrVerificationError, expected, certificate.Subject.CommonName,
			),
			connectionName.String(),
		)
	}

	return nil
}
