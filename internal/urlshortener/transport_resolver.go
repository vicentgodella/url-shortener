package urlshortener

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	kitlog "github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
)

func MakeResolverHandler(ctx context.Context, us Service, logger kitlog.Logger) http.Handler {
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
	URLRedirectHandler := kithttp.NewServer(
		makeURLRedirectEndpoint(us),
		decodeURLRedirectRequest,
		encodeRedirectResponse,
		opts...,
	)
	URLInfoHandler := kithttp.NewServer(
		makeURLInfoEndpoint(us),
		decodeURLInfoRequest,
		encodeResponse,
		opts...,
	)

	r.Handle("/healthz", URLHealthzHandler).Methods("GET")
	r.Handle("/{shortURL}", URLRedirectHandler).Methods("GET")
	r.Handle("/info/{shortURL}", URLInfoHandler).Methods("GET")

	return r
}

func encodeRedirectResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}
	if e, ok := response.(redirectResponse); ok && e.error() == nil {
		return json.NewEncoder(w).Encode(e)
	}
	return nil

}

func decodeURLInfoResponse(ctx context.Context, resp *http.Response) (interface{}, error) {
	var response infoResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response, nil
}
func decodeURLRedirectRequest(c context.Context, r *http.Request) (interface{}, error) {

	shURL := mux.Vars(r)
	if val, ok := shURL["shortURL"]; ok {
		//do something here
		return redirectRequest{id: val}, nil
	}
	return nil, errMalformedURL

}
func decodeURLInfoRequest(c context.Context, r *http.Request) (interface{}, error) {
	shURL := mux.Vars(r)
	if val, ok := shURL["shortURL"]; ok {
		//do something here
		return infoRequest{id: val}, nil
	}
	return nil, errMalformedURL
}
