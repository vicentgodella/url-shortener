package urlshortener

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
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
	r.Handle("/healthz", URLHealthzHandler).Methods("GET")

	var retry endpoint.Endpoint
	instancer := dnssrv.NewInstancer("example-url-shortener-resolver.default.svc.cluster.local", 200*time.Millisecond, logger)
	factory := endpointFactory(ctx, "resolver", "GET", logger)
	endpointer := sd.NewEndpointer(instancer, factory, logger)
	balancer := lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	r.Handle("/{shortURL}", kithttp.NewServer(retry, decodeURLRedirectRequest, encodeRedirectResponse)).Methods("GET")

	instancer = dnssrv.NewInstancer("example-url-shortener-resolver.default.svc.cluster.local", 200*time.Millisecond, logger)
	factory = endpointFactory(ctx, "info", "GET", logger)
	endpointer = sd.NewEndpointer(instancer, factory, logger)
	balancer = lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	r.Handle("/info/{shortURL}", kithttp.NewServer(retry, decodeURLInfoRequest, encodeResponse)).Methods("GET")

	instancer = dnssrv.NewInstancer("example-url-shortener-shortener.default.svc.cluster.local", 200*time.Millisecond, logger)
	factory = endpointFactory(ctx, "shortener", "POST", logger)
	endpointer = sd.NewEndpointer(instancer, factory, logger)
	balancer = lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	r.Handle("/", kithttp.NewServer(retry, decodeURLShortenerRequest, encodeResponse)).Methods("POST")

	return r
}

func endpointFactory(ctx context.Context, action, method string, logger kitlog.Logger) sd.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		s := strings.Split(instance, ":")
		// removing port since service discovery is getting a wrong one
		if len(s) == 0 || len(s) > 2 {
			return nil, nil, fmt.Errorf("Got wrong address from service discovery, something went wrong")
		}
		instance = s[0]
		if !strings.HasPrefix(instance, "http") {
			instance = "http://" + instance + ":8080"
		}
		tgt, err := url.Parse(instance)

		if err != nil {
			return nil, nil, err
		}
		logger.Log("TYPE", "apigateway request", "PATH", tgt.String(), "METHOD", method, "ACTION", action)

		var (
			enc kithttp.EncodeRequestFunc
			dec kithttp.DecodeResponseFunc
		)
		switch action {
		case "resolver":
			enc, dec = encodeAPIGWRedirectRequest, decodeAPIGWRedirectResponse
		case "info":
			tgt.Path = "/info"
			enc, dec = encodeAPIGWInfoRequest, decodeURLInfoResponse
		case "shortener":
			enc, dec = encodeHTTPGenericRequest, decodeURLShortenerResponse
		default:
			return nil, nil, fmt.Errorf("unknown resolver action %q", action)
		}
		kithttp.ClientBefore(func(ctx context.Context, resp *http.Request) context.Context {
			logger.Log("TYPE", "HTTP CLIENT", "PATH", tgt.String(), "METHOD", method, "ACTION", action)
			return ctx
		})
		return kithttp.NewClient(method, tgt, enc, dec).Endpoint(), nil, nil
	}
}

func encodeAPIGWRedirectRequest(ctx context.Context, req *http.Request, request interface{}) error {

	originalRequest, ok := request.(redirectRequest)
	if !ok {
		return fmt.Errorf("Cannot cast request to an redirectRequest")
	}
	url := url.URL{
		Scheme:  req.URL.Scheme,
		Host:    req.URL.Host,
		Path:    "/" + originalRequest.id,
		RawPath: "/" + url.QueryEscape(originalRequest.id),
	}
	req.URL = &url

	return nil

}
func encodeAPIGWInfoRequest(ctx context.Context, req *http.Request, request interface{}) error {
	originalRequest, ok := request.(infoRequest)
	if !ok {
		return fmt.Errorf("Cannot cast request to an inforequest")
	}
	url := url.URL{
		Scheme:  req.URL.Scheme,
		Host:    req.URL.Host,
		Path:    "/info/" + originalRequest.id,
		RawPath: "/info/" + url.QueryEscape(originalRequest.id),
	}
	req.URL = &url

	return nil

}
func decodeAPIGWRedirectResponse(ctx context.Context, resp *http.Response) (interface{}, error) {

	var response redirectResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response, nil
}
func debugRequest(req *http.Request) {
	dump, err := httputil.DumpRequest(req, true)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%q\n", dump)
}
func debugResponse(resp *http.Response) {
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%q", dump)
}

func encodeHTTPGenericRequest(_ context.Context, r *http.Request, request interface{}) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(&buf)
	return nil
}
