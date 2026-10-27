package jws_object

import (
	"fmt"

	"github.com/altshiftab/utils_go/pkg/json/jose/jws"
)

// GoString writes the object out as its fingerprint alone, and String and MarshalText write the same.
//
// An object is a token, and was attached to errors as one. %#v writes its signature and payload out
// as byte lists, which the log's masking of token-shaped text cannot see, so the object is written
// out as a name for the token rather than the token.
func (o *Object) GoString() string {
	if o == nil {
		return "(*jws_object.Object)(nil)"
	}
	return fmt.Sprintf("&jws_object.Object{Fingerprint:%q}", jws.Fingerprint(o.Raw))
}

func (o *Object) String() string {
	return o.GoString()
}

func (o *Object) MarshalText() ([]byte, error) {
	return []byte(o.GoString()), nil
}
