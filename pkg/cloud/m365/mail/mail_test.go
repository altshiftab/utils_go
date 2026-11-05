package mail

import (
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/get_attachment_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/get_message_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/list_messages_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/mail_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/item_body"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/message_update"
)

const userId = "5b7e9a64-1c2d-4e3f-8a9b-0c1d2e3f4a5b"

type request struct {
	method string
	uri    string
	prefer string
	body   []byte
}

type fakeGraph struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []*request
}

// newFakeGraph serves handle and returns a client of it.
func newFakeGraph(t *testing.T, handle func(w http.ResponseWriter, r *request)) (*fakeGraph, *Client) {
	t.Helper()
	fake := &fakeGraph{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		recorded := &request{method: r.Method, uri: r.RequestURI, prefer: r.Header.Get("Prefer"), body: body}
		fake.mu.Lock()
		fake.requests = append(fake.requests, recorded)
		fake.mu.Unlock()
		handle(w, recorded)
	}))
	t.Cleanup(fake.server.Close)

	baseUrl, err := url.Parse(fake.server.URL)
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}
	return fake, NewClient(mail_config.WithBaseUrl(baseUrl))
}

func (fake *fakeGraph) only(t *testing.T) *request {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.requests) != 1 {
		t.Fatalf("%d requests, want 1", len(fake.requests))
	}
	return fake.requests[0]
}

func writeJson(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestListMessagesPage_Urls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options []list_messages_config.Option
		wantUri string
	}{
		{
			name:    "the whole mailbox",
			wantUri: "/v1.0/users/" + userId + "/messages",
		},
		{
			name: "a folder, filtered and ordered",
			options: []list_messages_config.Option{
				list_messages_config.WithFolderId(FolderInbox),
				list_messages_config.WithFilter("receivedDateTime ge 2026-01-01T00:00:00Z and categories/any(c: c eq 'Acme/Support')"),
				list_messages_config.WithOrderBy("receivedDateTime desc"),
				list_messages_config.WithSelect("id", "subject"),
				list_messages_config.WithExpand("attachments($select=id,name)"),
				list_messages_config.WithTop(25),
			},
			wantUri: "/v1.0/users/" + userId + "/mailFolders/inbox/messages" +
				"?$filter=receivedDateTime%20ge%202026-01-01T00%3A00%3A00Z%20and%20categories%2Fany%28c%3A%20c%20eq%20%27Acme%2FSupport%27%29" +
				"&$orderby=receivedDateTime%20desc&$select=id%2Csubject&$expand=attachments%28%24select%3Did%2Cname%29&$top=25",
		},
		{
			name:    "a search, its quotes escaped",
			options: []list_messages_config.Option{list_messages_config.WithSearch(`from:a@example.com subject:"q3 report"`)},
			wantUri: "/v1.0/users/" + userId + "/messages?$search=%22from%3Aa%40example.com%20subject%3A%5C%22q3%20report%5C%22%22",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
				writeJson(w, http.StatusOK, `{"value":[{"id":"m1","subject":"s","categories":["Acme/Support"]}],"@odata.nextLink":"next"}`)
			})
			result, err := client.ListMessagesPage(t.Context(), userId, testCase.options...)
			if err != nil {
				t.Fatalf("%s: %v", testCase.name, err)
			}
			if got := fake.only(t); got.method != http.MethodGet || got.uri != testCase.wantUri {
				t.Errorf("%s: %s %s, want GET %s", testCase.name, got.method, got.uri, testCase.wantUri)
			}
			if len(result.Items) != 1 || result.Items[0].Id != "m1" || result.Items[0].Categories[0] != "Acme/Support" || result.NextLink != "next" {
				t.Errorf("%s: page %+v", testCase.name, result)
			}
		})
	}
}

// Every request asks for immutable ids, whatever header the caller sends, and for a body type
// where one is asked for.
func TestPreferHeader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		call func(client *Client) error
		want string
	}{
		{
			name: "a listing",
			call: func(client *Client) error {
				_, err := client.ListMessagesPage(t.Context(), userId)
				return err
			},
			want: `IdType="ImmutableId"`,
		},
		{
			name: "a listing with text bodies",
			call: func(client *Client) error {
				_, err := client.ListMessagesPage(t.Context(), userId, list_messages_config.WithBodyContentType(item_body.ContentTypeText))
				return err
			},
			want: `IdType="ImmutableId", outlook.body-content-type="text"`,
		},
		{
			name: "a message with HTML, and the caller's own Prefer",
			call: func(client *Client) error {
				_, err := client.GetMessage(
					t.Context(), userId, "m1",
					get_message_config.WithBodyContentType(item_body.ContentTypeHtml),
					get_message_config.WithFetchOptions(fetch_config.WithHeaders(map[string]string{"Prefer": "return=minimal"})),
				)
				return err
			},
			want: `IdType="ImmutableId", outlook.body-content-type="html"`,
		},
		{
			name: "a move",
			call: func(client *Client) error {
				_, err := client.MoveMessage(t.Context(), userId, "m1", FolderArchive)
				return err
			},
			want: `IdType="ImmutableId"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
				writeJson(w, http.StatusOK, `{"value":[],"id":"m1"}`)
			})
			if err := testCase.call(client); err != nil {
				t.Fatalf("%s: %v", testCase.name, err)
			}
			if got := fake.only(t).prefer; got != testCase.want {
				t.Errorf("%s: Prefer %q, want %q", testCase.name, got, testCase.want)
			}
		})
	}
}

func TestListMessagesPage_SearchRefusesAFilter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options []list_messages_config.Option
	}{
		{name: "a filter", options: []list_messages_config.Option{list_messages_config.WithSearch("a"), list_messages_config.WithFilter("isRead eq false")}},
		{name: "an order", options: []list_messages_config.Option{list_messages_config.WithSearch("a"), list_messages_config.WithOrderBy("receivedDateTime desc")}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
				t.Errorf("%s: a request was sent", testCase.name)
			})
			_, err := client.ListMessagesPage(t.Context(), userId, testCase.options...)
			if !errors.Is(err, ErrSearchWithFilter) || !errors.Is(err, altshiftErrors.ErrValidationError) {
				t.Errorf("%s: err = %v", testCase.name, err)
			}
		})
	}
}

func TestListMessagesPage_NextLink(t *testing.T) {
	t.Parallel()

	// {base} is the fake Graph's own scheme and host.
	testCases := []struct {
		name     string
		nextLink string
		wantUri  string
	}{
		{name: "by path", nextLink: "{base}/v1.0/users/" + userId + "/messages?$skip=10", wantUri: "/v1.0/users/" + userId + "/messages?$skip=10"},
		{name: "by key", nextLink: "{base}/v1.0/users('" + userId + "')/mailFolders('Inbox')/messages?%24skip=10", wantUri: "/v1.0/users('" + userId + "')/mailFolders('Inbox')/messages?%24skip=10"},
		{name: "the id in upper case", nextLink: "{base}/v1.0/users/" + strings.ToUpper(userId) + "/messages?$skip=10", wantUri: "/v1.0/users/" + strings.ToUpper(userId) + "/messages?$skip=10"},
		{name: "another host", nextLink: "https://graph.example.com/v1.0/users/" + userId + "/messages"},
		{name: "another scheme", nextLink: "{tls}/v1.0/users/" + userId + "/messages"},
		{name: "another mailbox", nextLink: "{base}/v1.0/users/0b4a3e62-6e3e-4bb4-9a0b-2f6c1d3a9e10/messages"},
		{name: "a mailbox whose id begins as this one's", nextLink: "{base}/v1.0/users/" + userId + "x/messages"},
		{name: "out of the mailbox by dot segments", nextLink: "{base}/v1.0/users/" + userId + "/../0b4a3e62/messages"},
		{name: "out of the mailbox by encoded dot segments", nextLink: "{base}/v1.0/users/" + userId + "/%2E%2E/0b4a3e62/messages"},
		{name: "a backslash", nextLink: "{base}/v1.0/users/" + userId + "/%5C..%5C0b4a3e62/messages"},
		{name: "a percent sign, double-encoded", nextLink: "{base}/v1.0/users/" + userId + "/%252E%252E/0b4a3e62/messages"},
		{name: "another version", nextLink: "{base}/beta/users/" + userId + "/messages"},
		{name: "credentials", nextLink: "{credentials}/v1.0/users/" + userId + "/messages"},
		{name: "a fragment", nextLink: "{base}/v1.0/users/" + userId + "/messages#x"},
		{name: "not a url", nextLink: "::"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
				writeJson(w, http.StatusOK, `{"value":[]}`)
			})
			base := fake.server.URL
			nextLink := strings.NewReplacer(
				"{base}", base,
				"{tls}", strings.Replace(base, "http://", "https://", 1),
				"{credentials}", strings.Replace(base, "http://", "http://u:p@", 1),
			).Replace(testCase.nextLink)

			_, err := client.ListMessagesPage(t.Context(), userId, list_messages_config.WithNextLink(nextLink), list_messages_config.WithTop(5))
			if testCase.wantUri == "" {
				if !errors.Is(err, ErrForeignNextLink) || !errors.Is(err, altshiftErrors.ErrValidationError) {
					t.Errorf("%s: err = %v", testCase.name, err)
				}
				fake.mu.Lock()
				defer fake.mu.Unlock()
				if len(fake.requests) != 0 {
					t.Errorf("%s: a request was sent to %s", testCase.name, fake.requests[0].uri)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: %v", testCase.name, err)
			}
			if got := fake.only(t).uri; got != testCase.wantUri {
				t.Errorf("%s: uri %s, want %s", testCase.name, got, testCase.wantUri)
			}
		})
	}
}

func TestGetMessage(t *testing.T) {
	t.Parallel()

	fake, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
		writeJson(w, http.StatusOK, `{"id":"m/1","subject":"s","body":{"contentType":"text","content":"hi"},`+
			`"attachments":[{"@odata.type":"#microsoft.graph.fileAttachment","id":"a1","name":"x.pdf","size":3,"isInline":false}]}`)
	})
	got, err := client.GetMessage(
		t.Context(), userId, "m/1",
		get_message_config.WithSelect("subject", "body"),
		get_message_config.WithExpand("attachments($select=id,name,size)"),
	)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	wantUri := "/v1.0/users/" + userId + "/messages/m%2F1?$select=subject%2Cbody&$expand=attachments%28%24select%3Did%2Cname%2Csize%29"
	if uri := fake.only(t).uri; uri != wantUri {
		t.Errorf("uri %s, want %s", uri, wantUri)
	}
	if got.Body == nil || got.Body.Content != "hi" || len(got.Attachments) != 1 || got.Attachments[0].Size != 3 {
		t.Errorf("message %+v", got)
	}
}

func TestUpdateMessage(t *testing.T) {
	t.Parallel()

	read := true
	testCases := []struct {
		name     string
		update   *message_update.Update
		wantBody string
	}{
		{name: "categories", update: &message_update.Update{Categories: []string{"Acme/Support"}}, wantBody: `{"categories":["Acme/Support"]}`},
		{name: "every category off", update: &message_update.Update{Categories: []string{}}, wantBody: `{"categories":[]}`},
		{name: "read", update: &message_update.Update{IsRead: &read}, wantBody: `{"isRead":true}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
				writeJson(w, http.StatusOK, `{"id":"m1"}`)
			})
			if _, err := client.UpdateMessage(t.Context(), userId, "m1", testCase.update); err != nil {
				t.Fatalf("%s: %v", testCase.name, err)
			}
			got := fake.only(t)
			if got.method != http.MethodPatch || got.uri != "/v1.0/users/"+userId+"/messages/m1" || string(got.body) != testCase.wantBody {
				t.Errorf("%s: %s %s %s, want PATCH with %s", testCase.name, got.method, got.uri, got.body, testCase.wantBody)
			}
		})
	}
}

func TestMoveMessage(t *testing.T) {
	t.Parallel()

	fake, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
		writeJson(w, http.StatusCreated, `{"id":"m1","parentFolderId":"f-archive"}`)
	})
	got, err := client.MoveMessage(t.Context(), userId, "m1", FolderArchive)
	if err != nil {
		t.Fatalf("move message: %v", err)
	}
	request := fake.only(t)
	if request.method != http.MethodPost || request.uri != "/v1.0/users/"+userId+"/messages/m1/move" {
		t.Errorf("%s %s", request.method, request.uri)
	}
	var body map[string]string
	if err := json.Unmarshal(request.body, &body); err != nil || body["destinationId"] != FolderArchive {
		t.Errorf("body %s: %v", request.body, err)
	}
	if got.Id != "m1" || got.ParentFolderId != "f-archive" {
		t.Errorf("message %+v", got)
	}
}

func TestGetAttachment(t *testing.T) {
	t.Parallel()

	fake, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
		writeJson(w, http.StatusOK, `{"@odata.type":"#microsoft.graph.itemAttachment","id":"a1","name":"Fwd",`+
			`"item":{"subject":"inner","body":{"contentType":"html","content":"<p>x</p>"}}}`)
	})
	got, err := client.GetAttachment(t.Context(), userId, "m1", "a1", get_attachment_config.WithExpand(get_attachment_config.ExpandItem))
	if err != nil {
		t.Fatalf("get attachment: %v", err)
	}
	wantUri := "/v1.0/users/" + userId + "/messages/m1/attachments/a1?$expand=microsoft.graph.itemattachment%2Fitem"
	if uri := fake.only(t).uri; uri != wantUri {
		t.Errorf("uri %s, want %s", uri, wantUri)
	}
	if got.Item == nil || got.Item.Subject != "inner" {
		t.Errorf("attachment %+v", got)
	}
}

func TestOpenAttachmentContent(t *testing.T) {
	t.Parallel()

	fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
		if strings.Contains(r.uri, "/missing/") {
			writeJson(w, http.StatusNotFound, `{"error":{"code":"ErrorItemNotFound"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-1.7 bytes")
	})

	content, err := client.OpenAttachmentContent(t.Context(), userId, "m1", "a1")
	if err != nil {
		t.Fatalf("open attachment content: %v", err)
	}
	data, err := io.ReadAll(content.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := content.Body.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
	if string(data) != "%PDF-1.7 bytes" || content.ContentType != "application/pdf" || content.Length != int64(len(data)) {
		t.Errorf("content %q %s %d", data, content.ContentType, content.Length)
	}
	if uri := fake.only(t).uri; uri != "/v1.0/users/"+userId+"/messages/m1/attachments/a1/$value" {
		t.Errorf("uri %s", uri)
	}

	_, err = client.OpenAttachmentContent(t.Context(), userId, "missing", "a1")
	statusErr, ok := errors.AsType[*altshiftHttpErrors.Non2xxStatusCodeError](err)
	if !ok || statusErr.StatusCode != http.StatusNotFound {
		t.Errorf("err = %v, want a 404", err)
	}
}

func TestListMasterCategories_FollowsPages(t *testing.T) {
	t.Parallel()

	var base string
	fake, client := newFakeGraph(t, func(w http.ResponseWriter, r *request) {
		if strings.Contains(r.uri, "skip") {
			writeJson(w, http.StatusOK, `{"value":[{"id":"2","displayName":"Acme/Billing","color":"preset1"}]}`)
			return
		}
		writeJson(w, http.StatusOK, `{"value":[{"id":"1","displayName":"Acme/Support","color":"preset0"}],"@odata.nextLink":"`+base+`/v1.0/users/`+userId+`/outlook/masterCategories?skip=1"}`)
	})
	base = fake.server.URL

	got, err := client.ListMasterCategories(t.Context(), userId)
	if err != nil {
		t.Fatalf("list master categories: %v", err)
	}
	if len(got) != 2 || got[0].DisplayName != "Acme/Support" || got[1].Color != "preset1" {
		t.Errorf("categories %+v", got)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.requests) != 2 || fake.requests[0].uri != "/v1.0/users/"+userId+"/outlook/masterCategories" {
		t.Errorf("requests %+v", fake.requests)
	}
}

func TestGetMailFolder(t *testing.T) {
	t.Parallel()

	fake, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
		writeJson(w, http.StatusOK, `{"id":"f1","displayName":"Archive","totalItemCount":7}`)
	})
	got, err := client.GetMailFolder(t.Context(), userId, FolderArchive)
	if err != nil {
		t.Fatalf("get mail folder: %v", err)
	}
	if uri := fake.only(t).uri; uri != "/v1.0/users/"+userId+"/mailFolders/archive" {
		t.Errorf("uri %s", uri)
	}
	if got.Id != "f1" || got.TotalItemCount != 7 {
		t.Errorf("folder %+v", got)
	}
}

// Every call refuses what it cannot address before it sends anything.
func TestCallsRefuseMissingIds(t *testing.T) {
	t.Parallel()

	_, client := newFakeGraph(t, func(w http.ResponseWriter, _ *request) {
		t.Error("a request was sent")
	})
	ctx := t.Context()

	testCases := []struct {
		name string
		call func() error
	}{
		{name: "list without a user", call: func() error { _, err := client.ListMessagesPage(ctx, ""); return err }},
		{name: "list with a negative top", call: func() error {
			_, err := client.ListMessagesPage(ctx, userId, list_messages_config.WithTop(-1))
			return err
		}},
		{name: "get without a message", call: func() error { _, err := client.GetMessage(ctx, userId, ""); return err }},
		{name: "update without an update", call: func() error { _, err := client.UpdateMessage(ctx, userId, "m1", nil); return err }},
		{name: "move without a destination", call: func() error { _, err := client.MoveMessage(ctx, userId, "m1", ""); return err }},
		{name: "attachment without an id", call: func() error { _, err := client.GetAttachment(ctx, userId, "m1", ""); return err }},
		{name: "content without a message", call: func() error { _, err := client.OpenAttachmentContent(ctx, userId, "", "a1"); return err }},
		{name: "categories without a user", call: func() error { _, err := client.ListMasterCategories(ctx, ""); return err }},
		{name: "folder without an id", call: func() error { _, err := client.GetMailFolder(ctx, userId, ""); return err }},
		{name: "next link check without a user", call: func() error { return client.CheckNextLink("", "https://graph.microsoft.com/v1.0/users/x/messages") }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := testCase.call(); err == nil {
				t.Errorf("%s: no error", testCase.name)
			}
		})
	}
}

func TestNewClient_DefaultBaseUrl(t *testing.T) {
	t.Parallel()

	client := NewClient()
	if got := client.baseUrl.String(); got != "https://graph.microsoft.com/v1.0" {
		t.Errorf("base url %s", got)
	}
	if err := client.CheckNextLink(userId, "https://graph.microsoft.com/v1.0/users/"+userId+"/messages?$skip=10"); err != nil {
		t.Errorf("a next link of Graph refused: %v", err)
	}
}
