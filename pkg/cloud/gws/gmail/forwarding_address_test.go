package gmail

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/forwarding_address"
)

func TestForwardingAddressesUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		userId          string
		forwardingEmail string
		want            string
	}{
		{
			name:   "collection",
			userId: "me",
			want:   "https://gmail.googleapis.com/gmail/v1/users/me/settings/forwardingAddresses",
		},
		{
			name:            "one address",
			userId:          "me",
			forwardingEmail: "inbox@example.com",
			want:            "https://gmail.googleapis.com/gmail/v1/users/me/settings/forwardingAddresses/inbox@example.com",
		},
	}

	client := NewClient()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := client.forwardingAddressesUrl(testCase.userId, testCase.forwardingEmail); got != testCase.want {
				t.Errorf("url = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestCreateForwardingAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		userId      string
		input       *forwarding_address.ForwardingAddress
		status      int
		wantErr     bool
		wantRequest bool
		wantStatus  forwarding_address.VerificationStatus
	}{
		{
			name:        "external address is pending",
			userId:      "me",
			input:       &forwarding_address.ForwardingAddress{ForwardingEmail: "inbox@example.com"},
			status:      http.StatusOK,
			wantRequest: true,
			wantStatus:  forwarding_address.VerificationStatusPending,
		},
		{
			name:        "refused",
			userId:      "me",
			input:       &forwarding_address.ForwardingAddress{ForwardingEmail: "inbox@example.com"},
			status:      http.StatusForbidden,
			wantErr:     true,
			wantRequest: true,
		},
		{
			name:    "empty user id",
			input:   &forwarding_address.ForwardingAddress{ForwardingEmail: "inbox@example.com"},
			wantErr: true,
		},
		{
			name:    "nil address",
			userId:  "me",
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			requested := false
			client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
				requested = true
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				if r.URL.Path != "/gmail/v1/users/me/settings/forwardingAddresses" {
					t.Errorf("path = %s", r.URL.Path)
				}

				var input forwarding_address.ForwardingAddress
				if err := json.UnmarshalRead(r.Body, &input); err != nil {
					t.Errorf("unmarshal: %v", err)
				}
				if input.VerificationStatus != "" {
					t.Errorf("request carried verification status %q", input.VerificationStatus)
				}

				if testCase.status != http.StatusOK {
					w.WriteHeader(testCase.status)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.MarshalWrite(w, &forwarding_address.ForwardingAddress{
					ForwardingEmail:    input.ForwardingEmail,
					VerificationStatus: forwarding_address.VerificationStatusPending,
				}); err != nil {
					t.Errorf("marshal: %v", err)
				}
			})

			created, err := client.CreateForwardingAddress(context.Background(), testCase.userId, testCase.input)
			if requested != testCase.wantRequest {
				t.Errorf("requested = %v, want %v", requested, testCase.wantRequest)
			}
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if created.ForwardingEmail != testCase.input.ForwardingEmail {
				t.Errorf("email = %q, want %q", created.ForwardingEmail, testCase.input.ForwardingEmail)
			}
			if created.VerificationStatus != testCase.wantStatus {
				t.Errorf("status = %q, want %q", created.VerificationStatus, testCase.wantStatus)
			}
		})
	}
}

func TestGetForwardingAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		userId          string
		forwardingEmail string
		status          int
		wantErr         bool
		wantStatus      forwarding_address.VerificationStatus
	}{
		{
			name:            "accepted",
			userId:          "me",
			forwardingEmail: "inbox@example.com",
			status:          http.StatusOK,
			wantStatus:      forwarding_address.VerificationStatusAccepted,
		},
		{
			name:            "missing",
			userId:          "me",
			forwardingEmail: "inbox@example.com",
			status:          http.StatusNotFound,
			wantErr:         true,
		},
		{name: "empty user id", forwardingEmail: "inbox@example.com", wantErr: true},
		{name: "empty forwarding email", userId: "me", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %s, want GET", r.Method)
				}
				if r.URL.Path != "/gmail/v1/users/me/settings/forwardingAddresses/inbox@example.com" {
					t.Errorf("path = %s", r.URL.Path)
				}
				if testCase.status != http.StatusOK {
					w.WriteHeader(testCase.status)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.MarshalWrite(w, &forwarding_address.ForwardingAddress{
					ForwardingEmail:    "inbox@example.com",
					VerificationStatus: forwarding_address.VerificationStatusAccepted,
				}); err != nil {
					t.Errorf("marshal: %v", err)
				}
			})

			got, err := client.GetForwardingAddress(context.Background(), testCase.userId, testCase.forwardingEmail)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.VerificationStatus != testCase.wantStatus {
				t.Errorf("status = %q, want %q", got.VerificationStatus, testCase.wantStatus)
			}
		})
	}
}

func TestListForwardingAddresses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		userId  string
		body    string
		wantErr bool
		want    []string
	}{
		{
			name:   "two addresses",
			userId: "me",
			body:   `{"forwardingAddresses":[{"forwardingEmail":"a@example.com","verificationStatus":"accepted"},{"forwardingEmail":"b@example.com","verificationStatus":"pending"}]}`,
			want:   []string{"a@example.com", "b@example.com"},
		},
		{
			// Gmail leaves the field out entirely when there are none.
			name:   "none",
			userId: "me",
			body:   `{}`,
		},
		{name: "empty user id", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/gmail/v1/users/me/settings/forwardingAddresses" {
					t.Errorf("path = %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(testCase.body))
			})

			addresses, err := client.ListForwardingAddresses(context.Background(), testCase.userId)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got []string
			for _, address := range addresses {
				got = append(got, address.ForwardingEmail)
			}
			if !slices.Equal(got, testCase.want) {
				t.Errorf("addresses = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestDeleteForwardingAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		userId          string
		forwardingEmail string
		status          int
		wantErr         bool
	}{
		{name: "deleted", userId: "me", forwardingEmail: "inbox@example.com", status: http.StatusNoContent},
		{name: "refused", userId: "me", forwardingEmail: "inbox@example.com", status: http.StatusForbidden, wantErr: true},
		{name: "empty user id", forwardingEmail: "inbox@example.com", wantErr: true},
		{name: "empty forwarding email", userId: "me", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Errorf("method = %s, want DELETE", r.Method)
				}
				if r.URL.Path != "/gmail/v1/users/me/settings/forwardingAddresses/inbox@example.com" {
					t.Errorf("path = %s", r.URL.Path)
				}
				w.WriteHeader(testCase.status)
			})

			err := client.DeleteForwardingAddress(context.Background(), testCase.userId, testCase.forwardingEmail)
			if (err != nil) != testCase.wantErr {
				t.Errorf("err = %v, wantErr %v", err, testCase.wantErr)
			}
		})
	}
}
