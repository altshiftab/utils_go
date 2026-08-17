package initialization_endpoint

import "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"

type Endpoint struct {
	*endpoint.Endpoint
	Initialized bool
}
