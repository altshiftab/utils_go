package retry_after

import (
	"errors"
	"testing"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestRetryAfterBadInput(t *testing.T) {
	t.Parallel()

	data := []byte("so bad")

	retryAfter, err := Parse(data)
	if retryAfter != nil && errors.Is(err, altshiftErrors.ErrSyntaxError) {
		t.Error("expected nil retry after and syntax error")
	}
}

func TestRetryAfterHttpDate(t *testing.T) {
	t.Parallel()

	data := []byte("Fri, 31 Dec 1999 23:59:59 GMT")

	retryAfter, err := Parse(data)
	if err != nil {
		t.Fatalf("an error occurred when parsing the data: %v", err)
	}
	if retryAfter == nil {
		t.Fatal("retry after is nil")
	}

	httpDate, ok := retryAfter.WaitTime.(time.Time)
	if !ok {
		t.Fatal("the wait time could not be converted to a time.Time")
	}

	if httpDate.Unix() != 946684799 {
		t.Fatal("the http date was not parsed as the correct time")
	}
}

func TestRetryAfterDelay(t *testing.T) {
	t.Parallel()

	data := []byte("120")

	retryAfter, err := Parse(data)
	if err != nil {
		t.Fatalf("an error occurred when parsing the data: %v", err)
	}
	if retryAfter == nil {
		t.Fatal("retry after is nil")
	}

	delayDuration, ok := retryAfter.WaitTime.(time.Duration)
	if !ok {
		t.Fatal("the wait time could not be converted to int64")
	}

	expectedDuration := 120 * time.Second

	if delayDuration != expectedDuration {
		t.Fatal("the delay seconds was not parsed as the correct duration")
	}
}
