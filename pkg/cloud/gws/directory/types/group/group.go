package group

import (
	"encoding/json/jsontext"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/force_send"
)

type Group struct {
	Kind string `json:"kind,omitempty"`
	Id   string `json:"id,omitempty"`
	Etag string `json:"etag,omitempty"`

	Email       string `json:"email,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	DirectMembersCount string `json:"directMembersCount,omitempty"`
	AdminCreated       bool   `json:"adminCreated,omitempty"`

	Aliases            []string `json:"aliases,omitempty"`
	NonEditableAliases []string `json:"nonEditableAliases,omitempty"`

	// ForceSendFields names, by their JSON names, the fields a request sends even where they are
	// empty: left out, the API keeps the value it had, so sending one empty is how it is cleared.
	ForceSendFields []string `json:"-"`
}

// MarshalJSONTo writes the group with the fields ForceSendFields names even where they are empty.
func (g *Group) MarshalJSONTo(encoder *jsontext.Encoder) error {
	type plain Group
	encoded, err := force_send.Marshal((*plain)(g), g.ForceSendFields)
	if err != nil {
		return err
	}

	return encoder.WriteValue(encoded)
}
