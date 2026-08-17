package create_permission_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.SendNotificationEmail != nil {
		t.Errorf("default SendNotificationEmail = %v, want nil", *defaults.SendNotificationEmail)
	}
	if defaults.EmailMessage != "" {
		t.Errorf("default EmailMessage = %q, want empty", defaults.EmailMessage)
	}
	if defaults.TransferOwnership {
		t.Error("default TransferOwnership = true, want false")
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	config := New(
		WithSendNotificationEmail(false),
		WithEmailMessage("message"),
		WithTransferOwnership(true),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if config.SendNotificationEmail == nil || *config.SendNotificationEmail {
		t.Errorf("SendNotificationEmail = %v, want false", config.SendNotificationEmail)
	}
	if config.EmailMessage != "message" {
		t.Errorf("EmailMessage = %q, want %q", config.EmailMessage, "message")
	}
	if !config.TransferOwnership {
		t.Error("TransferOwnership = false, want true")
	}
	if len(config.FetchOptions) != 1 {
		t.Fatalf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}
