package adapter

import (
	"net/http"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

type Adapter[T any] struct {
	Parser body_parser.BodyParser[T]
}

func (adapter Adapter[T]) Parse(request *http.Request, body []byte) (any, *response_error.ResponseError) {
	return adapter.Parser.Parse(request, body)
}

func New[T any](parser body_parser.BodyParser[T]) Adapter[T] {
	return Adapter[T]{Parser: parser}
}
