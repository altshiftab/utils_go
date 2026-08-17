package sheets

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/get_spreadsheet_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/get_values_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/sheets_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/types/spreadsheet"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/types/spreadsheet/sheet"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/types/spreadsheet/sheet/sheet_properties"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/types/value_range"
)

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(sheets_config.WithBaseUrl(u))
}

func TestGetSpreadsheet(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v4/spreadsheets/sheet-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &spreadsheet.Spreadsheet{
			SpreadsheetId: "sheet-1",
			Sheets: []*sheet.Sheet{
				{Properties: &sheet_properties.SheetProperties{SheetId: 1169986365, Title: "Besök"}},
			},
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	result, err := client.GetSpreadsheet(context.Background(), "sheet-1")
	if err != nil {
		t.Fatalf("get spreadsheet: %v", err)
	}
	if result.SpreadsheetId != "sheet-1" {
		t.Errorf("SpreadsheetId = %q, want %q", result.SpreadsheetId, "sheet-1")
	}
	if len(result.Sheets) != 1 {
		t.Fatalf("Sheets len = %d, want 1", len(result.Sheets))
	}
	properties := result.Sheets[0].Properties
	if properties == nil {
		t.Fatal("nil sheet properties")
	}
	if properties.SheetId != 1169986365 {
		t.Errorf("SheetId = %d, want 1169986365", properties.SheetId)
	}
	if properties.Title != "Besök" {
		t.Errorf("Title = %q, want %q", properties.Title, "Besök")
	}
}

func TestGetSpreadsheetWithFields(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if fields := r.URL.Query().Get("fields"); fields != "sheets.properties(sheetId,title)" {
			t.Errorf("fields = %q, want %q", fields, "sheets.properties(sheetId,title)")
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &spreadsheet.Spreadsheet{}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	if _, err := client.GetSpreadsheet(
		context.Background(),
		"sheet-1",
		get_spreadsheet_config.WithFields("sheets.properties(sheetId,title)"),
	); err != nil {
		t.Fatalf("get spreadsheet: %v", err)
	}
}

func TestGetSpreadsheetEmptySpreadsheetId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	if _, err := client.GetSpreadsheet(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty spreadsheet id")
	}
}

func TestGetSpreadsheetCancelledContext(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.GetSpreadsheet(ctx, "sheet-1"); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGetValues(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		// r.URL.Path is decoded; the escaped range arrives as its literal form.
		if !strings.HasSuffix(r.URL.Path, "/v4/spreadsheets/sheet-1/values/'Besök'!A2:E") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &value_range.ValueRange{
			Range:          "'Besök'!A2:E5",
			MajorDimension: "ROWS",
			Values: [][]string{
				{"2026-05-09", "Judarskogen (Stockholm)", "Stockholms Län", "2000019", ""},
				{"2026-06-17", "Hansta (Sollentuna, Stockholm)", "Stockholms Län", "2000140", "Besökte bara södra delen"},
			},
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	result, err := client.GetValues(context.Background(), "sheet-1", "'Besök'!A2:E")
	if err != nil {
		t.Fatalf("get values: %v", err)
	}
	if result.Range != "'Besök'!A2:E5" {
		t.Errorf("Range = %q, want %q", result.Range, "'Besök'!A2:E5")
	}
	if len(result.Values) != 2 {
		t.Fatalf("Values len = %d, want 2", len(result.Values))
	}
	if result.Values[1][3] != "2000140" {
		t.Errorf("Values[1][3] = %q, want %q", result.Values[1][3], "2000140")
	}
	if result.Values[1][4] != "Besökte bara södra delen" {
		t.Errorf("Values[1][4] = %q, want %q", result.Values[1][4], "Besökte bara södra delen")
	}
}

func TestGetValuesWithMajorDimension(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if majorDimension := r.URL.Query().Get("majorDimension"); majorDimension != "COLUMNS" {
			t.Errorf("majorDimension = %q, want %q", majorDimension, "COLUMNS")
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &value_range.ValueRange{}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	if _, err := client.GetValues(
		context.Background(),
		"sheet-1",
		"Besok",
		get_values_config.WithMajorDimension(get_values_config.MajorDimensionColumns),
	); err != nil {
		t.Fatalf("get values: %v", err)
	}
}

func TestGetValuesEmptySpreadsheetId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	if _, err := client.GetValues(context.Background(), "", "Besok"); err == nil {
		t.Fatal("expected error for empty spreadsheet id")
	}
}

func TestGetValuesEmptyRange(t *testing.T) {
	t.Parallel()

	client := NewClient()
	if _, err := client.GetValues(context.Background(), "sheet-1", ""); err == nil {
		t.Fatal("expected error for empty values range")
	}
}

func TestGetValuesCancelledContext(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.GetValues(ctx, "sheet-1", "Besok"); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSpreadsheetUrl(t *testing.T) {
	t.Parallel()

	client := NewClient()
	expected := "https://sheets.googleapis.com/v4/spreadsheets/sheet-1"
	if u := client.spreadsheetUrl("sheet-1"); u != expected {
		t.Errorf("spreadsheetUrl = %q, want %q", u, expected)
	}
}

func TestValuesUrl(t *testing.T) {
	t.Parallel()

	client := NewClient()
	expected := "https://sheets.googleapis.com/v4/spreadsheets/sheet-1/values/%27Bes%C3%B6k%27%21A2:E"
	if u := client.valuesUrl("sheet-1", "'Besök'!A2:E"); u != expected {
		t.Errorf("valuesUrl = %q, want %q", u, expected)
	}
}
