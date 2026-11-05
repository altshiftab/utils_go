package write_event_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.SendUpdates != "" || defaults.ConferenceDataVersion != 0 || len(defaults.FetchOptions) != 0 {
		t.Errorf("defaults = %+v, want zero", defaults)
	}

	config := New(
		WithSendUpdates(SendUpdatesExternalOnly),
		WithConferenceDataVersion(1),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if config.SendUpdates != "externalOnly" || config.ConferenceDataVersion != 1 || len(config.FetchOptions) != 1 {
		t.Errorf("config = %+v", config)
	}
}
