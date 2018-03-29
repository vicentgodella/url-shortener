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
	instancer := dnssrv.NewInstancer("example-url-shortener-resolver.default.svc.cluster.local", 30*time.Millisecond, logger)
	factory := resolverFactory(ctx, "resolver", "GET", logger)
	endpointer := sd.NewEndpointer(instancer, factory, logger)
	balancer := lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	r.Handle("/{shortURL}", kithttp.NewServer(retry, decodeURLRedirectRequest, encodeRedirectResponse)).Methods("GET")

	// instancer = dnssrv.NewInstancer("example-url-shortener-resolver.default.svc.cluster.local", 30*time.Millisecond, logger)
	// factory = resolverFactory(ctx, "info", "GET", logger)
	// endpointer = sd.NewEndpointer(instancer, factory, logger)
	// balancer = lb.NewRoundRobin(endpointer)
	// retry = lb.Retry(3, 500*time.Millisecond, balancer)
	// r.Handle("/info/{shortURL}", kithttp.NewServer(retry, decodeURLInfoRequest, encodeResponse)).Methods("GET")

	// instancer = dnssrv.NewInstancer("example-url-shortener-shortener.default.svc.cluster.local", 30*time.Millisecond, logger)
	// factory = shortenerFactory(ctx, "POST", "/")
	// endpointer = sd.NewEndpointer(instancer, factory, logger)
	// balancer = lb.NewRoundRobin(endpointer)
	// retry = lb.Retry(3, 500*time.Millisecond, balancer)
	// r.Handle("/", kithttp.NewServer(retry, decodeURLShortenerRequest, encodeResponse)).Methods("POST")

	return r
}

func resolverFactory(ctx context.Context, action, method string, logger kitlog.Logger) sd.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		s := strings.Split(instance, ":")
		// removing port since service discovery is getting a wrong one
		instance = s[0]
		if !strings.HasPrefix(instance, "http") {
			instance = "http://" + instance + ":8080"
		}
		tgt, err := url.Parse(instance)

		if err != nil {
			return nil, nil, err
		}
		logger.Log("transport", "HTTP", "apigateway request", tgt.String(), " method ", method)

		var (
			enc kithttp.EncodeRequestFunc
			dec kithttp.DecodeResponseFunc
		)
		switch action {
		case "resolver":
			enc, dec = encodeAPIGWRedirectRequest, decodeAPIGWRedirectResponse
		case "info":
			enc, dec = encodeHTTPGenericRequest, decodeURLInfoResponse
		default:
			return nil, nil, fmt.Errorf("unknown resolver action %q", action)
		}
		return kithttp.NewClient(method, tgt, enc, dec).Endpoint(), nil, nil
	}
}

func shortenerFactory(ctx context.Context, method, path string) sd.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		s := strings.Split(instance, ":")
		// removing port since service discovery is getting a wrong one
		instance = s[0]
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
			enc, dec = encodeHTTPShortenerRequest, decodeURLShortenerResponse
		default:
			return nil, nil, fmt.Errorf("unknown shortener path %q", path)
		}

		return kithttp.NewClient(method, tgt, enc, dec).Endpoint(), nil, nil
	}
}

func encodeAPIGWRedirectRequest(ctx context.Context, req *http.Request, request interface{}) error {

	originalRequest := request.(redirectRequest)
	url := url.URL{
		Scheme:  req.URL.Scheme,
		Host:    req.URL.Host,
		Path:    "/" + originalRequest.id,
		RawPath: "/" + url.QueryEscape(originalRequest.id),
	}
	req.URL = &url
	dump, err := httputil.DumpRequest(req, true)
	if err != nil {
		return err
	}
	fmt.Printf("%q\n", dump)
	return nil

}

func decodeAPIGWRedirectResponse(ctx context.Context, resp *http.Response) (interface{}, error) {
	fmt.Printf("REDIRECT RESPONSE!!!\n")
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%q", dump)
	var response redirectResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response, nil
}

func encodeHTTPInfoRequest(_ context.Context, req *http.Request, request interface{}) error {
	originalRequest := request.(infoRequest)
	fmt.Printf("Calling %s with method %s  and request %+v\n", req.URL, req.Method, originalRequest)
	return nil

}

func encodeHTTPShortenerRequest(_ context.Context, req *http.Request, request interface{}) error {
	originalRequest := request.(shortenerRequest)
	fmt.Printf("Calling %s with method %s  and request %+v\n", req.URL, req.Method, originalRequest)
	return nil

}

func encodeHTTPGenericRequest(_ context.Context, r *http.Request, request interface{}) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(&buf)
	return nil
}
