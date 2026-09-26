package dialer_config

import (
	"net"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.IpType != IpTypePublic || defaults.Port != DefaultPort || defaults.NetDialer == nil || defaults.RefreshBuffer <= 0 || defaults.Now == nil {
		t.Errorf("defaults = %+v", defaults)
	}

	fixed := time.Unix(1, 0)
	netDialer := &net.Dialer{}
	config := New(
		WithIpType(IpTypePrivate),
		WithPort("1234"),
		WithNetDialer(netDialer),
		WithRefreshBuffer(time.Second),
		WithNow(func() time.Time { return fixed }),
	)
	if config.IpType != IpTypePrivate || config.Port != "1234" || config.NetDialer != netDialer || config.RefreshBuffer != time.Second || !config.Now().Equal(fixed) {
		t.Errorf("config = %+v", config)
	}
}
