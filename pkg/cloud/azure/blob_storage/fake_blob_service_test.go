package blob_storage

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/azure/blob_storage/blob_storage_config"
	"github.com/altshiftab/utils_go/pkg/cloud/azure/blob_storage/types/blob_list"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// fakeVersion is one version of a blob as the fake service keeps it.
type fakeVersion struct {
	id          string
	content     []byte
	contentType string
	metadata    map[string]string
}

// fakeBlob is a blob's versions, oldest first, and whether the newest is current.
type fakeBlob struct {
	versions []*fakeVersion
	// current is false once the blob is deleted and only earlier versions remain.
	current bool
}

// fakeBlobService models the parts of the Blob service the client uses, side effects included:
// blocks are staged apart from the blob and become its content only on commit, in the order the
// block list gives; a commit to a name a current version holds makes a new version and keeps the
// old one; a conditional commit to such a name is refused with the service's code; listings page
// through markers.
type fakeBlobService struct {
	t *testing.T

	mu       sync.Mutex
	blobs    map[string]*fakeBlob
	staged   map[string]map[string][]byte
	versions int
	requests int
	// stagedIds is every block id staged, by blob, in order.
	stagedIds map[string][]string
}

func newFakeBlobService(t *testing.T, fetchOptions ...fetch_config.Option) (*fakeBlobService, *Client) {
	t.Helper()

	service := &fakeBlobService{t: t, blobs: map[string]*fakeBlob{}, staged: map[string]map[string][]byte{}}
	server := httptest.NewServer(service)
	t.Cleanup(server.Close)

	baseUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}
	client, err := NewClient(
		"account",
		blob_storage_config.WithBaseUrl(baseUrl),
		blob_storage_config.WithFetchOptions(append([]fetch_config.Option{fetch_config.WithHttpClient(server.Client())}, fetchOptions...)...),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	return service, client
}

func (s *fakeBlobService) refuse(w http.ResponseWriter, status int, code string) {
	w.Header().Set(errorCodeHeaderName, code)
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"utf-8\"?><Error><Code>%s</Code></Error>", code)
}

func (s *fakeBlobService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++

	if r.Header.Get(versionHeaderName) != ApiVersion {
		s.refuse(w, http.StatusBadRequest, "MissingRequiredHeader")
		return
	}

	// The path, decoded: /<container>/<blob name, slashes and all>.
	container, blob, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	query := r.URL.Query()
	key := container + "/" + blob

	switch {
	case r.Method == http.MethodPut && query.Get("comp") == "block":
		s.putBlock(w, r, key, query.Get("blockid"))
	case r.Method == http.MethodPut && query.Get("comp") == "blocklist":
		s.putBlockList(w, r, key)
	case r.Method == http.MethodGet && query.Get("comp") == "list" && query.Get("restype") == "container":
		s.listBlobs(w, container, query)
	case r.Method == http.MethodGet && blob != "":
		s.getBlob(w, key, query.Get("versionid"))
	default:
		s.refuse(w, http.StatusBadRequest, "UnsupportedHttpVerb")
	}
}

func (s *fakeBlobService) putBlock(w http.ResponseWriter, r *http.Request, key string, id string) {
	if _, err := base64.StdEncoding.DecodeString(id); err != nil || id == "" {
		s.refuse(w, http.StatusBadRequest, "InvalidQueryParameterValue")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Errorf("io read all: %v", err)
		return
	}

	staged := s.staged[key]
	if staged == nil {
		staged = map[string][]byte{}
		s.staged[key] = staged
	}
	// The service refuses a block id of another length than the blob's others.
	for other := range staged {
		if len(other) != len(id) {
			s.refuse(w, http.StatusBadRequest, "InvalidBlobOrBlock")
			return
		}
	}
	staged[id] = body
	if s.stagedIds == nil {
		s.stagedIds = map[string][]string{}
	}
	s.stagedIds[key] = append(s.stagedIds[key], id)
	w.WriteHeader(http.StatusCreated)
}

func (s *fakeBlobService) putBlockList(w http.ResponseWriter, r *http.Request, key string) {
	var list blockList
	if err := xml.NewDecoder(r.Body).Decode(&list); err != nil {
		s.refuse(w, http.StatusBadRequest, "InvalidXmlDocument")
		return
	}

	existing := s.blobs[key]
	if r.Header.Get("If-None-Match") == "*" && existing != nil && existing.current {
		s.refuse(w, http.StatusConflict, errorCodeBlobAlreadyExists)
		return
	}

	var content []byte
	for _, id := range list.Latest {
		block, ok := s.staged[key][id]
		if !ok {
			s.refuse(w, http.StatusBadRequest, "InvalidBlockList")
			return
		}
		content = append(content, block...)
	}

	metadata := map[string]string{}
	for name, values := range r.Header {
		if rest, found := strings.CutPrefix(strings.ToLower(name), metadataHeaderPrefix); found && len(values) > 0 {
			metadata[rest] = values[0]
		}
	}

	s.versions++
	version := &fakeVersion{
		id:          fmt.Sprintf("2026-11-19T00:00:00.%07dZ", s.versions),
		content:     content,
		contentType: r.Header.Get(blobContentTypeHeader),
		metadata:    metadata,
	}
	if existing == nil {
		existing = &fakeBlob{}
		s.blobs[key] = existing
	}
	existing.versions = append(existing.versions, version)
	existing.current = true
	// Committing discards every block the list left out.
	delete(s.staged, key)

	w.Header().Set("ETag", `"`+version.id+`"`)
	w.Header().Set(versionIdHeaderName, version.id)
	w.WriteHeader(http.StatusCreated)
}

func (s *fakeBlobService) getBlob(w http.ResponseWriter, key string, versionId string) {
	blob := s.blobs[key]
	var version *fakeVersion
	switch {
	case blob == nil:
	case versionId != "":
		for _, candidate := range blob.versions {
			if candidate.id == versionId {
				version = candidate
			}
		}
	case blob.current:
		version = blob.versions[len(blob.versions)-1]
	}
	if version == nil {
		s.refuse(w, http.StatusNotFound, "BlobNotFound")
		return
	}

	for name, value := range version.metadata {
		w.Header().Set(metadataHeaderPrefix+name, value)
	}
	if version.contentType != "" {
		w.Header().Set("Content-Type", version.contentType)
	}
	w.Header().Set(versionIdHeaderName, version.id)
	w.Header().Set("Content-Length", strconv.Itoa(len(version.content)))
	_, _ = w.Write(version.content)
}

func (s *fakeBlobService) listBlobs(w http.ResponseWriter, container string, query url.Values) {
	include := strings.Split(query.Get("include"), ",")
	withVersions := slices.Contains(include, "versions")
	withMetadata := slices.Contains(include, "metadata")

	all := []*blob_list.Blob{}
	names := make([]string, 0, len(s.blobs))
	for key := range s.blobs {
		names = append(names, key)
	}
	slices.Sort(names)
	for _, key := range names {
		name, found := strings.CutPrefix(key, container+"/")
		if !found || !strings.HasPrefix(name, query.Get("prefix")) {
			continue
		}
		blob := s.blobs[key]
		if blob == nil {
			continue
		}
		for i, version := range blob.versions {
			isCurrent := blob.current && i == len(blob.versions)-1
			if !withVersions && !isCurrent {
				continue
			}
			listed := &blob_list.Blob{
				Name:       name,
				Properties: &blob_list.Properties{ContentLength: int64(len(version.content)), ContentType: version.contentType},
			}
			if withVersions {
				listed.VersionId = version.id
				listed.IsCurrentVersion = isCurrent
			}
			if withMetadata {
				listed.Metadata = &blob_list.Metadata{}
				for metadataName, value := range version.metadata {
					listed.Metadata.Items = append(listed.Metadata.Items, &blob_list.MetadataItem{XMLName: xml.Name{Local: metadataName}, Value: value})
				}
			}
			all = append(all, listed)
		}
	}

	start := 0
	if marker := query.Get("marker"); marker != "" {
		var err error
		if start, err = strconv.Atoi(marker); err != nil {
			s.refuse(w, http.StatusBadRequest, "InvalidQueryParameterValue")
			return
		}
	}
	end := len(all)
	if maxResults, err := strconv.Atoi(query.Get("maxresults")); err == nil && start+maxResults < end {
		end = start + maxResults
	}

	results := &blob_list.EnumerationResults{Prefix: query.Get("prefix"), Blobs: &blob_list.Blobs{Blob: all[start:end]}}
	if end < len(all) {
		results.NextMarker = strconv.Itoa(end)
	}

	w.Header().Set("Content-Type", "application/xml")
	_, _ = io.WriteString(w, xml.Header)
	if err := xml.NewEncoder(w).Encode(results); err != nil {
		s.t.Errorf("xml encode: %v", err)
	}
}
