package urlshortener

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	endpoint "github.com/go-kit/kit/endpoint"
	sd "github.com/go-kit/kit/sd"

	"github.com/gorilla/mux"

	kitlog "github.com/go-kit/kit/log"
	dnssrv "github.com/go-kit/kit/sd/dnssrv"
	"github.com/go-kit/kit/sd/lb"
	kithttp "github.com/go-kit/kit/transport/http"
)

// MakeHandler returns a handler for the urlshortener service.
func MakeHandler(ctx context.Context, us Service, logger kitlog.Logger) http.Handler {
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

	r.Handle("/", URLShortifyHandler).Methods("POST")
	r.Handle("/healthz", URLHealthzHandler).Methods("GET")
	r.Handle("/{shortURL}", URLRedirectHandler).Methods("GET")
	r.Handle("/info/{shortURL}", URLInfoHandler).Methods("GET")

	return r
}

func MakeAPIGWHandler(ctx context.Context, us Service, logger kitlog.Logger) http.Handler {
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

	var retry endpoint.Endpoint
	instancer := dnssrv.NewInstancer("example-url-shortener-resolver.default.svc.cluster.local", 30*time.Millisecond, logger)
	factory := resolverFactory(ctx, "GET", "/{shortURL}")
	endpointer := sd.NewEndpointer(instancer, factory, logger)
	balancer := lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	r.Handle("/{shortURL}", kithttp.NewServer(retry, decodeURLRedirectRequest, encodeRedirectResponse)).Methods("GET")

	instancer = dnssrv.NewInstancer("example-url-shortener-resolver.default.svc.cluster.local", 30*time.Millisecond, logger)
	factory = resolverFactory(ctx, "GET", "/info/{shortURL}")
	endpointer = sd.NewEndpointer(instancer, factory, logger)
	balancer = lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	r.Handle("/info/{shortURL}", kithttp.NewServer(retry, decodeURLInfoRequest, encodeResponse)).Methods("GET")

	instancer = dnssrv.NewInstancer("example-url-shortener-shortener.default.svc.cluster.local", 30*time.Millisecond, logger)
	factory = shortenerFactory(ctx, "POST", "/")
	endpointer = sd.NewEndpointer(instancer, factory, logger)
	balancer = lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	r.Handle("/", kithttp.NewServer(retry, decodeURLShortenerRequest, encodeResponse)).Methods("POST")
	r.Handle("/healthz", URLHealthzHandler).Methods("GET")

	return r
}

func resolverFactory(ctx context.Context, method, path string) sd.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		if !strings.HasPrefix(instance, "http") {
			instance = "http://" + instance + ":8080"
		}
		tgt, err := url.Parse(instance)
		if err != nil {
			return nil, nil, err
		}
		tgt.Path = path

		// Since stringsvc doesn't have any kind of package we can import, or
		// any formal spec, we are forced to just assert where the endpoints
		// live, and write our own code to encode and decode requests and
		// responses. Ideally, if you write the service, you will want to
		// provide stronger guarantees to your clients.

		var (
			enc kithttp.EncodeRequestFunc
			dec kithttp.DecodeResponseFunc
		)
		switch path {
		case "/{shortURL}":
			enc, dec = encodeJSONRequest, decodeRedirectResponse
		case "/info/{shortURL}":
			enc, dec = encodeJSONRequest, decodeURLInfoResponse
		default:
			return nil, nil, fmt.Errorf("unknown resolver path %q", path)
		}

		return kithttp.NewClient(method, tgt, enc, dec).Endpoint(), nil, nil
	}
}

func shortenerFactory(ctx context.Context, method, path string) sd.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		if !strings.HasPrefix(instance, "http") {
			instance = "http://" + instance + ":8080"
		}
		tgt, err := url.Parse(instance)
		if err != nil {
			return nil, nil, err
		}
		tgt.Path = path

		// Since stringsvc doesn't have any kind of package we can import, or
		// any formal spec, we are forced to just assert where the endpoints
		// live, and write our own code to encode and decode requests and
		// responses. Ideally, if you write the service, you will want to
		// provide stronger guarantees to your clients.

		var (
			enc kithttp.EncodeRequestFunc
			dec kithttp.DecodeResponseFunc
		)
		switch path {
		case "/":
			enc, dec = encodeJSONRequest, decodeURLShortenerResponse
		default:
			return nil, nil, fmt.Errorf("unknown shortener path %q", path)
		}

		return kithttp.NewClient(method, tgt, enc, dec).Endpoint(), nil, nil
	}
}

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

func decodeURLRedirectRequest(c context.Context, r *http.Request) (interface{}, error) {
	shURL := mux.Vars(r)
	return redirectRequest{id: shURL["shortURL"]}, nil
}

func decodeURLInfoRequest(c context.Context, r *http.Request) (interface{}, error) {
	shURL := mux.Vars(r)
	return infoRequest{id: shURL["shortURL"]}, nil

}

func encodeRedirectResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}
	if e, ok := response.(redirectResponse); ok && e.error() == nil {
		w.Header().Set("Location", e.URL)
		w.Header().Set("Referer", e.id)
		w.WriteHeader(http.StatusPermanentRedirect)
		return nil
	}
	encodeError(ctx, errMalformedURL, w)
	return nil
}

func decodeRedirectResponse(ctx context.Context, resp *http.Response) (interface{}, error) {
	var response redirectResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response, nil
}

func decodeURLShortenerResponse(ctx context.Context, resp *http.Response) (interface{}, error) {
	var response shortenerResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response, nil
}

func decodeURLInfoResponse(ctx context.Context, resp *http.Response) (interface{}, error) {
	var response infoResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response, nil
}

func encodeJSONRequest(_ context.Context, req *http.Request, request interface{}) error {
	// Both uppercase and count requests are encoded in the same way:
	// simple JSON serialization to the request body.
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return err
	}
	req.Body = ioutil.NopCloser(&buf)
	return nil
}

func encodeResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}

type errorer interface {
	error() error
}

// encode errors from business-logic
func encodeError(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch err {
	case errURLNotFound:
		w.WriteHeader(http.StatusNotFound)
	case errMalformedURL:
		w.WriteHeader(http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": err.Error(),
	})
}
