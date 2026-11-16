// Package pades seals a PDF with a PAdES baseline signature at level B-LTA: a CAdES signature with a signature timestamp, the validation data in a document security store, and a document timestamp over all of it.
//
// Each step is an incremental update, so every byte of the document as given stays a prefix of the sealed file.
package pades

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/cms"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/pdf/file"
	"github.com/altshiftab/utils_go/pkg/pdf/object"
	"github.com/altshiftab/utils_go/pkg/tsa"
	"github.com/altshiftab/utils_go/pkg/x509/validation_data"
)

const (
	// The reserved /Contents sizes, in bytes before hex encoding: room for the chain and a timestamp token with its own.
	signatureContentsSize = 32 * 1024
	timestampContentsSize = 16 * 1024

	byteRangePlaceholder = "[0 0000000000 0000000000 0000000000]"

	sealFieldName      = "Seal"
	timestampFieldName = "DocumentTimestamp"

	// widgetFlags is Print and Locked: an invisible field that cannot be edited.
	widgetFlags = 132
	// sigFlags is SignaturesExist and AppendOnly.
	sigFlags = 3
)

// Errors in the sealer's own setup; they are not the document's fault, so none is classed as client input.
var (
	ErrKeyMismatch         = errors.New("the sealing certificate is not for the signer's key")
	ErrReservationTooSmall = errors.New("signature contents larger than reserved")
)

// ErrInvalidRectangle is a field rectangle that is not a box on a page.
var ErrInvalidRectangle = errors.New("invalid field rectangle")

// Rectangle is a box on a page, in points from its lower-left corner.
type Rectangle struct {
	Left   float64
	Bottom float64
	Right  float64
	Top    float64
}

func (r *Rectangle) validate() error {
	for _, value := range []float64{r.Left, r.Bottom, r.Right, r.Top} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %v", ErrInvalidRectangle, *r))
		}
	}
	if r.Right <= r.Left || r.Top <= r.Bottom {
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %v", ErrInvalidRectangle, *r))
	}
	return nil
}

// Sealer seals documents with one key and its certificate chain.
type Sealer struct {
	Signer crypto.Signer
	// Certificates is the sealing certificate first, then its issuers.
	Certificates []*x509.Certificate
	Timestamper  tsa.Timestamper
	Collector    *validation_data.Collector
	// SealRectangle places the seal's field on the last page, over what the page prints for it, so a reader opens its check of the seal there; nil leaves the field off the page.
	SealRectangle *Rectangle
	Now           func() time.Time
}

func (s *Sealer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type placeholder struct {
	byteRangeOffset int
	contentsOffset  int
	contentsLength  int
}

func pdfDate(t time.Time) string {
	return t.UTC().Format("D:20060102150405Z")
}

// appendToArray adds the reference to holder's array under key, writing the array itself when it is an indirect object; it reports whether holder changed.
func appendToArray(document *file.File, update *file.Update, holder object.Dictionary, key object.Name, reference *object.Reference) (bool, error) {
	switch value := holder[key].(type) {
	case nil:
		holder[key] = object.Array{reference}
		return true, nil
	case object.Array:
		holder[key] = append(slices.Clone(value), reference)
		return true, nil
	case *object.Reference:
		array, err := document.ResolveArray(value)
		if err != nil {
			return false, fmt.Errorf("resolve array (%s): %w", key, err)
		}
		if err := update.Set(value, append(slices.Clone(array), reference)); err != nil {
			return false, fmt.Errorf("update set (%s): %w", key, err)
		}
		return false, nil
	default:
		return false, altshiftErrors.NewWithTrace(fmt.Errorf("%w: /%s is a %T", object.ErrSyntax, key, value))
	}
}

// unusedFieldName returns the name, or the name with the first number that makes it one no top-level field has.
func unusedFieldName(document *file.File, catalog object.Dictionary, name string) (string, error) {
	taken := map[string]bool{}
	if form, ok := catalog["AcroForm"]; ok {
		formDictionary, err := document.ResolveDictionary(form)
		if err != nil {
			return "", fmt.Errorf("resolve dictionary (acro form): %w", err)
		}
		if fieldsValue, ok := formDictionary["Fields"]; ok {
			fields, err := document.ResolveArray(fieldsValue)
			if err != nil {
				return "", fmt.Errorf("resolve array (fields): %w", err)
			}
			for _, field := range fields {
				fieldDictionary, err := document.ResolveDictionary(field)
				if err != nil {
					continue
				}
				if title, ok := fieldDictionary["T"].(*object.String); ok && title != nil {
					taken[string(title.Value)] = true
				}
			}
		}
	}
	candidate := name
	for number := 2; taken[candidate]; number++ {
		candidate = fmt.Sprintf("%s%d", name, number)
	}
	return candidate, nil
}

// addSignatureField appends an update with a signature dictionary, its invisible field on the last page, and the form that lists it.
func addSignatureField(data []byte, fieldName string, dictionaryEntries string, contentsSize int, rectangle *Rectangle) (*file.Written, *placeholder, error) {
	document, err := file.Open(data)
	if err != nil {
		return nil, nil, fmt.Errorf("file open: %w", err)
	}
	catalog, catalogReference, err := document.Catalog()
	if err != nil {
		return nil, nil, fmt.Errorf("catalog: %w", err)
	}
	tree, err := document.PageTree()
	if err != nil {
		return nil, nil, fmt.Errorf("page tree: %w", err)
	}
	if len(tree.Pages) == 0 {
		return nil, nil, altshiftErrors.NewWithTrace(empty_error.New("pages"))
	}
	lastPage := tree.Pages[len(tree.Pages)-1]

	fieldName, err = unusedFieldName(document, catalog, fieldName)
	if err != nil {
		return nil, nil, fmt.Errorf("unused field name: %w", err)
	}

	update := document.NewUpdate()
	signatureReference := update.NewObject()
	fieldReference := update.NewObject()

	contentsPlaceholder := "<" + strings.Repeat("0", 2*contentsSize) + ">"
	signatureBody := "<< " + dictionaryEntries + " /ByteRange "
	byteRangeRelative := len(signatureBody)
	signatureBody += byteRangePlaceholder + " /Contents "
	contentsRelative := len(signatureBody)
	signatureBody += contentsPlaceholder + " >>"
	update.SetRaw(signatureReference, []byte(signatureBody))

	field := object.Dictionary{
		"Type":    object.Name("Annot"),
		"Subtype": object.Name("Widget"),
		"FT":      object.Name("Sig"),
		"T":       &object.String{Value: []byte(fieldName)},
		"V":       signatureReference,
		"P":       lastPage.Reference,
		"Rect":    object.Array{object.Integer(0), object.Integer(0), object.Integer(0), object.Integer(0)},
		"F":       object.Integer(widgetFlags),
	}
	if rectangle != nil {
		field["Rect"] = object.Array{object.Real(rectangle.Left), object.Real(rectangle.Bottom), object.Real(rectangle.Right), object.Real(rectangle.Top)}
		// A field on the page needs an appearance; an empty one, since the page itself prints the mark beneath.
		appearanceReference := update.NewObject()
		appearance := &object.Stream{Dictionary: object.Dictionary{
			"Type":      object.Name("XObject"),
			"Subtype":   object.Name("Form"),
			"BBox":      object.Array{object.Real(0), object.Real(0), object.Real(rectangle.Right - rectangle.Left), object.Real(rectangle.Top - rectangle.Bottom)},
			"Resources": object.Dictionary{},
		}}
		if err := update.Set(appearanceReference, appearance); err != nil {
			return nil, nil, fmt.Errorf("update set (appearance): %w", err)
		}
		field["AP"] = object.Dictionary{"N": appearanceReference}
	}
	if err := update.Set(fieldReference, field); err != nil {
		return nil, nil, fmt.Errorf("update set (field): %w", err)
	}

	page := lastPage.Dictionary.Clone()
	pageChanged, err := appendToArray(document, update, page, "Annots", fieldReference)
	if err != nil {
		return nil, nil, fmt.Errorf("append to array (annots): %w", err)
	}
	if pageChanged {
		if err := update.Set(lastPage.Reference, page); err != nil {
			return nil, nil, fmt.Errorf("update set (page): %w", err)
		}
	}

	switch form := catalog["AcroForm"].(type) {
	case nil:
		catalog = catalog.Clone()
		catalog["AcroForm"] = object.Dictionary{"Fields": object.Array{fieldReference}, "SigFlags": object.Integer(sigFlags)}
		if err := update.Set(catalogReference, catalog); err != nil {
			return nil, nil, fmt.Errorf("update set (catalog): %w", err)
		}
	case object.Dictionary:
		form = form.Clone()
		if _, err := appendToArray(document, update, form, "Fields", fieldReference); err != nil {
			return nil, nil, fmt.Errorf("append to array (fields): %w", err)
		}
		form["SigFlags"] = object.Integer(sigFlags)
		catalog = catalog.Clone()
		catalog["AcroForm"] = form
		if err := update.Set(catalogReference, catalog); err != nil {
			return nil, nil, fmt.Errorf("update set (catalog): %w", err)
		}
	case *object.Reference:
		formDictionary, err := document.ResolveDictionary(form)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve dictionary (acro form): %w", err)
		}
		formDictionary = formDictionary.Clone()
		if _, err := appendToArray(document, update, formDictionary, "Fields", fieldReference); err != nil {
			return nil, nil, fmt.Errorf("append to array (fields): %w", err)
		}
		formDictionary["SigFlags"] = object.Integer(sigFlags)
		if err := update.Set(form, formDictionary); err != nil {
			return nil, nil, fmt.Errorf("update set (acro form): %w", err)
		}
	default:
		return nil, nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: /AcroForm is a %T", object.ErrSyntax, form))
	}

	written, err := update.Write()
	if err != nil {
		return nil, nil, fmt.Errorf("update write: %w", err)
	}

	bodyOffset := written.BodyOffsets[signatureReference.Number]
	return written, &placeholder{
		byteRangeOffset: bodyOffset + byteRangeRelative,
		contentsOffset:  bodyOffset + contentsRelative,
		contentsLength:  len(contentsPlaceholder),
	}, nil
}

// fill writes the byte range, which excludes only the /Contents string, and then the contents made from the digest of that range.
func fill(data []byte, p *placeholder, contents func(digest []byte) ([]byte, error)) ([]byte, error) {
	afterContents := p.contentsOffset + p.contentsLength
	byteRange := fmt.Sprintf("[0 %d %d %d]", p.contentsOffset, afterContents, len(data)-afterContents)
	if len(byteRange) > len(byteRangePlaceholder) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: byte range %s does not fit its placeholder", ErrReservationTooSmall, byteRange))
	}
	copy(data[p.byteRangeOffset:], byteRange+strings.Repeat(" ", len(byteRangePlaceholder)-len(byteRange)))

	hash := sha256.New()
	hash.Write(data[:p.contentsOffset])
	hash.Write(data[afterContents:])

	value, err := contents(hash.Sum(nil))
	if err != nil {
		return nil, fmt.Errorf("contents: %w", err)
	}
	encoded := hex.EncodeToString(value)
	if len(encoded) > p.contentsLength-2 {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %d bytes for %d reserved", ErrReservationTooSmall, len(value), (p.contentsLength-2)/2))
	}
	copy(data[p.contentsOffset+1:], encoded)
	return data, nil
}

// ByteRangeContent returns what a signature with this byte range covers in the revision it signed, which ends where the range does.
func ByteRangeContent(data []byte, byteRange [4]int) ([]byte, error) {
	// Each bound checked on its own, so no sum can overflow into a range that passes.
	if byteRange[0] != 0 || byteRange[1] < 0 || byteRange[3] < 0 || byteRange[2] < byteRange[1] || byteRange[2] > len(data) || byteRange[3] != len(data)-byteRange[2] {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: byte range %v does not span the document", altshiftErrors.ErrValidationError, byteRange))
	}
	return slices.Concat(data[:byteRange[1]], data[byteRange[2]:]), nil
}

func (s *Sealer) sign(ctx context.Context, data []byte) ([]byte, []byte, error) {
	date, err := object.Append(nil, &object.String{Value: []byte(pdfDate(s.now()))})
	if err != nil {
		return nil, nil, fmt.Errorf("object append (date): %w", err)
	}
	entries := "/Type /Sig /Filter /Adobe.PPKLite /SubFilter /ETSI.CAdES.detached /M " + string(date)
	written, p, err := addSignatureField(data, sealFieldName, entries, signatureContentsSize, s.SealRectangle)
	if err != nil {
		return nil, nil, fmt.Errorf("add signature field: %w", err)
	}

	var token []byte
	sealed, err := fill(written.Data, p, func(digest []byte) ([]byte, error) {
		signerInfo, err := cms.NewSignerInfo(s.Signer, s.Certificates[0], cms.OidData, digest)
		if err != nil {
			return nil, fmt.Errorf("cms new signer info: %w", err)
		}

		signatureDigest := sha256.Sum256(signerInfo.Signature())
		token, err = s.Timestamper.Timestamp(ctx, signatureDigest[:])
		if err != nil {
			return nil, fmt.Errorf("timestamper timestamp (signature): %w", err)
		}
		signerInfo.UnsignedAttributes = append(
			signerInfo.UnsignedAttributes,
			&cms.Attribute{Type: cms.OidAttributeTimeStampToken, Values: []asn1.RawValue{{FullBytes: token}}},
		)

		signedData, err := cms.MarshalSignedData(cms.OidData, nil, s.Certificates, signerInfo)
		if err != nil {
			return nil, fmt.Errorf("cms marshal signed data: %w", err)
		}
		return signedData, nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("fill (signature): %w", err)
	}
	return sealed, token, nil
}

// addDss appends the document security store holding the validation data.
func addDss(data []byte, validationData *validation_data.ValidationData) ([]byte, error) {
	document, err := file.Open(data)
	if err != nil {
		return nil, fmt.Errorf("file open: %w", err)
	}
	catalog, catalogReference, err := document.Catalog()
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	if _, ok := catalog["DSS"]; ok {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: the document already has a DSS", altshiftErrors.ErrValidationError))
	}

	update := document.NewUpdate()
	streams := func(items [][]byte) (object.Array, error) {
		array := object.Array{}
		for _, item := range items {
			reference := update.NewObject()
			if err := update.Set(reference, &object.Stream{Dictionary: object.Dictionary{}, Data: item}); err != nil {
				return nil, fmt.Errorf("update set (dss stream): %w", err)
			}
			array = append(array, reference)
		}
		return array, nil
	}

	certificates := make([][]byte, 0, len(validationData.Certificates))
	for _, certificate := range validationData.Certificates {
		certificates = append(certificates, certificate.Raw)
	}
	certificateStreams, err := streams(certificates)
	if err != nil {
		return nil, err
	}
	dss := object.Dictionary{"Type": object.Name("DSS"), "Certs": certificateStreams}
	if len(validationData.Crls) != 0 {
		crlStreams, err := streams(validationData.Crls)
		if err != nil {
			return nil, err
		}
		dss["CRLs"] = crlStreams
	}
	dssReference := update.NewObject()
	if err := update.Set(dssReference, dss); err != nil {
		return nil, fmt.Errorf("update set (dss): %w", err)
	}

	catalog = catalog.Clone()
	catalog["DSS"] = dssReference
	if err := update.Set(catalogReference, catalog); err != nil {
		return nil, fmt.Errorf("update set (catalog): %w", err)
	}

	written, err := update.Write()
	if err != nil {
		return nil, fmt.Errorf("update write: %w", err)
	}
	return written.Data, nil
}

func (s *Sealer) documentTimestamp(ctx context.Context, data []byte) ([]byte, error) {
	written, p, err := addSignatureField(data, timestampFieldName, "/Type /DocTimeStamp /Filter /Adobe.PPKLite /SubFilter /ETSI.RFC3161", timestampContentsSize, nil)
	if err != nil {
		return nil, fmt.Errorf("add signature field: %w", err)
	}
	stamped, err := fill(written.Data, p, func(digest []byte) ([]byte, error) {
		token, err := s.Timestamper.Timestamp(ctx, digest)
		if err != nil {
			return nil, fmt.Errorf("timestamper timestamp (document): %w", err)
		}
		return token, nil
	})
	if err != nil {
		return nil, fmt.Errorf("fill (document timestamp): %w", err)
	}
	return stamped, nil
}

// Seal returns the document with a B-LTA seal appended as three incremental updates.
func (s *Sealer) Seal(ctx context.Context, data []byte) ([]byte, error) {
	if s == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("sealer"))
	}
	if s.Signer == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("signer"))
	}
	if len(s.Certificates) == 0 || s.Certificates[0] == nil {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("certificates"))
	}
	if s.Timestamper == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("timestamper"))
	}
	if s.Collector == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("collector"))
	}
	if s.SealRectangle != nil {
		if err := s.SealRectangle.validate(); err != nil {
			return nil, fmt.Errorf("seal rectangle validate: %w", err)
		}
	}
	publicKey, ok := s.Certificates[0].PublicKey.(interface{ Equal(x crypto.PublicKey) bool })
	if !ok || !publicKey.Equal(s.Signer.Public()) {
		return nil, altshiftErrors.NewWithTrace(ErrKeyMismatch)
	}

	signed, signatureToken, err := s.sign(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	token, err := tsa.ParseToken(signatureToken)
	if err != nil {
		return nil, fmt.Errorf("tsa parse token: %w", err)
	}

	collector := *s.Collector
	collector.Certificates = slices.Concat(s.Collector.Certificates, s.Certificates, token.SignedData.Certificates)
	validationData, err := collector.Collect(ctx, s.Certificates[0], token.Certificate)
	if err != nil {
		return nil, fmt.Errorf("collector collect: %w", err)
	}

	withDss, err := addDss(signed, validationData)
	if err != nil {
		return nil, fmt.Errorf("add dss: %w", err)
	}

	sealed, err := s.documentTimestamp(ctx, withDss)
	if err != nil {
		return nil, fmt.Errorf("document timestamp: %w", err)
	}

	if !bytes.HasPrefix(sealed, data) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: sealing changed the original bytes", altshiftErrors.ErrVerificationError))
	}
	return sealed, nil
}

var (
	oidQcStatements = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3}
	oidQcCompliance = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}
	oidQcType       = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6}
	oidQcTypeESeal  = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6, 2}
)

type qcStatement struct {
	Id   asn1.ObjectIdentifier
	Info asn1.RawValue `asn1:"optional"`
}

// QualifiedForSeals reports whether the certificate states, in its QC statements, that it is an EU qualified certificate for electronic seals.
func QualifiedForSeals(certificate *x509.Certificate) bool {
	if certificate == nil {
		return false
	}
	for _, extension := range certificate.Extensions {
		if !extension.Id.Equal(oidQcStatements) {
			continue
		}
		var statements []qcStatement
		if _, err := asn1.Unmarshal(extension.Value, &statements); err != nil {
			return false
		}
		compliant, seal := false, false
		for _, statement := range statements {
			switch {
			case statement.Id.Equal(oidQcCompliance):
				compliant = true
			case statement.Id.Equal(oidQcType):
				var qcTypes []asn1.ObjectIdentifier
				if _, err := asn1.Unmarshal(statement.Info.FullBytes, &qcTypes); err == nil {
					seal = slices.ContainsFunc(qcTypes, oidQcTypeESeal.Equal)
				}
			}
		}
		return compliant && seal
	}
	return false
}
