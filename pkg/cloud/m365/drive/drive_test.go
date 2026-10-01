package drive

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/create_folder_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/drive_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/move_item_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/conflict_behavior"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/drive_item"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/drive_item/folder"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/upload_config"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type request struct {
	method        string
	uri           string
	contentType   string
	contentRange  string
	authorization string
	body          []byte
}

type fakeGraph struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	requests []*request
	handle   func(w http.ResponseWriter, r *request)
}

// newFakeGraph serves handle and returns a client whose Graph calls carry a
// bearer token through its own http.Client, as a real caller's would.
func newFakeGraph(t *testing.T, handle func(w http.ResponseWriter, r *request), options ...drive_config.Option) (*fakeGraph, *Client) {
	t.Helper()
	fake := &fakeGraph{t: t, handle: handle}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		recorded := &request{
			method:        r.Method,
			uri:           r.RequestURI,
			contentType:   r.Header.Get("Content-Type"),
			contentRange:  r.Header.Get("Content-Range"),
			authorization: r.Header.Get("Authorization"),
			body:          body,
		}
		fake.mu.Lock()
		fake.requests = append(fake.requests, recorded)
		fake.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		handle(w, recorded)
	}))
	t.Cleanup(fake.server.Close)

	baseUrl, err := url.Parse(fake.server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	authClient := &http.Client{Transport: bearerTransport{}}
	options = append(
		[]drive_config.Option{drive_config.WithBaseUrl(baseUrl), drive_config.WithFetchOptions(fetch_config.WithHttpClient(authClient))},
		options...,
	)
	return fake, NewClient(options...)
}

type bearerTransport struct{}

func (bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer graph-token")
	return http.DefaultTransport.RoundTrip(r)
}

func writeJson(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.WriteHeader(status)
	if err := json.MarshalWrite(w, value); err != nil {
		t.Errorf("marshal: %v", err)
	}
}

func folderItem(id string, name string) *drive_item.DriveItem {
	return &drive_item.DriveItem{Id: id, Name: name, Folder: &folder.Folder{}}
}

func TestRequestUris(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		call    func(c *Client) error
		method  string
		wantUri string
	}{
		{
			name:    "get drive",
			call:    func(c *Client) error { _, err := c.GetDrive(context.Background(), "b!abc"); return err },
			method:  http.MethodGet,
			wantUri: "/v1.0/drives/b%21abc",
		},
		{
			name:    "get user drive",
			call:    func(c *Client) error { _, err := c.GetUserDrive(context.Background(), "anna@contoso.se"); return err },
			method:  http.MethodGet,
			wantUri: "/v1.0/users/anna@contoso.se/drive",
		},
		{
			name: "get site drive",
			call: func(c *Client) error {
				_, err := c.GetSiteDrive(context.Background(), "contoso.sharepoint.com,1,2")
				return err
			},
			method:  http.MethodGet,
			wantUri: "/v1.0/sites/contoso.sharepoint.com%2C1%2C2/drive",
		},
		{
			name: "get site by path",
			call: func(c *Client) error {
				_, err := c.GetSiteByPath(context.Background(), "contoso.sharepoint.com", "/sites/Ekonomi")
				return err
			},
			method:  http.MethodGet,
			wantUri: "/v1.0/sites/contoso.sharepoint.com:/sites/Ekonomi",
		},
		{
			name: "get root site",
			call: func(c *Client) error {
				_, err := c.GetSiteByPath(context.Background(), "contoso.sharepoint.com", "")
				return err
			},
			method:  http.MethodGet,
			wantUri: "/v1.0/sites/contoso.sharepoint.com",
		},
		{
			name: "get root site by slash",
			call: func(c *Client) error {
				_, err := c.GetSiteByPath(context.Background(), "contoso.sharepoint.com", "/")
				return err
			},
			method:  http.MethodGet,
			wantUri: "/v1.0/sites/contoso.sharepoint.com",
		},
		{
			name:    "get item",
			call:    func(c *Client) error { _, err := c.GetItem(context.Background(), "d", "01ABC"); return err },
			method:  http.MethodGet,
			wantUri: "/v1.0/drives/d/items/01ABC",
		},
		{
			name: "get item by path",
			call: func(c *Client) error {
				_, err := c.GetItemByPath(context.Background(), "d", "/Rapporter/Q3 #1.xlsx")
				return err
			},
			method:  http.MethodGet,
			wantUri: "/v1.0/drives/d/root:/Rapporter/Q3%20%231.xlsx:",
		},
		{
			name:    "get root by empty path",
			call:    func(c *Client) error { _, err := c.GetItemByPath(context.Background(), "d", "/"); return err },
			method:  http.MethodGet,
			wantUri: "/v1.0/drives/d/root",
		},
		{
			name:    "delete item",
			call:    func(c *Client) error { return c.DeleteItem(context.Background(), "d", "01ABC") },
			method:  http.MethodDelete,
			wantUri: "/v1.0/drives/d/items/01ABC",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
				writeJson(t, w, http.StatusOK, map[string]string{"id": "x"})
			})
			if err := testCase.call(client); err != nil {
				t.Fatalf("call: %v", err)
			}
			if len(fake.requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(fake.requests))
			}
			got := fake.requests[0]
			if got.method != testCase.method || got.uri != testCase.wantUri {
				t.Errorf("request = %s %s, want %s %s", got.method, got.uri, testCase.method, testCase.wantUri)
			}
			if got.authorization != "Bearer graph-token" {
				t.Errorf("Authorization = %q", got.authorization)
			}
		})
	}
}

func TestListChildrenFollowsNextLink(t *testing.T) {
	t.Parallel()

	var fake *fakeGraph
	fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
		switch r.uri {
		case "/v1.0/drives/d/items/root/children":
			writeJson(t, w, http.StatusOK, map[string]any{
				"value":           []*drive_item.DriveItem{folderItem("1", "Kunder")},
				"@odata.nextLink": fake.server.URL + "/v1.0/drives/d/items/root/children?$skiptoken=abc",
			})
		case "/v1.0/drives/d/items/root/children?$skiptoken=abc":
			writeJson(t, w, http.StatusOK, map[string]any{
				"value": []*drive_item.DriveItem{{Id: "2", Name: "avtal.pdf"}},
			})
		default:
			t.Errorf("unexpected request %s", r.uri)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	items, err := client.ListChildren(context.Background(), "d", RootItemId)
	if err != nil {
		t.Fatalf("list children: %v", err)
	}
	if len(items) != 2 || items[0].Name != "Kunder" || items[0].Folder == nil || items[1].Name != "avtal.pdf" || items[1].Folder != nil {
		t.Errorf("items = %+v", items)
	}
}

func TestCreateFolder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		options  []create_folder_config.Option
		wantBody string
	}{
		{
			name:     "fails on conflict by default",
			wantBody: `{"name":"Kunder","folder":{},"@microsoft.graph.conflictBehavior":"fail"}`,
		},
		{
			name:     "rename",
			options:  []create_folder_config.Option{create_folder_config.WithConflictBehavior(conflict_behavior.Rename)},
			wantBody: `{"name":"Kunder","folder":{},"@microsoft.graph.conflictBehavior":"rename"}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
				writeJson(t, w, http.StatusCreated, folderItem("f1", "Kunder"))
			})
			item, err := client.CreateFolder(context.Background(), "d", "p1", "Kunder", testCase.options...)
			if err != nil {
				t.Fatalf("create folder: %v", err)
			}
			if item.Id != "f1" {
				t.Errorf("Id = %q", item.Id)
			}
			got := fake.requests[0]
			if got.method != http.MethodPost || got.uri != "/v1.0/drives/d/items/p1/children" {
				t.Errorf("request = %s %s", got.method, got.uri)
			}
			if string(got.body) != testCase.wantBody {
				t.Errorf("body = %s, want %s", got.body, testCase.wantBody)
			}
		})
	}
}

func TestEnsureFolderPath(t *testing.T) {
	t.Parallel()

	const (
		pathA   = "/v1.0/drives/d/root:/A:"
		pathAB  = "/v1.0/drives/d/root:/A/B:"
		pathABC = "/v1.0/drives/d/root:/A/B/C:"
	)

	testCases := []struct {
		name string
		// existing maps the paths that resolve to their items; creating a
		// folder adds it, unless raced, when the create conflicts but the
		// folder appears.
		existing map[string]*drive_item.DriveItem
		raced    bool
		wantId   string
		wantErr  bool
		wantUris []string
	}{
		{
			name:     "already exists",
			existing: map[string]*drive_item.DriveItem{pathABC: folderItem("c", "C")},
			wantId:   "c",
			wantUris: []string{"GET " + pathABC},
		},
		{
			name:     "creates missing tail",
			existing: map[string]*drive_item.DriveItem{pathA: folderItem("a", "A")},
			wantId:   "C-new",
			wantUris: []string{
				"GET " + pathABC,
				"GET " + pathA,
				"GET " + pathAB,
				"POST /v1.0/drives/d/items/a/children",
				"GET " + pathABC,
				"POST /v1.0/drives/d/items/B-new/children",
			},
		},
		{
			name:     "concurrent creation is used as found",
			existing: map[string]*drive_item.DriveItem{pathA: folderItem("a", "A"), pathAB: folderItem("b", "B")},
			raced:    true,
			wantId:   "C-raced",
			wantUris: []string{
				"GET " + pathABC,
				"GET " + pathA,
				"GET " + pathAB,
				"GET " + pathABC,
				"POST /v1.0/drives/d/items/b/children",
				"GET " + pathABC,
			},
		},
		{
			name:     "path is a file",
			existing: map[string]*drive_item.DriveItem{pathABC: {Id: "c", Name: "C"}},
			wantErr:  true,
		},
		{
			name:     "file in the way",
			existing: map[string]*drive_item.DriveItem{pathA: {Id: "a", Name: "A"}},
			wantErr:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			existing := testCase.existing
			folderPaths := map[string]string{"root": "", "a": "A", "b": "A/B"}
			fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
				if r.method == http.MethodGet {
					if item, ok := existing[r.uri]; ok {
						writeJson(t, w, http.StatusOK, item)
						return
					}
					writeJson(t, w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "itemNotFound"}})
					return
				}

				var body createFolderRequest
				if err := json.Unmarshal(r.body, &body); err != nil {
					t.Errorf("unmarshal: %v", err)
				}
				parentId := strings.TrimSuffix(strings.TrimPrefix(r.uri, "/v1.0/drives/d/items/"), "/children")
				createdPath := strings.TrimPrefix(folderPaths[parentId]+"/"+body.Name, "/")
				createdUri := "/v1.0/drives/d/root:/" + createdPath + ":"
				if testCase.raced {
					existing[createdUri] = folderItem(body.Name+"-raced", body.Name)
					writeJson(t, w, http.StatusConflict, map[string]any{"error": map[string]string{"code": "nameAlreadyExists"}})
					return
				}
				item := folderItem(body.Name+"-new", body.Name)
				existing[createdUri] = item
				folderPaths[item.Id] = createdPath
				writeJson(t, w, http.StatusCreated, item)
			})

			item, err := client.EnsureFolderPath(context.Background(), "d", "A/B/C")
			if testCase.wantErr {
				if !errors.Is(err, ErrNotFolder) {
					t.Fatalf("err = %v, want ErrNotFolder", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ensure folder path: %v", err)
			}
			if item.Id != testCase.wantId {
				t.Errorf("Id = %q, want %q", item.Id, testCase.wantId)
			}
			var gotUris []string
			for _, r := range fake.requests {
				gotUris = append(gotUris, r.method+" "+r.uri)
			}
			if strings.Join(gotUris, "\n") != strings.Join(testCase.wantUris, "\n") {
				t.Errorf("requests =\n%s\nwant\n%s", strings.Join(gotUris, "\n"), strings.Join(testCase.wantUris, "\n"))
			}
		})
	}
}

func TestUploadFileSimple(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		options         []upload_config.Option
		wantUri         string
		wantContentType string
	}{
		{
			name:            "defaults",
			wantUri:         "/v1.0/drives/d/items/p1:/r%C3%A4kning%201.pptx:/content?%40microsoft.graph.conflictBehavior=fail",
			wantContentType: "application/octet-stream",
		},
		{
			name: "replace with content type",
			options: []upload_config.Option{
				upload_config.WithConflictBehavior(conflict_behavior.Replace),
				upload_config.WithContentType("application/vnd.openxmlformats-officedocument.presentationml.presentation"),
			},
			wantUri:         "/v1.0/drives/d/items/p1:/r%C3%A4kning%201.pptx:/content?%40microsoft.graph.conflictBehavior=replace",
			wantContentType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
				writeJson(t, w, http.StatusCreated, &drive_item.DriveItem{Id: "new", Name: "räkning 1.pptx", Size: int64(len(r.body))})
			})
			data := []byte("PK\x03\x04 deck")
			item, err := client.UploadFile(context.Background(), "d", "p1", "räkning 1.pptx", data, testCase.options...)
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			if item.Id != "new" || item.Size != int64(len(data)) {
				t.Errorf("item = %+v", item)
			}
			got := fake.requests[0]
			if got.method != http.MethodPut || got.uri != testCase.wantUri {
				t.Errorf("request = %s %s, want PUT %s", got.method, got.uri, testCase.wantUri)
			}
			if got.contentType != testCase.wantContentType {
				t.Errorf("Content-Type = %q, want %q", got.contentType, testCase.wantContentType)
			}
			if !bytes.Equal(got.body, data) {
				t.Errorf("body = %q, want %q", got.body, data)
			}
		})
	}
}

func TestUploadFileSession(t *testing.T) {
	t.Parallel()

	const chunkSize = uploadChunkUnit
	data := bytes.Repeat([]byte("0123456789"), (SimpleUploadLimit+chunkSize)/10+1)

	var fake *fakeGraph
	var received []byte
	fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
		switch {
		case r.uri == "/v1.0/drives/d/items/p1:/big.bin:/createUploadSession":
			if want := `{"item":{"@microsoft.graph.conflictBehavior":"replace"}}`; string(r.body) != want {
				t.Errorf("session body = %s, want %s", r.body, want)
			}
			writeJson(t, w, http.StatusOK, map[string]string{"uploadUrl": fake.server.URL + "/upload/session-1"})
		case r.uri == "/upload/session-1" && r.method == http.MethodPut:
			received = append(received, r.body...)
			if len(received) < len(data) {
				writeJson(t, w, http.StatusAccepted, map[string]any{"nextExpectedRanges": []string{"x-"}})
				return
			}
			writeJson(t, w, http.StatusCreated, &drive_item.DriveItem{Id: "big", Size: int64(len(received))})
		default:
			t.Errorf("unexpected request %s %s", r.method, r.uri)
			w.WriteHeader(http.StatusBadRequest)
		}
	}, drive_config.WithUploadChunkSize(chunkSize))

	item, err := client.UploadFile(
		context.Background(), "d", "p1", "big.bin", data, upload_config.WithConflictBehavior(conflict_behavior.Replace),
	)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if item.Id != "big" {
		t.Errorf("Id = %q", item.Id)
	}
	if !bytes.Equal(received, data) {
		t.Errorf("received %d bytes, want the %d uploaded", len(received), len(data))
	}

	chunks := fake.requests[1:]
	wantChunks := (len(data) + chunkSize - 1) / chunkSize
	if len(chunks) != wantChunks {
		t.Fatalf("chunks = %d, want %d", len(chunks), wantChunks)
	}
	if fake.requests[0].authorization != "Bearer graph-token" {
		t.Errorf("session creation not authenticated")
	}
	for i, chunk := range chunks {
		if chunk.authorization != "" {
			t.Errorf("chunk %d sent Authorization to the pre-authenticated upload URL", i)
		}
	}
	if got, want := chunks[0].contentRange, "bytes 0-327679/"+strconv.Itoa(len(data)); got != want {
		t.Errorf("first Content-Range = %q, want %q", got, want)
	}
	lastStart := (wantChunks - 1) * chunkSize
	if got, want := chunks[len(chunks)-1].contentRange, "bytes "+strconv.Itoa(lastStart)+"-"+strconv.Itoa(len(data)-1)+"/"+strconv.Itoa(len(data)); got != want {
		t.Errorf("last Content-Range = %q, want %q", got, want)
	}
}

func TestUploadFileSessionFailureCancels(t *testing.T) {
	t.Parallel()

	var fake *fakeGraph
	fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
		switch {
		case strings.HasSuffix(r.uri, "/createUploadSession"):
			writeJson(t, w, http.StatusOK, map[string]string{"uploadUrl": fake.server.URL + "/upload/session-1"})
		case r.method == http.MethodPut:
			writeJson(t, w, http.StatusInternalServerError, map[string]any{"error": map[string]string{"code": "generalException"}})
		case r.method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	data := make([]byte, SimpleUploadLimit+1)
	if _, err := client.UploadFile(context.Background(), "d", "p1", "big.bin", data); err == nil {
		t.Fatal("expected error")
	}
	last := fake.requests[len(fake.requests)-1]
	if last.method != http.MethodDelete || last.uri != "/upload/session-1" {
		t.Errorf("last request = %s %s, want DELETE of the session", last.method, last.uri)
	}
	if last.authorization != "" {
		t.Errorf("cancel sent Authorization to the pre-authenticated upload URL")
	}
}

func TestMoveItem(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		options  []move_item_config.Option
		wantUri  string
		wantBody string
	}{
		{
			name:     "move",
			wantUri:  "/v1.0/drives/d/items/i1",
			wantBody: `{"parentReference":{"id":"p2"}}`,
		},
		{
			name: "move and rename, replacing",
			options: []move_item_config.Option{
				move_item_config.WithName("2026-10 rapport.xlsx"),
				move_item_config.WithConflictBehavior(conflict_behavior.Replace),
			},
			wantUri:  "/v1.0/drives/d/items/i1?%40microsoft.graph.conflictBehavior=replace",
			wantBody: `{"parentReference":{"id":"p2"},"name":"2026-10 rapport.xlsx"}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
				writeJson(t, w, http.StatusOK, &drive_item.DriveItem{Id: "i1"})
			})
			if _, err := client.MoveItem(context.Background(), "d", "i1", "p2", testCase.options...); err != nil {
				t.Fatalf("move: %v", err)
			}
			got := fake.requests[0]
			if got.method != http.MethodPatch || got.uri != testCase.wantUri {
				t.Errorf("request = %s %s, want PATCH %s", got.method, got.uri, testCase.wantUri)
			}
			if string(got.body) != testCase.wantBody {
				t.Errorf("body = %s, want %s", got.body, testCase.wantBody)
			}
		})
	}
}

func TestValidation(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx := context.Background()

	testCases := []struct {
		name string
		call func() error
	}{
		{name: "get drive", call: func() error { _, err := client.GetDrive(ctx, ""); return err }},
		{name: "get user drive", call: func() error { _, err := client.GetUserDrive(ctx, ""); return err }},
		{name: "get site drive", call: func() error { _, err := client.GetSiteDrive(ctx, ""); return err }},
		{name: "list site drives", call: func() error { _, err := client.ListSiteDrives(ctx, ""); return err }},
		{name: "get site by path", call: func() error { _, err := client.GetSiteByPath(ctx, "", "x"); return err }},
		{name: "get item drive", call: func() error { _, err := client.GetItem(ctx, "", "i"); return err }},
		{name: "get item id", call: func() error { _, err := client.GetItem(ctx, "d", ""); return err }},
		{name: "get item by path", call: func() error { _, err := client.GetItemByPath(ctx, "", "x"); return err }},
		{name: "list children folder", call: func() error { _, err := client.ListChildren(ctx, "d", ""); return err }},
		{name: "create folder name", call: func() error { _, err := client.CreateFolder(ctx, "d", "p", ""); return err }},
		{name: "ensure folder path", call: func() error { _, err := client.EnsureFolderPath(ctx, "", "a"); return err }},
		{name: "upload name", call: func() error { _, err := client.UploadFile(ctx, "d", "p", "", nil); return err }},
		{name: "upload parent", call: func() error { _, err := client.UploadFile(ctx, "d", "", "n", nil); return err }},
		{name: "move parent", call: func() error { _, err := client.MoveItem(ctx, "d", "i", ""); return err }},
		{name: "delete item", call: func() error { return client.DeleteItem(ctx, "d", "") }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if err := testCase.call(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestUploadFileSessionChunkSizeAndClient(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		chunkSize     int
		wantChunkSize int
	}{
		{name: "rounded down to whole units", chunkSize: 1 << 20, wantChunkSize: 3 * uploadChunkUnit},
		{name: "at least one unit", chunkSize: 1000, wantChunkSize: uploadChunkUnit},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data := make([]byte, SimpleUploadLimit+1)
			uploadClientUsed := 0
			uploadClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				uploadClientUsed++
				return http.DefaultTransport.RoundTrip(r)
			})}

			var fake *fakeGraph
			received := 0
			fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
				if strings.HasSuffix(r.uri, "/createUploadSession") {
					writeJson(t, w, http.StatusOK, map[string]string{"uploadUrl": fake.server.URL + "/upload/s"})
					return
				}
				if received+len(r.body) < len(data) && len(r.body) != testCase.wantChunkSize {
					t.Errorf("chunk of %d bytes, want %d", len(r.body), testCase.wantChunkSize)
				}
				received += len(r.body)
				if received < len(data) {
					writeJson(t, w, http.StatusAccepted, map[string]any{})
					return
				}
				writeJson(t, w, http.StatusCreated, &drive_item.DriveItem{Id: "big"})
			}, drive_config.WithUploadChunkSize(testCase.chunkSize), drive_config.WithUploadHttpClient(uploadClient))

			if _, err := client.UploadFile(context.Background(), "d", "p1", "big.bin", data); err != nil {
				t.Fatalf("upload: %v", err)
			}
			wantChunks := (len(data) + testCase.wantChunkSize - 1) / testCase.wantChunkSize
			if uploadClientUsed != wantChunks {
				t.Errorf("upload client sent %d requests, want %d", uploadClientUsed, wantChunks)
			}
		})
	}
}

func TestUploadFileSessionEarlyCompletion(t *testing.T) {
	t.Parallel()

	var fake *fakeGraph
	fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
		switch {
		case strings.HasSuffix(r.uri, "/createUploadSession"):
			writeJson(t, w, http.StatusOK, map[string]string{"uploadUrl": fake.server.URL + "/upload/s"})
		case r.method == http.MethodPut:
			writeJson(t, w, http.StatusOK, &drive_item.DriveItem{Id: "truncated"})
		case r.method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	_, err := client.UploadFile(context.Background(), "d", "p1", "big.bin", make([]byte, SimpleUploadLimit+1))
	if !errors.Is(err, ErrUnexpectedUploadStatus) {
		t.Fatalf("err = %v, want ErrUnexpectedUploadStatus", err)
	}
	if last := fake.requests[len(fake.requests)-1]; last.method != http.MethodDelete {
		t.Errorf("last request = %s, want the session cancelled", last.method)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
