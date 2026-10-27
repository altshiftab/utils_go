package hmac

import "fmt"

// GoString writes the method out without its secret, and String and MarshalText write the same.
//
// A method is attached to errors and so to log entries, and every way a value is written out would
// otherwise write the secret along with it: %#v and %v print its bytes, a JSON encoder its base64.
func (method *Method) GoString() string {
	if method == nil {
		return "(*hmac.Method)(nil)"
	}

	secret := "<nil>"
	if len(method.Secret) != 0 {
		secret = "<redacted>"
	}

	return fmt.Sprintf("&hmac.Method{Name:%q, Secret:%s}", method.Name, secret)
}

func (method *Method) String() string {
	return method.GoString()
}

func (method *Method) MarshalText() ([]byte, error) {
	return []byte(method.GoString()), nil
}
