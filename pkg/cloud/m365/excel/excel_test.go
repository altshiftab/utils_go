package excel

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/add_table_rows_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/excel_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/request_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/types/table"
)

type request struct {
	method    string
	uri       string
	sessionId string
	body      string
}

func testClient(t *testing.T, status int, response any) (*Client, *[]*request) {
	t.Helper()
	var requests []*request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		requests = append(requests, &request{
			method:    r.Method,
			uri:       r.RequestURI,
			sessionId: r.Header.Get(SessionIdHeader),
			body:      string(body),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if response != nil {
			if err := json.MarshalWrite(w, response); err != nil {
				t.Errorf("marshal: %v", err)
			}
		}
	}))
	t.Cleanup(server.Close)

	baseUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(excel_config.WithBaseUrl(baseUrl)), &requests
}

func TestRequests(t *testing.T) {
	t.Parallel()

	const workbook = "/v1.0/drives/b%21d/items/01X/workbook"
	ctx := context.Background()

	testCases := []struct {
		name          string
		call          func(c *Client) error
		status        int
		response      any
		wantMethod    string
		wantUri       string
		wantSessionId string
		wantBody      string
	}{
		{
			name: "create session",
			call: func(c *Client) error {
				s, err := c.CreateSession(ctx, "b!d", "01X", true)
				if err == nil && s.Id != "sess-1" {
					t.Errorf("session id = %q", s.Id)
				}
				return err
			},
			response:   map[string]any{"id": "sess-1", "persistChanges": true},
			wantMethod: http.MethodPost,
			wantUri:    workbook + "/createSession",
			wantBody:   `{"persistChanges":true}`,
		},
		{
			name:          "close session",
			call:          func(c *Client) error { return c.CloseSession(ctx, "b!d", "01X", "sess-1") },
			status:        http.StatusNoContent,
			wantMethod:    http.MethodPost,
			wantUri:       workbook + "/closeSession",
			wantSessionId: "sess-1",
		},
		{
			name: "list tables",
			call: func(c *Client) error {
				tables, err := c.ListTables(ctx, "b!d", "01X")
				if err == nil && (len(tables) != 1 || tables[0].Name != "Besök") {
					t.Errorf("tables = %+v", tables)
				}
				return err
			},
			response:   map[string]any{"value": []*table.Table{{Id: "1", Name: "Besök"}}},
			wantMethod: http.MethodGet,
			wantUri:    workbook + "/tables",
		},
		{
			name: "add table",
			call: func(c *Client) error {
				_, err := c.AddTable(ctx, "b!d", "01X", "Sheet1!A1:C1", true, request_config.WithSessionId("sess-1"))
				return err
			},
			response:      &table.Table{Id: "2", Name: "Table1"},
			wantMethod:    http.MethodPost,
			wantUri:       workbook + "/tables/add",
			wantSessionId: "sess-1",
			wantBody:      `{"address":"Sheet1!A1:C1","hasHeaders":true}`,
		},
		{
			name: "add table rows appends",
			call: func(c *Client) error {
				_, err := c.AddTableRows(ctx, "b!d", "01X", "Besök", [][]any{{"2026-10-01", "Anna", 3}, {"2026-10-02", nil, 4.5}})
				return err
			},
			response:   map[string]any{"index": 7},
			wantMethod: http.MethodPost,
			wantUri:    workbook + "/tables/Bes%C3%B6k/rows",
			wantBody:   `{"values":[["2026-10-01","Anna",3],["2026-10-02",null,4.5]]}`,
		},
		{
			name: "add table rows at index zero in session",
			call: func(c *Client) error {
				_, err := c.AddTableRows(
					ctx, "b!d", "01X", "Besök", [][]any{{true}},
					add_table_rows_config.WithIndex(0), add_table_rows_config.WithSessionId("sess-1"),
				)
				return err
			},
			response:      map[string]any{"index": 0},
			wantMethod:    http.MethodPost,
			wantUri:       workbook + "/tables/Bes%C3%B6k/rows",
			wantSessionId: "sess-1",
			wantBody:      `{"index":0,"values":[[true]]}`,
		},
		{
			name: "list table rows",
			call: func(c *Client) error {
				rows, err := c.ListTableRows(ctx, "b!d", "01X", "Besök")
				if err == nil && (len(rows) != 1 || rows[0].Values[0][1] != "Anna") {
					t.Errorf("rows = %+v", rows)
				}
				return err
			},
			response:   map[string]any{"value": []map[string]any{{"index": 0, "values": [][]any{{"2026-10-01", "Anna"}}}}},
			wantMethod: http.MethodGet,
			wantUri:    workbook + "/tables/Bes%C3%B6k/rows",
		},
		{
			name: "get range",
			call: func(c *Client) error {
				r, err := c.GetRange(ctx, "b!d", "01X", "Mitt blad", "A1:B2")
				if err == nil && r.Address != "'Mitt blad'!A1:B2" {
					t.Errorf("address = %q", r.Address)
				}
				return err
			},
			response:   map[string]any{"address": "'Mitt blad'!A1:B2"},
			wantMethod: http.MethodGet,
			wantUri:    workbook + "/worksheets/Mitt%20blad/range(address='A1:B2')",
		},
		{
			name: "update range escapes quotes in the address",
			call: func(c *Client) error {
				_, err := c.UpdateRange(ctx, "b!d", "01X", "Sheet1", "Named'Range", [][]any{{"=SUM(A1:A3)"}})
				return err
			},
			response:   map[string]any{"address": "Sheet1!A4"},
			wantMethod: http.MethodPatch,
			wantUri:    workbook + "/worksheets/Sheet1/range(address='Named%27%27Range')",
			wantBody:   `{"values":[["=SUM(A1:A3)"]]}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			status := testCase.status
			if status == 0 {
				status = http.StatusOK
			}
			client, requests := testClient(t, status, testCase.response)
			if err := testCase.call(client); err != nil {
				t.Fatalf("call: %v", err)
			}
			if len(*requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(*requests))
			}
			got := (*requests)[0]
			if got.method != testCase.wantMethod || got.uri != testCase.wantUri {
				t.Errorf("request = %s %s, want %s %s", got.method, got.uri, testCase.wantMethod, testCase.wantUri)
			}
			if got.sessionId != testCase.wantSessionId {
				t.Errorf("session id = %q, want %q", got.sessionId, testCase.wantSessionId)
			}
			if got.body != testCase.wantBody {
				t.Errorf("body = %s, want %s", got.body, testCase.wantBody)
			}
		})
	}
}

func TestSessionHeaderDoesNotLeakBetweenCalls(t *testing.T) {
	t.Parallel()

	client, requests := testClient(t, http.StatusOK, map[string]any{"value": []any{}})
	if _, err := client.ListTables(context.Background(), "d", "i", request_config.WithSessionId("sess-1")); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if _, err := client.ListTables(context.Background(), "d", "i"); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if got := (*requests)[1].sessionId; got != "" {
		t.Errorf("sessionless call sent session id %q", got)
	}
}

func TestValidation(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx := context.Background()
	values := [][]any{{1}}

	testCases := []struct {
		name string
		call func() error
	}{
		{name: "create session drive", call: func() error { _, err := client.CreateSession(ctx, "", "i", false); return err }},
		{name: "create session item", call: func() error { _, err := client.CreateSession(ctx, "d", "", false); return err }},
		{name: "close session id", call: func() error { return client.CloseSession(ctx, "d", "i", "") }},
		{name: "list tables", call: func() error { _, err := client.ListTables(ctx, "", "i"); return err }},
		{name: "add table address", call: func() error { _, err := client.AddTable(ctx, "d", "i", "", true); return err }},
		{name: "add rows table", call: func() error { _, err := client.AddTableRows(ctx, "d", "i", "", values); return err }},
		{name: "add rows values", call: func() error { _, err := client.AddTableRows(ctx, "d", "i", "t", nil); return err }},
		{name: "list rows table", call: func() error { _, err := client.ListTableRows(ctx, "d", "i", ""); return err }},
		{name: "get range worksheet", call: func() error { _, err := client.GetRange(ctx, "d", "i", "", "A1"); return err }},
		{name: "get range address", call: func() error { _, err := client.GetRange(ctx, "d", "i", "s", ""); return err }},
		{name: "update range values", call: func() error { _, err := client.UpdateRange(ctx, "d", "i", "s", "A1", nil); return err }},
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
