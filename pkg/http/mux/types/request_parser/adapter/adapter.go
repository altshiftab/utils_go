package adapter

import (
	"net/http"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

type Adapter[T any] struct {
	Parser request_parser.RequestParser[T]
}

func (adapter Adapter[T]) Parse(request *http.Request) (any, *response_error.ResponseError) {
	return adapter.Parser.Parse(request)
}

func New[T any](parser request_parser.RequestParser[T]) Adapter[T] {
	return Adapter[T]{Parser: parser}
}
