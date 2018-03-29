package urlshortener

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	kitlog "github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
)

func MakeShortenerHandler(ctx context.Context, us Service, logger kitlog.Logger) http.Handler {
	r := mux.NewRouter()

	opts := []kithttp.ServerOption{
		kithttp.ServerErrorLogger(logger),
		kithttp.ServerErrorEncoder(encodeError),
		kithttp.ServerBefore(kithttp.PopulateRequestContext, func(c context.Context, r *http.Request) context.Context {
			var scheme = "http"
			if r.TLS != nil {
				scheme = "https"
			}
			c = context.WithValue(c, contextKeyHTTPAddress, scheme+"://"+r.Host+"/")
			return c
		}),
	}

	URLHealthzHandler := kithttp.NewServer(
		makeURLHealthzEndpoint(us),
		func(c context.Context, r *http.Request) (interface{}, error) {
			return nil, nil
		},
		encodeResponse,
		opts...,
	)
	URLShortifyHandler := kithttp.NewServer(
		makeURLShortifyEndpoint(us),
		decodeURLShortenerRequest,
		encodeResponse,
		opts...,
	)

	r.Handle("/", URLShortifyHandler).Methods("POST")
	r.Handle("/healthz", URLHealthzHandler).Methods("GET")
	return r
}

func decodeURLShortenerResponse(ctx context.Context, resp *http.Response) (interface{}, error) {
	var response shortenerResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response, nil
}
func decodeURLShortenerRequest(c context.Context, r *http.Request) (interface{}, error) {
	decoder := json.NewDecoder(r.Body)
	var t shortURL
	if !decoder.More() {
		return nil, errors.New("Empty request, cannot shortify the emptiness")

	}
	err := decoder.Decode(&t)
	if err != nil {
		return nil, err
	}
	if t.URL == "" {
		return nil, errors.New("Empty request, cannot shortify the emptiness")
	}
	return shortenerRequest{URL: t.URL}, nil
}
