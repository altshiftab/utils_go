package blob_list

import (
	"encoding/xml"
	"testing"
)

// TestDecode reads a listing in the service's own shape, as its documentation gives it.
func TestDecode(t *testing.T) {
	t.Parallel()

	const listing = `<?xml version="1.0" encoding="utf-8"?>
<EnumerationResults ServiceEndpoint="https://intilsignalsbackupdev.blob.core.windows.net/" ContainerName="databases">
  <Prefix>signals-dev/</Prefix>
  <MaxResults>2</MaxResults>
  <Blobs>
    <Blob>
      <Name>signals-dev/signals_dd/2026-11-19.tar.age</Name>
      <VersionId>2026-11-19T00:31:02.1234567Z</VersionId>
      <IsCurrentVersion>true</IsCurrentVersion>
      <Properties>
        <Creation-Time>Thu, 19 Nov 2026 00:31:02 GMT</Creation-Time>
        <Content-Length>1048576</Content-Length>
        <Content-Type>application/octet-stream</Content-Type>
        <ImmutabilityPolicyUntilDate>Fri, 18 Dec 2026 00:31:02 GMT</ImmutabilityPolicyUntilDate>
        <ImmutabilityPolicyMode>unlocked</ImmutabilityPolicyMode>
      </Properties>
      <Metadata>
        <ciphertextsha256>ab12</ciphertextsha256>
      </Metadata>
    </Blob>
  </Blobs>
  <NextMarker>2!72!MDAwMDE</NextMarker>
</EnumerationResults>`

	var results EnumerationResults
	if err := xml.Unmarshal([]byte(listing), &results); err != nil {
		t.Fatalf("xml unmarshal: %v", err)
	}
	if results.NextMarker != "2!72!MDAwMDE" || results.Blobs == nil || len(results.Blobs.Blob) != 1 {
		t.Fatalf("results %+v", results)
	}

	blob := results.Blobs.Blob[0]
	testCases := []struct {
		name string
		got  any
		want any
	}{
		{name: "name", got: blob.Name, want: "signals-dev/signals_dd/2026-11-19.tar.age"},
		{name: "version id", got: blob.VersionId, want: "2026-11-19T00:31:02.1234567Z"},
		{name: "current", got: blob.IsCurrentVersion, want: true},
		{name: "length", got: blob.Properties.ContentLength, want: int64(1048576)},
		{name: "retention", got: blob.Properties.ImmutabilityPolicyUntilDate, want: "Fri, 18 Dec 2026 00:31:02 GMT"},
		{name: "retention mode", got: blob.Properties.ImmutabilityPolicyMode, want: "unlocked"},
		{name: "metadata", got: blob.Metadata.Map()["ciphertextsha256"], want: "ab12"},
	}
	for _, testCase := range testCases {
		if testCase.got != testCase.want {
			t.Errorf("%s: %v, want %v", testCase.name, testCase.got, testCase.want)
		}
	}
}

func TestMetadataMapOfNil(t *testing.T) {
	t.Parallel()

	var metadata *Metadata
	if got := metadata.Map(); got != nil {
		t.Errorf("map of nil metadata %v", got)
	}
}
