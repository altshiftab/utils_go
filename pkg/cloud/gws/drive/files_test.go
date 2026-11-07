package drive

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/drive_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/get_file_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/list_comments_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/list_files_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/file"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/update_file_config"
)

// TestFileRequests checks that each call asks Drive what it should -- method, path, query and body -- and reads back
// what Drive answers.
func TestFileRequests(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		allDrives   bool
		call        func(client *Client) (any, error)
		method      string
		path        string
		query       url.Values
		body        string
		contentType string
		response    string
		// want is the result, as JSON.
		want string
	}{
		{
			name:      "list comments page",
			allDrives: true,
			call: func(client *Client) (any, error) {
				return client.ListCommentsPage(
					context.Background(), "d1",
					list_comments_config.WithPageSize(100),
					list_comments_config.WithPageToken("p1"),
					list_comments_config.WithIncludeDeleted(true),
				)
			},
			method: http.MethodGet,
			path:   "/drive/v3/files/d1/comments",
			// Without supportsAllDrives, which a listing of comments does not take.
			query: url.Values{
				"fields":         {commentFields},
				"pageSize":       {"100"},
				"pageToken":      {"p1"},
				"includeDeleted": {"true"},
			},
			response: `{"comments":[{"id":"c1","createdTime":"2026-11-07T10:00:00.000Z","author":{"displayName":"Anna","me":false},` +
				`"content":"Stämmer siffran?","quotedFileContent":{"mimeType":"text/html","value":"1 200 kr"},"anchor":"kix.1","resolved":true,` +
				`"htmlContent":"Stämmer siffran?","replies":[{"id":"r1","author":{"displayName":"Viktor","me":true},"content":"Ja.","action":"resolve"}]}],` +
				`"nextPageToken":"p2"}`,
			want: `{"comments":[{"id":"c1","createdTime":"2026-11-07T10:00:00.000Z","author":{"displayName":"Anna"},"content":"Stämmer siffran?",` +
				`"quotedFileContent":{"mimeType":"text/html","value":"1 200 kr"},"anchor":"kix.1","resolved":true,` +
				`"replies":[{"id":"r1","author":{"displayName":"Viktor","me":true},"content":"Ja.","action":"resolve"}]}],"nextPageToken":"p2"}`,
		},
		{
			name: "list comments page, its fields named",
			call: func(client *Client) (any, error) {
				return client.ListCommentsPage(context.Background(), "d#1", list_comments_config.WithFields("comments(id)"))
			},
			method:   http.MethodGet,
			path:     "/drive/v3/files/d%231/comments",
			query:    url.Values{"fields": {"comments(id)"}},
			response: `{"comments":[{"id":"c1"}]}`,
			want:     `{"comments":[{"id":"c1"}]}`,
		},
		{
			name:      "list files page",
			allDrives: true,
			call: func(client *Client) (any, error) {
				return client.ListFilesPage(
					context.Background(),
					list_files_config.WithQuery("name contains 'faktura'"),
					list_files_config.WithPageSize(20),
					list_files_config.WithPageToken("p1"),
					list_files_config.WithOrderBy("modifiedTime desc"),
					list_files_config.WithFields("nextPageToken, files(id, name, size)"),
					list_files_config.WithCorpora(list_files_config.CorporaAllDrives),
					list_files_config.WithIncludeItemsFromAllDrives(true),
				)
			},
			method: http.MethodGet,
			path:   "/drive/v3/files",
			query: url.Values{
				"q":                         {"name contains 'faktura'"},
				"pageSize":                  {"20"},
				"pageToken":                 {"p1"},
				"orderBy":                   {"modifiedTime desc"},
				"fields":                    {"nextPageToken, files(id, name, size)"},
				"corpora":                   {"allDrives"},
				"includeItemsFromAllDrives": {"true"},
				"supportsAllDrives":         {"true"},
			},
			response: `{"files":[{"id":"f1","name":"faktura.pdf","size":"2048"}],"nextPageToken":"p2","incompleteSearch":true}`,
			want:     `{"files":[{"id":"f1","name":"faktura.pdf","size":"2048"}],"nextPageToken":"p2","incompleteSearch":true}`,
		},
		{
			name: "get file",
			call: func(client *Client) (any, error) {
				return client.GetFile(context.Background(), "f1", get_file_config.WithFields("id, name, version"))
			},
			method:   http.MethodGet,
			path:     "/drive/v3/files/f1",
			query:    url.Values{"fields": {"id, name, version"}},
			response: `{"id":"f1","name":"a.txt","version":"7"}`,
			want:     `{"id":"f1","name":"a.txt","version":"7"}`,
		},
		{
			name: "download file",
			call: func(client *Client) (any, error) {
				content, contentType, err := client.DownloadFile(context.Background(), "f1")
				return map[string]string{"content": string(content), "contentType": contentType}, err
			},
			method:      http.MethodGet,
			path:        "/drive/v3/files/f1",
			query:       url.Values{"alt": {"media"}},
			contentType: "application/pdf",
			response:    "%PDF-1.7",
			want:        `{"content":"%PDF-1.7","contentType":"application/pdf"}`,
		},
		{
			name: "export file",
			call: func(client *Client) (any, error) {
				content, err := client.ExportFile(context.Background(), "d1", "text/markdown")
				return string(content), err
			},
			method:      http.MethodGet,
			path:        "/drive/v3/files/d1/export",
			query:       url.Values{"mimeType": {"text/markdown"}},
			contentType: "text/markdown",
			response:    "# Rubrik",
			want:        `"# Rubrik"`,
		},
		{
			name: "create folder",
			call: func(client *Client) (any, error) {
				return client.CreateFile(
					context.Background(),
					&file.File{Name: "Kvitton", MimeType: file.MimeTypeFolder, Parents: []string{"root"}},
					nil,
					"",
				)
			},
			method:   http.MethodPost,
			path:     "/drive/v3/files",
			body:     `{"name":"Kvitton","mimeType":"application/vnd.google-apps.folder","parents":["root"]}`,
			response: `{"id":"folder1","name":"Kvitton"}`,
			want:     `{"id":"folder1","name":"Kvitton"}`,
		},
		{
			name: "update file",
			call: func(client *Client) (any, error) {
				return client.UpdateFile(
					context.Background(),
					"f1",
					&file.File{Name: "b.txt", Trashed: new(false)},
					update_file_config.WithAddParents("p2"),
					update_file_config.WithRemoveParents("p1", "p3"),
					update_file_config.WithFields("id, name, parents"),
				)
			},
			method:   http.MethodPatch,
			path:     "/drive/v3/files/f1",
			query:    url.Values{"addParents": {"p2"}, "removeParents": {"p1,p3"}, "fields": {"id, name, parents"}},
			body:     `{"name":"b.txt","trashed":false}`,
			response: `{"id":"f1","name":"b.txt","parents":["p2"]}`,
			want:     `{"id":"f1","name":"b.txt","parents":["p2"]}`,
		},
		{
			name: "update file content",
			call: func(client *Client) (any, error) {
				return client.UpdateFileContent(context.Background(), "f1", []byte("new,csv\n"), "text/csv")
			},
			method:   http.MethodPatch,
			path:     "/upload/drive/v3/files/f1",
			query:    url.Values{"uploadType": {"media"}},
			body:     "new,csv",
			response: `{"id":"f1","version":"8"}`,
			want:     `{"id":"f1","version":"8"}`,
		},
		{
			name: "delete file",
			call: func(client *Client) (any, error) {
				return "deleted", client.DeleteFile(context.Background(), "tmp")
			},
			method: http.MethodDelete,
			path:   "/drive/v3/files/tmp",
			want:   `"deleted"`,
		},
		{
			name: "copy file",
			call: func(client *Client) (any, error) {
				return client.CopyFile(context.Background(), "f1", &file.File{Name: "kopia"})
			},
			method:   http.MethodPost,
			path:     "/drive/v3/files/f1/copy",
			body:     `{"name":"kopia"}`,
			response: `{"id":"f2","name":"kopia"}`,
			want:     `{"id":"f2","name":"kopia"}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != testCase.method {
					t.Errorf("method = %s, want %s", r.Method, testCase.method)
				}
				if r.URL.EscapedPath() != testCase.path {
					t.Errorf("path = %s, want %s", r.URL.EscapedPath(), testCase.path)
				}
				query := r.URL.Query()
				if len(query) != len(testCase.query) {
					t.Errorf("query = %v, want %v", query, testCase.query)
				}
				for name, want := range testCase.query {
					if got := query[name]; strings.Join(got, ",") != strings.Join(want, ",") {
						t.Errorf("query %s = %v, want %v", name, got, want)
					}
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				if testCase.body != "" && strings.TrimSpace(string(body)) != testCase.body {
					t.Errorf("body = %s, want %s", body, testCase.body)
				}
				contentType := testCase.contentType
				if contentType == "" {
					contentType = "application/json"
				}
				w.Header().Set("Content-Type", contentType)
				if _, err := io.WriteString(w, testCase.response); err != nil {
					t.Errorf("write: %v", err)
				}
			}, drive_config.WithSupportsAllDrives(testCase.allDrives))

			result, err := testCase.call(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("marshal result: %v", err)
			}
			if string(got) != testCase.want {
				t.Errorf("result = %s, want %s", got, testCase.want)
			}
		})
	}
}

// TestCreateFileWithContent checks that metadata and content go up together, as the two parts of one multipart/related
// body, each with its own type.
func TestCreateFileWithContent(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/upload/drive/v3/files" {
			t.Errorf("path = %s, want /upload/drive/v3/files", r.URL.Path)
		}
		if got := r.URL.Query().Get("uploadType"); got != "multipart" {
			t.Errorf("uploadType = %q, want multipart", got)
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse media type: %v", err)
		}
		if mediaType != "multipart/related" {
			t.Errorf("media type = %q, want multipart/related", mediaType)
		}

		reader := multipart.NewReader(r.Body, params["boundary"])
		wantParts := []struct{ contentType, body string }{
			{contentType: "application/json; charset=UTF-8", body: `{"name":"a.pdf","parents":["p1"]}`},
			{contentType: "application/pdf", body: "%PDF-1.7"},
		}
		for index, want := range wantParts {
			part, err := reader.NextPart()
			if err != nil {
				t.Fatalf("part %d: %v", index, err)
			}
			if got := part.Header.Get("Content-Type"); got != want.contentType {
				t.Errorf("part %d content type = %q, want %q", index, got, want.contentType)
			}
			var body bytes.Buffer
			if _, err := io.Copy(&body, part); err != nil {
				t.Fatalf("part %d read: %v", index, err)
			}
			if body.String() != want.body {
				t.Errorf("part %d body = %q, want %q", index, body.String(), want.body)
			}
		}
		if _, err := reader.NextPart(); err != io.EOF {
			t.Errorf("expected two parts, got more (%v)", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"id":"f9","name":"a.pdf"}`); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	created, err := client.CreateFile(
		context.Background(),
		&file.File{Name: "a.pdf", Parents: []string{"p1"}},
		[]byte("%PDF-1.7"),
		"application/pdf",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.Id != "f9" {
		t.Errorf("id = %q, want f9", created.Id)
	}
}

// TestFileArgumentErrors checks that a call missing what it needs is refused before anything is asked of Drive.
func TestFileArgumentErrors(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	ctx := context.Background()
	tooLarge := make([]byte, MaxSimpleUploadBytes+1)

	testCases := []struct {
		name string
		call func() error
	}{
		{name: "get, empty id", call: func() error { _, err := client.GetFile(ctx, ""); return err }},
		{name: "list comments, empty id", call: func() error { _, err := client.ListCommentsPage(ctx, ""); return err }},
		{name: "download, empty id", call: func() error { _, _, err := client.DownloadFile(ctx, ""); return err }},
		{name: "export, empty id", call: func() error { _, err := client.ExportFile(ctx, "", "text/plain"); return err }},
		{name: "export, empty type", call: func() error { _, err := client.ExportFile(ctx, "d1", ""); return err }},
		{name: "create, nil file", call: func() error { _, err := client.CreateFile(ctx, nil, nil, ""); return err }},
		{name: "create, no name", call: func() error { _, err := client.CreateFile(ctx, &file.File{}, nil, ""); return err }},
		{
			name: "create, content with no type",
			call: func() error { _, err := client.CreateFile(ctx, &file.File{Name: "a"}, []byte("x"), ""); return err },
		},
		{
			name: "create, content too large",
			call: func() error {
				_, err := client.CreateFile(ctx, &file.File{Name: "a"}, tooLarge, "application/octet-stream")
				return err
			},
		},
		{name: "update, empty id", call: func() error { _, err := client.UpdateFile(ctx, "", &file.File{}); return err }},
		{name: "update, nil file", call: func() error { _, err := client.UpdateFile(ctx, "f1", nil); return err }},
		{
			name: "update content, nil content",
			call: func() error { _, err := client.UpdateFileContent(ctx, "f1", nil, "text/plain"); return err },
		},
		{
			name: "update content, too large",
			call: func() error { _, err := client.UpdateFileContent(ctx, "f1", tooLarge, "text/plain"); return err },
		},
		{name: "copy, empty id", call: func() error { _, err := client.CopyFile(ctx, "", nil); return err }},
		{name: "delete, empty id", call: func() error { return client.DeleteFile(ctx, "") }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if err := testCase.call(); err == nil {
				t.Errorf("%s: expected an error", testCase.name)
			}
		})
	}
}
