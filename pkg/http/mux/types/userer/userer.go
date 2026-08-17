package userer

import (
	"github.com/altshiftab/utils_go/pkg/schema"
)

type Userer interface {
	GetUser() *schema.User
}

type Function func() *schema.User

func (f Function) GetUser() *schema.User {
	return f()
}

func New(f func() *schema.User) Userer {
	return Function(f)
}

// TODO: In future, add something for getting client metadata (IP enrichment; geo, tags e.g.)
