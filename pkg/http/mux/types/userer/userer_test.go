package userer

import (
	"testing"

	"github.com/altshiftab/utils_go/pkg/schema"
)

func TestNew(t *testing.T) {
	t.Parallel()

	user := &schema.User{Id: "u1", Name: "alice"}
	if got := New(func() *schema.User { return user }).GetUser(); got != user {
		t.Fatalf("GetUser() = %#v, want %#v", got, user)
	}
}
