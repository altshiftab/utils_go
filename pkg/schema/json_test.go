package schema

import (
	"encoding/json/v2"
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/net/types/domain_parts"
)

// marshalToMap marshals v and unmarshals the result into a generic map so that
// tests can assert on individual JSON fields without depending on key ordering.
func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(%T) returned error: %v", v, err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal into map returned error: %v (raw: %s)", err, string(data))
	}

	return m
}

// hasKey reports whether the (possibly nested) key path exists in m.
func lookup(t *testing.T, m map[string]any, path ...string) (any, bool) {
	t.Helper()

	var current any = m
	for _, key := range path {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = asMap[key]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

func TestMarshalOmitzero(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		value  any
		assert func(t *testing.T, m map[string]any)
	}{
		{
			name:  "empty base marshals to empty object",
			value: Base{},
			assert: func(t *testing.T, m map[string]any) {
				if len(m) != 0 {
					t.Errorf("expected empty object, got %v", m)
				}
			},
		},
		{
			name:  "custom timestamp tag name",
			value: Base{Timestamp: "2026-01-01T00:00:00Z"},
			assert: func(t *testing.T, m map[string]any) {
				if _, ok := m["timestamp"]; ok {
					t.Error("did not expect field name 'timestamp'")
				}
				if v, ok := m["@timestamp"]; !ok || v != "2026-01-01T00:00:00Z" {
					t.Errorf("@timestamp = %v (ok=%v), want the configured value", v, ok)
				}
			},
		},
		{
			name:  "only set scalar field is emitted",
			value: Base{Message: "hello"},
			assert: func(t *testing.T, m map[string]any) {
				if len(m) != 1 {
					t.Fatalf("expected exactly one key, got %v", m)
				}
				if m["message"] != "hello" {
					t.Errorf("message = %v, want hello", m["message"])
				}
			},
		},
		{
			name:  "map and slice fields",
			value: Base{Labels: map[string]string{"env": "prod"}, Tags: []string{"a", "b"}},
			assert: func(t *testing.T, m map[string]any) {
				labels, ok := m["labels"].(map[string]any)
				if !ok || labels["env"] != "prod" {
					t.Errorf("labels = %v, want env=prod", m["labels"])
				}
				tags, ok := m["tags"].([]any)
				if !ok || len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
					t.Errorf("tags = %v, want [a b]", m["tags"])
				}
			},
		},
		{
			name:  "nil pointer field omitted",
			value: Target{Address: "addr", Nat: nil},
			assert: func(t *testing.T, m map[string]any) {
				if _, ok := m["nat"]; ok {
					t.Error("nil nat pointer should be omitted")
				}
				if m["address"] != "addr" {
					t.Errorf("address = %v, want addr", m["address"])
				}
			},
		},
		{
			name:  "non-nil nested pointer field emitted",
			value: Target{Nat: &Nat{Ip: "192.0.2.1", Port: 4444}},
			assert: func(t *testing.T, m map[string]any) {
				ip, ok := lookup(t, m, "nat", "ip")
				if !ok || ip != "192.0.2.1" {
					t.Errorf("nat.ip = %v (ok=%v), want 192.0.2.1", ip, ok)
				}
				port, ok := lookup(t, m, "nat", "port")
				if !ok || port != float64(4444) {
					t.Errorf("nat.port = %v (ok=%v), want 4444", port, ok)
				}
			},
		},
		{
			name:  "embedded domain_parts promoted with omitempty",
			value: Target{Parts: domain_parts.Parts{RegisteredDomain: "example.com"}, Address: "a"},
			assert: func(t *testing.T, m map[string]any) {
				if m["registered_domain"] != "example.com" {
					t.Errorf("registered_domain = %v, want example.com", m["registered_domain"])
				}
				if _, ok := m["subdomain"]; ok {
					t.Error("empty subdomain should be omitted (omitempty)")
				}
				if m["address"] != "a" {
					t.Errorf("address = %v, want a", m["address"])
				}
			},
		},
		{
			name:  "zero-value embedded struct field omitted",
			value: Host{Name: "h1"},
			assert: func(t *testing.T, m map[string]any) {
				if _, ok := m["os"]; ok {
					t.Error("zero-value Os should be omitted by omitzero")
				}
				if m["name"] != "h1" {
					t.Errorf("name = %v, want h1", m["name"])
				}
			},
		},
		{
			name:  "non-zero value struct field emitted",
			value: Host{Os: Os{Name: "linux"}},
			assert: func(t *testing.T, m map[string]any) {
				name, ok := lookup(t, m, "os", "name")
				if !ok || name != "linux" {
					t.Errorf("os.name = %v (ok=%v), want linux", name, ok)
				}
			},
		},
		{
			name:  "slice of pointer structs",
			value: Email{To: []*EmailAddress{{Address: "a@example.com"}, {Name: "B", Address: "b@example.com"}}},
			assert: func(t *testing.T, m map[string]any) {
				to, ok := m["to"].([]any)
				if !ok || len(to) != 2 {
					t.Fatalf("to = %v, want 2 elements", m["to"])
				}
				first, ok := to[0].(map[string]any)
				if !ok || first["address"] != "a@example.com" {
					t.Errorf("to[0] = %v, want address a@example.com", to[0])
				}
				second, ok := to[1].(map[string]any)
				if !ok || second["name"] != "B" || second["address"] != "b@example.com" {
					t.Errorf("to[1] = %v, want name B address b@example.com", to[1])
				}
			},
		},
		{
			name:  "nil optional int pointer omitted",
			value: Tcp{State: "SYN_SENT"},
			assert: func(t *testing.T, m map[string]any) {
				if _, ok := m["sequence_number"]; ok {
					t.Error("nil sequence_number pointer should be omitted")
				}
				if _, ok := m["acknowledgement_number"]; ok {
					t.Error("nil acknowledgement_number pointer should be omitted")
				}
				if m["state"] != "SYN_SENT" {
					t.Errorf("state = %v, want SYN_SENT", m["state"])
				}
			},
		},
		{
			name:  "non-nil int pointer to zero is emitted",
			value: Tcp{SequenceNumber: new(0)},
			assert: func(t *testing.T, m map[string]any) {
				v, ok := m["sequence_number"]
				if !ok {
					t.Fatal("non-nil pointer to zero must not be omitted")
				}
				if v != float64(0) {
					t.Errorf("sequence_number = %v, want 0", v)
				}
			},
		},
		{
			name:  "interface any field with value emitted",
			value: Geo{Location: map[string]any{"lat": 1.5, "lon": 2.5}},
			assert: func(t *testing.T, m map[string]any) {
				lat, ok := lookup(t, m, "location", "lat")
				if !ok || lat != 1.5 {
					t.Errorf("location.lat = %v (ok=%v), want 1.5", lat, ok)
				}
			},
		},
		{
			name:  "nil interface any field omitted",
			value: Geo{Name: "somewhere"},
			assert: func(t *testing.T, m map[string]any) {
				if _, ok := m["location"]; ok {
					t.Error("nil location interface should be omitted")
				}
				if m["name"] != "somewhere" {
					t.Errorf("name = %v, want somewhere", m["name"])
				}
			},
		},
		{
			name:  "recursive error cause",
			value: Error{Message: "outer", Cause: &Error{Message: "inner"}},
			assert: func(t *testing.T, m map[string]any) {
				if m["message"] != "outer" {
					t.Errorf("message = %v, want outer", m["message"])
				}
				inner, ok := lookup(t, m, "cause", "message")
				if !ok || inner != "inner" {
					t.Errorf("cause.message = %v (ok=%v), want inner", inner, ok)
				}
			},
		},
		{
			name:  "int64 numeric field",
			value: AutonomousSystem{Number: 64512, Organization: &Organization{Name: "Example ISP"}},
			assert: func(t *testing.T, m map[string]any) {
				if m["number"] != float64(64512) {
					t.Errorf("number = %v, want 64512", m["number"])
				}
				org, ok := lookup(t, m, "organization", "name")
				if !ok || org != "Example ISP" {
					t.Errorf("organization.name = %v (ok=%v), want Example ISP", org, ok)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			m := marshalToMap(t, testCase.value)
			testCase.assert(t, m)
		})
	}
}

func TestMarshalNestedBaseRoundTrip(t *testing.T) {
	t.Parallel()

	original := Base{
		Timestamp: "2026-08-03T12:00:00Z",
		Message:   "connection observed",
		Tags:      []string{"network", "flow"},
		Source:    &Target{Ip: "10.0.0.1", Port: 12345},
		Destination: &Target{
			Ip:   "10.0.0.2",
			Port: 443,
			As:   &AutonomousSystem{Number: 64500},
		},
		Network: &Network{Transport: "tcp", Bytes: 2048},
		Http: &Http{
			Version:  "2",
			Request:  &HttpRequest{Method: http.MethodGet, Bytes: 100},
			Response: &HttpResponse{StatusCode: http.StatusOK},
		},
	}

	data, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var decoded Base
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %q, want %q", decoded.Timestamp, original.Timestamp)
	}
	if decoded.Message != original.Message {
		t.Errorf("Message = %q, want %q", decoded.Message, original.Message)
	}
	if len(decoded.Tags) != 2 || decoded.Tags[0] != "network" || decoded.Tags[1] != "flow" {
		t.Errorf("Tags = %v, want [network flow]", decoded.Tags)
	}
	if decoded.Source == nil || decoded.Source.Ip != "10.0.0.1" || decoded.Source.Port != 12345 {
		t.Errorf("Source = %+v, want ip=10.0.0.1 port=12345", decoded.Source)
	}
	if decoded.Destination == nil || decoded.Destination.As == nil || decoded.Destination.As.Number != 64500 {
		t.Errorf("Destination.As = %+v, want number=64500", decoded.Destination)
	}
	if decoded.Network == nil || decoded.Network.Transport != "tcp" || decoded.Network.Bytes != 2048 {
		t.Errorf("Network = %+v, want transport=tcp bytes=2048", decoded.Network)
	}
	if decoded.Http == nil || decoded.Http.Request == nil || decoded.Http.Request.Method != http.MethodGet {
		t.Errorf("Http.Request = %+v, want method=GET", decoded.Http)
	}
	if decoded.Http == nil || decoded.Http.Response == nil || decoded.Http.Response.StatusCode != http.StatusOK {
		t.Errorf("Http.Response = %+v, want status_code=200", decoded.Http)
	}
}
