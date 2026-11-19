// Package blob_list is the List Blobs response, as the Blob service writes it in XML.
package blob_list

import (
	"encoding/xml"
)

// EnumerationResults is one page of a listing.
type EnumerationResults struct {
	XMLName    xml.Name `xml:"EnumerationResults"`
	Prefix     string   `xml:"Prefix"`
	Marker     string   `xml:"Marker"`
	MaxResults int      `xml:"MaxResults"`
	Blobs      *Blobs   `xml:"Blobs"`
	// NextMarker continues the listing; empty on its last page.
	NextMarker string `xml:"NextMarker"`
}

type Blobs struct {
	Blob []*Blob `xml:"Blob"`
}

type Blob struct {
	Name string `xml:"Name"`
	// VersionId and IsCurrentVersion are present where versioning is on.
	VersionId        string      `xml:"VersionId"`
	IsCurrentVersion bool        `xml:"IsCurrentVersion"`
	Properties       *Properties `xml:"Properties"`
	Metadata         *Metadata   `xml:"Metadata"`
}

type Properties struct {
	CreationTime  string `xml:"Creation-Time"`
	LastModified  string `xml:"Last-Modified"`
	Etag          string `xml:"Etag"`
	ContentLength int64  `xml:"Content-Length"`
	ContentType   string `xml:"Content-Type"`
	// ImmutabilityPolicyUntilDate and ImmutabilityPolicyMode describe a version's time-based
	// retention, where it has one: until when, and whether it is Locked or Unlocked.
	ImmutabilityPolicyUntilDate string `xml:"ImmutabilityPolicyUntilDate"`
	ImmutabilityPolicyMode      string `xml:"ImmutabilityPolicyMode"`
}

// Metadata is a blob's user metadata: one element per name.
type Metadata struct {
	Items []*MetadataItem `xml:",any"`
}

type MetadataItem struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

// Map returns the metadata by name.
func (m *Metadata) Map() map[string]string {
	if m == nil {
		return nil
	}

	values := make(map[string]string, len(m.Items))
	for _, item := range m.Items {
		if item != nil {
			values[item.XMLName.Local] = item.Value
		}
	}

	return values
}
