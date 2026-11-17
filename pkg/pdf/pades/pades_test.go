package pades_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cms"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/pdf/file"
	"github.com/altshiftab/utils_go/pkg/pdf/object"
	"github.com/altshiftab/utils_go/pkg/pdf/pades"
	"github.com/altshiftab/utils_go/pkg/testing/test_ca"
	"github.com/altshiftab/utils_go/pkg/tsa"
	"github.com/altshiftab/utils_go/pkg/x509/validation_data"
)

var byteRangePattern = regexp.MustCompile(`/ByteRange \[(\d+) (\d+) (\d+) (\d+)\]`)

func onePagePdf() []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	bodies := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>",
	}
	offsets := make([]int, 0, len(bodies))
	for i, body := range bodies {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := out.Len()
	out.WriteString("xref\n0 4\n0000000000 65535 f\r\n")
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n\r\n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return out.Bytes()
}

type signature struct {
	byteRange [4]int
	contents  []byte
	covered   []byte
}

func signatures(t *testing.T, document []byte) []*signature {
	t.Helper()
	var found []*signature
	for _, match := range byteRangePattern.FindAllSubmatch(document, -1) {
		var byteRange [4]int
		for i := range byteRange {
			value, err := strconv.Atoi(string(match[i+1]))
			if err != nil {
				t.Fatalf("atoi: %v", err)
			}
			byteRange[i] = value
		}
		contents, err := hex.DecodeString(string(document[byteRange[1]+1 : byteRange[2]-1]))
		if err != nil {
			t.Fatalf("hex decode contents: %v", err)
		}
		// A signature covers the revision it was made in, which ends where its byte range does.
		covered, err := pades.ByteRangeContent(document[:byteRange[2]+byteRange[3]], byteRange)
		if err != nil {
			t.Fatalf("byte range content: %v", err)
		}
		found = append(found, &signature{byteRange: byteRange, contents: contents, covered: covered})
	}
	return found
}

func newSealer(t *testing.T) (*pades.Sealer, *test_ca.Authority) {
	t.Helper()
	authority, err := test_ca.New(time.Now(), "Test AB")
	if err != nil {
		t.Fatalf("test ca new: %v", err)
	}
	return &pades.Sealer{
		Signer:       authority.SealKey,
		Certificates: authority.Chain(),
		Timestamper:  authority,
		Collector: &validation_data.Collector{
			Certificates: []*x509.Certificate{authority.Root},
			CrlSource:    authority.Crls,
		},
	}, authority
}

func TestSeal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sealer, authority := newSealer(t)
	document := onePagePdf()

	sealed, err := sealer.Seal(ctx, document)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !bytes.HasPrefix(sealed, document) {
		t.Fatal("sealing changed the original bytes")
	}

	found := signatures(t, sealed)
	if len(found) != 2 {
		t.Fatalf("%d signatures, want the seal and the document timestamp", len(found))
	}
	seal, documentTimestamp := found[0], found[1]

	t.Run("the seal verifies over its byte range", func(t *testing.T) {
		t.Parallel()

		signedData, err := cms.ParseSignedData(seal.contents)
		if err != nil {
			t.Fatalf("parse signed data: %v", err)
		}
		signer, err := signedData.Verify(seal.covered)
		if err != nil {
			t.Fatalf("verify: %v", err)
		}
		if !signer.Equal(authority.Seal) {
			t.Error("the seal is not the sealing certificate's")
		}

		info := signedData.SignerInfos[0]
		if _, ok := info.Attribute(cms.OidAttributeSigningCertificateV2); !ok {
			t.Error("no signing-certificate-v2, which PAdES requires")
		}
		if len(info.UnsignedAttributes) != 1 || !info.UnsignedAttributes[0].Type.Equal(cms.OidAttributeTimeStampToken) {
			t.Fatal("no signature timestamp")
		}
		token, err := tsa.ParseToken(info.UnsignedAttributes[0].Values[0].FullBytes)
		if err != nil {
			t.Fatalf("parse token: %v", err)
		}
		signatureDigest := sha256.Sum256(info.Signature)
		if !bytes.Equal(token.Info.HashedMessage, signatureDigest[:]) {
			t.Error("the signature timestamp is not over the signature value")
		}
	})

	t.Run("the document timestamp covers the whole file", func(t *testing.T) {
		t.Parallel()

		if documentTimestamp.byteRange[2]+documentTimestamp.byteRange[3] != len(sealed) {
			t.Errorf("byte range %v leaves the end of a %d-byte file out", documentTimestamp.byteRange, len(sealed))
		}
		token, err := tsa.ParseToken(documentTimestamp.contents)
		if err != nil {
			t.Fatalf("parse token: %v", err)
		}
		coveredDigest := sha256.Sum256(documentTimestamp.covered)
		if !bytes.Equal(token.Info.HashedMessage, coveredDigest[:]) {
			t.Error("the document timestamp is not over its byte range")
		}
	})

	t.Run("the dss holds both chains and their crls", func(t *testing.T) {
		t.Parallel()

		document, err := file.Open(sealed)
		if err != nil {
			t.Fatalf("file open: %v", err)
		}
		catalog, _, err := document.Catalog()
		if err != nil {
			t.Fatalf("catalog: %v", err)
		}
		dss, err := document.ResolveDictionary(catalog["DSS"])
		if err != nil {
			t.Fatalf("resolve dss: %v", err)
		}
		// The seal, the issuing CA, the root and the timestamping unit; a CRL from each CA.
		if certificates, _ := dss["Certs"].(object.Array); len(certificates) != 4 {
			t.Errorf("%d certificates in the dss, want 4", len(certificates))
		}
		if crls, _ := dss["CRLs"].(object.Array); len(crls) != 2 {
			t.Errorf("%d crls in the dss, want 2", len(crls))
		}
		form, ok := catalog["AcroForm"].(object.Dictionary)
		if fields, _ := form["Fields"].(object.Array); !ok || len(fields) != 2 {
			t.Errorf("the form does not list both fields: %v", catalog["AcroForm"])
		}
	})

	t.Run("a changed byte breaks the seal", func(t *testing.T) {
		t.Parallel()

		tampered := bytes.Clone(sealed)
		index := bytes.Index(tampered, []byte("/MediaBox [0 0 595"))
		// 595 becomes 495: one byte, inside the original revision.
		tampered[index+len("/MediaBox [0 0 ")] = '4'
		content, err := pades.ByteRangeContent(tampered[:seal.byteRange[2]+seal.byteRange[3]], seal.byteRange)
		if err != nil {
			t.Fatalf("byte range content: %v", err)
		}
		signedData, err := cms.ParseSignedData(seal.contents)
		if err != nil {
			t.Fatalf("parse signed data: %v", err)
		}
		if _, err := signedData.Verify(content); !errors.Is(err, altshiftErrors.ErrVerificationError) {
			t.Errorf("verify of a changed document = %v, want a verification error", err)
		}
	})
}

func TestSealRejects(t *testing.T) {
	t.Parallel()

	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	testCases := []struct {
		name     string
		modify   func(sealer *pades.Sealer)
		document []byte
		wantErr  error
	}{
		{name: "a key the certificate is not for", modify: func(s *pades.Sealer) { s.Signer = otherKey }, document: onePagePdf(), wantErr: pades.ErrKeyMismatch},
		{name: "no timestamper", modify: func(s *pades.Sealer) { s.Timestamper = nil }, document: onePagePdf()},
		{name: "no certificates", modify: func(s *pades.Sealer) { s.Certificates = nil }, document: onePagePdf()},
		{name: "no collector", modify: func(s *pades.Sealer) { s.Collector = nil }, document: onePagePdf()},
		{name: "not a pdf", modify: func(*pades.Sealer) {}, document: []byte("hello"), wantErr: altshiftErrors.ErrParseError},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			sealer, _ := newSealer(t)
			testCase.modify(sealer)
			_, err := sealer.Seal(context.Background(), testCase.document)
			if err == nil {
				t.Fatalf("%s: seal succeeded", testCase.name)
			}
			if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
				t.Errorf("%s: seal = %v, want %v", testCase.name, err, testCase.wantErr)
			}
		})
	}
}

// TestSealWithASelfSignedCertificate seals with a certificate the key issued itself, as an organization's interim seal
// certificate is: no CA, a digital signature and content commitment key usage, and no CRL to find. The chain is that one
// certificate, the DSS carries it, and the seal verifies by it.
func TestSealWithASelfSignedCertificate(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Country: []string{"SE"}, Organization: []string{"Test AB"}, CommonName: "Test AB e-seal (interim)"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatalf("x509 create certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509 parse certificate: %v", err)
	}

	// The timestamps are the test authority's, whose chain has CRLs; the seal's chain has none to give.
	sealer, authority := newSealer(t)
	sealer.Signer = key
	sealer.Certificates = []*x509.Certificate{certificate}
	sealer.Collector.Certificates = append(sealer.Collector.Certificates, authority.Issuing)

	document := onePagePdf()
	sealed, err := sealer.Seal(context.Background(), document)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	found := signatures(t, sealed)
	if len(found) != 2 {
		t.Fatalf("%d signatures, want the seal and the document timestamp", len(found))
	}
	signedData, err := cms.ParseSignedData(found[0].contents)
	if err != nil {
		t.Fatalf("parse signed data: %v", err)
	}
	signer, err := signedData.Verify(found[0].covered)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !signer.Equal(certificate) {
		t.Errorf("signed by %q, want the self-signed certificate", signer.Subject)
	}
	if !bytes.Contains(sealed, certificate.Raw) {
		t.Error("the self-signed certificate is not in the sealed file")
	}
	if !bytes.Contains(sealed, authority.Tsa.Raw) {
		t.Error("the timestamping unit's certificate is not in the sealed file")
	}
}

// countingTimestamper counts the timestamps asked of the test authority.
type countingTimestamper struct {
	authority *test_ca.Authority
	count     int
}

func (c *countingTimestamper) Timestamp(ctx context.Context, digest []byte) ([]byte, error) {
	c.count++
	return c.authority.Timestamp(ctx, digest)
}

// TestSealRefusesACertificateNotValidNow refuses a sealing certificate that has expired or is not valid yet, as of the
// sealer's clock, before anything is timestamped; one valid at that moment seals.
func TestSealRefusesACertificateNotValidNow(t *testing.T) {
	t.Parallel()

	sealer, authority := newSealer(t)
	seal := authority.Seal

	testCases := []struct {
		name    string
		now     time.Time
		wantErr error
	}{
		{name: "valid", now: seal.NotBefore.Add(time.Hour)},
		{name: "expired", now: seal.NotAfter.Add(time.Second), wantErr: pades.ErrCertificateNotValid},
		{name: "not valid yet", now: seal.NotBefore.Add(-time.Second), wantErr: pades.ErrCertificateNotValid},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			timestamper := &countingTimestamper{authority: authority}
			clocked := *sealer
			clocked.Timestamper = timestamper
			clocked.Now = func() time.Time { return testCase.now }
			// The CRLs are issued at the moment collected, as a CA's are current when fetched.
			collector := *sealer.Collector
			collector.Now = clocked.Now
			collector.CrlSource = func(time.Time) ([][]byte, error) { return authority.Crls(testCase.now) }
			clocked.Collector = &collector

			_, err := clocked.Seal(context.Background(), onePagePdf())
			if testCase.wantErr == nil {
				if err != nil {
					t.Fatalf("%s: seal: %v", testCase.name, err)
				}
				return
			}
			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("%s: seal = %v, want %v", testCase.name, err, testCase.wantErr)
			}
			if timestamper.count != 0 {
				t.Errorf("%s: %d timestamps asked for", testCase.name, timestamper.count)
			}
		})
	}
}

func TestByteRangeContent(t *testing.T) {
	t.Parallel()

	document := []byte("0123456789")
	testCases := []struct {
		name      string
		byteRange [4]int
		want      string
		wantErr   bool
	}{
		{name: "a gap in the middle", byteRange: [4]int{0, 3, 6, 4}, want: "0126789"},
		{name: "not from the start", byteRange: [4]int{1, 3, 6, 4}, wantErr: true},
		{name: "short of the end", byteRange: [4]int{0, 3, 6, 3}, wantErr: true},
		{name: "overlapping", byteRange: [4]int{0, 6, 3, 7}, wantErr: true},
		{name: "a negative length that makes the sum fit", byteRange: [4]int{0, 3, 15, -5}, wantErr: true},
		{name: "an offset past the end", byteRange: [4]int{0, 3, 11, -1}, wantErr: true},
		{name: "an overflowing sum", byteRange: [4]int{0, 3, math.MaxInt, 10 - math.MaxInt}, wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := pades.ByteRangeContent(document, testCase.byteRange)
			if (err != nil) != testCase.wantErr || string(got) != testCase.want {
				t.Errorf("%s: byte range content = %q, %v", testCase.name, got, err)
			}
		})
	}
}

// TestSealNamesFieldsApart seals a document that already has a field named Seal, which must not be given a second.
func TestSealNamesFieldsApart(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	bodies := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Annots [4 0 R] >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (Seal) /P 3 0 R /Rect [0 0 10 10] >>",
	}
	offsets := make([]int, 0, len(bodies))
	for i, body := range bodies {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := out.Len()
	out.WriteString("xref\n0 5\n0000000000 65535 f\r\n")
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n\r\n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)

	sealer, _ := newSealer(t)
	sealed, err := sealer.Seal(context.Background(), out.Bytes())
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	document, err := file.Open(sealed)
	if err != nil {
		t.Fatalf("file open: %v", err)
	}
	catalog, _, err := document.Catalog()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	form, err := document.ResolveDictionary(catalog["AcroForm"])
	if err != nil {
		t.Fatalf("acro form: %v", err)
	}
	fields, err := document.ResolveArray(form["Fields"])
	if err != nil {
		t.Fatalf("fields: %v", err)
	}
	var names []string
	for _, field := range fields {
		dictionary, err := document.ResolveDictionary(field)
		if err != nil {
			t.Fatalf("field: %v", err)
		}
		title, _ := dictionary["T"].(*object.String)
		names = append(names, string(title.Value))
	}
	if want := []string{"Seal", "Seal2", "DocumentTimestamp"}; !slices.Equal(names, want) {
		t.Errorf("field names %v, want %v", names, want)
	}
}

func qcCertificate(t *testing.T, statements []any) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "qc"}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	if statements != nil {
		value, err := asn1.Marshal(statements)
		if err != nil {
			t.Fatalf("asn1 marshal: %v", err)
		}
		template.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3}, Value: value}}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), crypto.Signer(key))
	if err != nil {
		t.Fatalf("x509 create certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509 parse certificate: %v", err)
	}
	return certificate
}

type qcStatement struct {
	Id   asn1.ObjectIdentifier
	Info any `asn1:"optional"`
}

type qcStatementWithoutInfo struct {
	Id asn1.ObjectIdentifier
}

func TestQualifiedForSeals(t *testing.T) {
	t.Parallel()

	compliance := qcStatementWithoutInfo{Id: asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}}
	sealType := qcStatement{Id: asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6}, Info: []asn1.ObjectIdentifier{{0, 4, 0, 1862, 1, 6, 2}}}
	signatureType := qcStatement{Id: asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6}, Info: []asn1.ObjectIdentifier{{0, 4, 0, 1862, 1, 6, 1}}}

	testCases := []struct {
		name       string
		statements []any
		want       bool
	}{
		{name: "qualified seal", statements: []any{compliance, sealType}, want: true},
		{name: "qualified signature, not a seal", statements: []any{compliance, signatureType}},
		{name: "a seal type without compliance", statements: []any{sealType}},
		{name: "no qc statements"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := pades.QualifiedForSeals(qcCertificate(t, testCase.statements)); got != testCase.want {
				t.Errorf("%s: qualified for seals = %v", testCase.name, got)
			}
		})
	}
	if pades.QualifiedForSeals(nil) {
		t.Error("nil: qualified for seals")
	}
}

func fieldsOf(t *testing.T, sealed []byte) map[string]object.Dictionary {
	t.Helper()
	document, err := file.Open(sealed)
	if err != nil {
		t.Fatalf("file open: %v", err)
	}
	catalog, _, err := document.Catalog()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	form, err := document.ResolveDictionary(catalog["AcroForm"])
	if err != nil {
		t.Fatalf("acro form: %v", err)
	}
	fields, err := document.ResolveArray(form["Fields"])
	if err != nil {
		t.Fatalf("fields: %v", err)
	}
	byName := map[string]object.Dictionary{}
	for _, field := range fields {
		dictionary, err := document.ResolveDictionary(field)
		if err != nil {
			t.Fatalf("field: %v", err)
		}
		title, _ := dictionary["T"].(*object.String)
		if title != nil {
			// The appearance is resolved here, while the file is open.
			if appearance, ok := dictionary["AP"].(object.Dictionary); ok {
				normal, err := document.ResolveDictionary(appearance["N"])
				if err != nil {
					t.Fatalf("appearance: %v", err)
				}
				dictionary = dictionary.Clone()
				dictionary["AP"] = normal
			}
			byName[string(title.Value)] = dictionary
		}
	}
	return byName
}

// TestSealRectangle places the seal's field over a box on the last page, with an empty appearance, and leaves the document timestamp off the page.
func TestSealRectangle(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		rectangle *pades.Rectangle
		wantRect  object.Array
		wantErr   error
	}{
		{name: "off the page", wantRect: object.Array{object.Integer(0), object.Integer(0), object.Integer(0), object.Integer(0)}},
		{
			name:      "over the mark",
			rectangle: &pades.Rectangle{Left: 100, Bottom: 200, Right: 175.5, Top: 275.5},
			wantRect:  object.Array{object.Real(100), object.Real(200), object.Real(175.5), object.Real(275.5)},
		},
		{name: "inside out", rectangle: &pades.Rectangle{Left: 100, Bottom: 200, Right: 90, Top: 275}, wantErr: pades.ErrInvalidRectangle},
		{name: "off the edge", rectangle: &pades.Rectangle{Left: -5, Bottom: 200, Right: 90, Top: 275}, wantErr: pades.ErrInvalidRectangle},
		{name: "not a number", rectangle: &pades.Rectangle{Left: math.NaN(), Bottom: 200, Right: 90, Top: 275}, wantErr: pades.ErrInvalidRectangle},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			sealer, _ := newSealer(t)
			sealer.SealRectangle = testCase.rectangle
			sealed, err := sealer.Seal(context.Background(), onePagePdf())
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("%s: seal = %v, want %v", testCase.name, err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: seal: %v", testCase.name, err)
			}

			fields := fieldsOf(t, sealed)
			seal := fields["Seal"]
			if fmt.Sprint(seal["Rect"]) != fmt.Sprint(testCase.wantRect) {
				t.Errorf("%s: seal /Rect %v, want %v", testCase.name, seal["Rect"], testCase.wantRect)
			}
			appearance, hasAppearance := seal["AP"].(object.Dictionary)
			if (testCase.rectangle != nil) != hasAppearance {
				t.Errorf("%s: appearance %v", testCase.name, seal["AP"])
			}
			if hasAppearance {
				box, _ := appearance["BBox"].(object.Array)
				if fmt.Sprint(box) != fmt.Sprint(object.Array{object.Real(0), object.Real(0), object.Real(75.5), object.Real(75.5)}) {
					t.Errorf("%s: appearance /BBox %v", testCase.name, box)
				}
			}
			if timestamp := fields["DocumentTimestamp"]; fmt.Sprint(timestamp["Rect"]) != fmt.Sprint(object.Array{object.Integer(0), object.Integer(0), object.Integer(0), object.Integer(0)}) {
				t.Errorf("%s: the document timestamp is on the page: %v", testCase.name, timestamp["Rect"])
			}
		})
	}
}
