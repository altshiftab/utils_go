package about

import "github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/user"

// About is what Drive says of the user a request is made as.
type About struct {
	User *user.User `json:"user,omitzero"`
}
