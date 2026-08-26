package format

import "testing"

func TestDateFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "leap day in leap year", instance: "2024-02-29", wantErr: false},
		{name: "last day of year", instance: "2024-12-31", wantErr: false},
		{name: "minimal date", instance: "0001-01-01", wantErr: false},
		{name: "leap day in non-leap year", instance: "2023-02-29", wantErr: true},
		{name: "month out of range", instance: "2024-13-01", wantErr: true},
		{name: "day out of range", instance: "2024-01-32", wantErr: true},
		{name: "day zero", instance: "2024-01-00", wantErr: true},
		{name: "non-padded month", instance: "2024-1-01", wantErr: true},
		{name: "signed year", instance: "+124-01-01", wantErr: true},
		{name: "signed day", instance: "2024-01-+9", wantErr: true},
		{name: "missing separators", instance: "20240101", wantErr: true},
		{name: "wrong separators", instance: "2024/01/01", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := dateFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("dateFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestDateTimeFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "uppercase T and Z", instance: "2024-02-29T12:34:56Z", wantErr: false},
		{name: "lowercase t and z", instance: "2024-02-29t12:34:56z", wantErr: false},
		{name: "numeric offset", instance: "2024-01-01T12:00:00+01:00", wantErr: false},
		{name: "fractional seconds", instance: "2024-01-01T12:00:00.123Z", wantErr: false},
		{name: "leap second", instance: "2024-01-01T23:59:60Z", wantErr: false},
		{name: "invalid date part", instance: "2023-02-29T12:34:56Z", wantErr: true},
		{name: "space separator", instance: "2024-01-01 12:00:00Z", wantErr: true},
		{name: "hour out of range", instance: "2024-01-01T24:00:00Z", wantErr: true},
		{name: "missing offset", instance: "2024-01-01T12:00:00", wantErr: true},
		{name: "date only", instance: "2024-01-01", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := dateTimeFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("dateTimeFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

func TestTimeFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "uppercase Z", instance: "12:00:00Z", wantErr: false},
		{name: "lowercase z", instance: "12:00:00z", wantErr: false},
		{name: "positive offset", instance: "12:00:00+01:00", wantErr: false},
		{name: "negative offset", instance: "12:00:00-23:59", wantErr: false},
		{name: "fractional seconds", instance: "12:00:00.5Z", wantErr: false},
		{name: "leap second UTC", instance: "23:59:60Z", wantErr: false},
		{name: "leap second matching negative offset", instance: "22:59:60-01:00", wantErr: false},
		{name: "leap second wrong offset", instance: "23:59:60+01:00", wantErr: true},
		{name: "leap second not at midnight UTC", instance: "12:34:60Z", wantErr: true},
		{name: "missing offset", instance: "12:00:00", wantErr: true},
		{name: "hour out of range", instance: "24:00:00Z", wantErr: true},
		{name: "minute out of range", instance: "12:60:00Z", wantErr: true},
		{name: "second out of range", instance: "12:00:61Z", wantErr: true},
		{name: "offset hour out of range", instance: "12:00:00+24:00", wantErr: true},
		{name: "non-padded offset hour", instance: "12:00:00+1:00", wantErr: true},
		{name: "signed hour", instance: "+5:00:00Z", wantErr: true},
		{name: "signed minute", instance: "00:+5:00Z", wantErr: true},
		{name: "empty fraction", instance: "12:00:00.Z", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := timeFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("timeFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

func TestDurationFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "years only", instance: "P1Y", wantErr: false},
		{name: "months only", instance: "P1M", wantErr: false},
		{name: "weeks only", instance: "P1W", wantErr: false},
		{name: "days only", instance: "P1D", wantErr: false},
		{name: "hours only", instance: "PT1H", wantErr: false},
		{name: "minutes only", instance: "PT1M", wantErr: false},
		{name: "seconds only", instance: "PT1S", wantErr: false},
		{name: "full date and time", instance: "P1Y2M3DT4H5M6S", wantErr: false},
		{name: "days and hours", instance: "P1DT12H", wantErr: false},
		{name: "lowercase designators", instance: "p2w", wantErr: false},
		{name: "P alone", instance: "P", wantErr: true},
		{name: "PT alone", instance: "PT", wantErr: true},
		{name: "seconds without T", instance: "P1S", wantErr: true},
		{name: "missing P", instance: "1Y", wantErr: true},
		{name: "weeks combined with years", instance: "P1Y1W", wantErr: true},
		{name: "weeks combined with time", instance: "P1WT1H", wantErr: true},
		{name: "hours followed directly by seconds", instance: "PT1H1S", wantErr: true},
		{name: "trailing digits after seconds", instance: "PT1S5", wantErr: true},
		{name: "trailing garbage after seconds", instance: "PT1Sjunk", wantErr: true},
		{name: "trailing digits after minutes and seconds", instance: "PT1M2S3", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := durationFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("durationFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestEmailFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "simple address", instance: "user@example.com", wantErr: false},
		{name: "dotted local part and subdomain", instance: "first.last@sub.example.com", wantErr: false},
		{name: "IPv4 address literal domain", instance: "user@[192.168.0.1]", wantErr: false},
		{name: "IPv6 address literal domain", instance: "user@[IPv6:::1]", wantErr: false},
		{name: "IPv6 address literal domain without tag", instance: "user@[::1]", wantErr: true},
		{name: "invalid IPv6 address literal domain", instance: "user@[IPv6:::g]", wantErr: true},
		{name: "quoted local part with space", instance: `"quoted local"@example.com`, wantErr: false},
		{name: "non-ASCII domain", instance: "user@exämple.com", wantErr: true},
		{name: "missing at sign", instance: "plainaddress", wantErr: true},
		{name: "missing domain", instance: "user@", wantErr: true},
		{name: "missing local part", instance: "@example.com", wantErr: true},
		{name: "display name present", instance: "Joe <user@example.com>", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := emailFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("emailFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestIdnEmailFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "ASCII address", instance: "user@example.com", wantErr: false},
		{name: "non-ASCII domain", instance: "user@exämple.com", wantErr: false},
		{name: "non-ASCII local part and domain", instance: "用户@例子.jp", wantErr: false}, //nolint:gosmopolitan // Intentional Unicode test data for IDN formats.
		{name: "missing at sign", instance: "plainaddress", wantErr: true},
		{name: "missing domain", instance: "user@", wantErr: true},
		{name: "display name present", instance: "Joe <user@example.com>", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := idnEmailFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("idnEmailFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

func TestHostnameFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "simple hostname", instance: "example.com", wantErr: false},
		{name: "single label", instance: "localhost", wantErr: false},
		{name: "hyphenated label", instance: "foo-bar.example.com", wantErr: false},
		{name: "IPv4 address accepted", instance: "127.0.0.1", wantErr: false},
		{name: "IPv6 address accepted", instance: "::1", wantErr: false},
		{name: "underscore", instance: "under_score.com", wantErr: true},
		{name: "non-ASCII", instance: "exämple.com", wantErr: true},
		{name: "leading hyphen", instance: "-example.com", wantErr: true},
		{name: "trailing hyphen in label", instance: "example-.com", wantErr: true},
		{name: "empty label", instance: "example..com", wantErr: true},
		// The JSON-Schema-Test-Suite rejects an empty root label.
		{name: "trailing dot", instance: "example.", wantErr: true},
		{name: "valid A-labels", instance: "xn--9n2bp8q.xn--9t4b11yi5a", wantErr: false},
		{name: "A-label decoding to only ASCII", instance: "xn--example-", wantErr: true},
		{name: "A-label decoding to disallowed rune", instance: "xn--7a", wantErr: true},
		{name: "A-label with middle dot context violation", instance: "xn--l-fda", wantErr: true},
		{name: "A-label with katakana middle dot and no kana or han", instance: "xn--vek", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := hostnameFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("hostnameFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

func TestIdnHostnameFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "ASCII hostname", instance: "example.com", wantErr: false},
		{name: "non-ASCII latin", instance: "exämple.com", wantErr: false},
		{name: "han labels", instance: "实例.测试", wantErr: false},     //nolint:gosmopolitan // Intentional Unicode test data for IDN formats.
		{name: "japanese label", instance: "例え.jp", wantErr: false}, //nolint:gosmopolitan // Intentional Unicode test data for IDN formats.
		{name: "IPv4 address accepted", instance: "127.0.0.1", wantErr: false},
		{name: "underscore", instance: "under_score.com", wantErr: true},
		{name: "disallowed arabic tatweel", instance: "exـample.com", wantErr: true},
		{name: "leading hyphen", instance: "-example.com", wantErr: true},
		// The JSON-Schema-Test-Suite rejects an empty root label,
		// whichever label separator precedes it.
		{name: "trailing dot", instance: "example.", wantErr: true},
		{name: "trailing ideographic full stop", instance: "example。", wantErr: true},
		{name: "ideographic full stop as separator", instance: "a。b", wantErr: false},
		{name: "A-label decoding to only ASCII", instance: "xn--example-", wantErr: true},
		{name: "A-label decoding to disallowed rune", instance: "xn--7a", wantErr: true},
		{name: "katakana middle dot with katakana", instance: "・ァ", wantErr: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := idnHostnameFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("idnHostnameFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestIPv4Format(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "loopback", instance: "127.0.0.1", wantErr: false},
		{name: "max octet", instance: "192.168.0.255", wantErr: false},
		{name: "octet out of range", instance: "256.0.0.1", wantErr: true},
		{name: "too few octets", instance: "1.2.3", wantErr: true},
		{name: "too many octets", instance: "1.2.3.4.5", wantErr: true},
		{name: "leading zeros", instance: "127.000.000.001", wantErr: true},
		{name: "IPv6 address", instance: "::1", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := ipv4Format(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("ipv4Format(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestIPv6Format(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "loopback", instance: "::1", wantErr: false},
		{name: "compressed address", instance: "2001:db8:85a3::8a2e:370:7334", wantErr: false},
		{name: "IPv4-mapped", instance: "::ffff:192.168.0.1", wantErr: false},
		{name: "zone identifier", instance: "fe80::1%eth0", wantErr: true},
		{name: "IPv4 address", instance: "127.0.0.1", wantErr: true},
		{name: "group too long", instance: "12345::", wantErr: true},
		{name: "too many groups", instance: "1:2:3:4:5:6:7:8:9", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := ipv6Format(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("ipv6Format(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestURIFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "https URL", instance: "https://example.com/path", wantErr: false},
		{name: "mailto URI", instance: "mailto:user@example.com", wantErr: false},
		{name: "URL with query and fragment", instance: "https://example.com/path?q=1#frag", wantErr: false},
		{name: "relative reference", instance: "/foo", wantErr: true},
		{name: "missing scheme", instance: "example.com", wantErr: true},
		{name: "non-ASCII in path", instance: "https://example.com/ümlaut", wantErr: true},
		{name: "backslash in fragment", instance: "https://example.com/#\\", wantErr: true},
		{name: "space in path", instance: "https://example.com/a b", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := uriFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("uriFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestURIReferenceFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "absolute URL", instance: "https://example.com/path", wantErr: false},
		{name: "relative reference", instance: "/foo", wantErr: false},
		{name: "fragment only", instance: "#fragment", wantErr: false},
		{name: "empty string", instance: "", wantErr: false},
		{name: "leading double backslash", instance: `\\server\share`, wantErr: true},
		{name: "backslash in fragment", instance: "https://example.com/#\\", wantErr: true},
		{name: "non-ASCII in path", instance: "/ümlaut", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := uriReferenceFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("uriReferenceFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestIRIFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "ASCII URL", instance: "https://example.com/path", wantErr: false},
		{name: "non-ASCII in path", instance: "https://example.com/ümlaut", wantErr: false},
		{name: "relative reference", instance: "/foo", wantErr: true},
		{name: "backslash in fragment", instance: "https://example.com/#\\", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := iriFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("iriFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestIRIReferenceFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "non-ASCII relative reference", instance: "/ümlaut", wantErr: false},
		{name: "absolute IRI", instance: "https://example.com/ümlaut", wantErr: false},
		{name: "fragment only", instance: "#fragment", wantErr: false},
		{name: "leading double backslash", instance: `\\server\share`, wantErr: true},
		{name: "backslash in fragment", instance: "#\\", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := iriReferenceFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("iriReferenceFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestUUIDFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "lowercase v4", instance: "550e8400-e29b-41d4-a716-446655440000", wantErr: false},
		{name: "uppercase hex", instance: "550E8400-E29B-41D4-A716-446655440000", wantErr: false},
		{name: "mixed case hex", instance: "550e8400-E29B-41d4-A716-446655440000", wantErr: false},
		{name: "last group too short", instance: "550e8400-e29b-41d4-a716-44665544000", wantErr: true},
		{name: "trailing extra character", instance: "550e8400-e29b-41d4-a716-4466554400000", wantErr: true},
		{name: "missing dash", instance: "550e8400e29b-41d4-a716-446655440000", wantErr: true},
		{name: "non-hex character", instance: "g50e8400-e29b-41d4-a716-446655440000", wantErr: true},
		{name: "urn prefix", instance: "urn:uuid:550e8400-e29b-41d4-a716-446655440000", wantErr: true},
		{name: "empty string", instance: "", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := uuidFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("uuidFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestJSONPointerFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "empty string", instance: "", wantErr: false},
		{name: "simple pointer", instance: "/foo/bar", wantErr: false},
		{name: "escaped tilde", instance: "/a~0b", wantErr: false},
		{name: "escaped slash", instance: "/a~1b", wantErr: false},
		{name: "single slash", instance: "/", wantErr: false},
		{name: "empty token", instance: "/foo//bar", wantErr: false},
		{name: "missing leading slash", instance: "foo", wantErr: true},
		{name: "slash inside but not leading", instance: "a/b", wantErr: true},
		{name: "invalid escape digit", instance: "/a~2", wantErr: true},
		{name: "trailing bare tilde", instance: "/a~", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := jsonPointerFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("jsonPointerFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestRelativeJSONPointerFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "zero", instance: "0", wantErr: false},
		{name: "number with pointer", instance: "1/foo", wantErr: false},
		{name: "zero with hash", instance: "0#", wantErr: false},
		{name: "multi-digit number with escapes", instance: "10/a~1b", wantErr: false},
		{name: "empty string", instance: "", wantErr: true},
		{name: "leading zero", instance: "01", wantErr: true},
		{name: "hash alone", instance: "#", wantErr: true},
		{name: "hash followed by pointer", instance: "1#/foo", wantErr: true},
		{name: "missing number prefix", instance: "/foo", wantErr: true},
		{name: "negative number", instance: "-1/foo", wantErr: true},
		{name: "invalid escape", instance: "0/a~2", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := relativeJSONPointerFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("relativeJSONPointerFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

//nolint:dupl // Per-format table tests are intentionally parallel in structure.
func TestRegexFormat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "non-string instance", instance: 42, wantErr: false},
		{name: "simple quantifiers", instance: "a+b*", wantErr: false},
		{name: "anchored character class", instance: "^[a-z]+$", wantErr: false},
		{name: "alternation group", instance: "(foo|bar)", wantErr: false},
		{name: "empty pattern", instance: "", wantErr: false},
		{name: "unclosed group", instance: "a(b", wantErr: true},
		{name: "unclosed character class", instance: "[a-", wantErr: true},
		{name: "inverted repeat range", instance: "a{2,1}", wantErr: true},
		{name: "unsupported lookahead", instance: "(?=x)", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := regexFormat(testCase.instance, nil)
			if gotErr := err != nil; gotErr != testCase.wantErr {
				t.Errorf("regexFormat(%#v) error = %v, wantErr = %v", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}
