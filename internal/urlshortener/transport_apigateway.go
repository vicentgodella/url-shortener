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

	"github.com/afex/hystrix-go/hystrix"
	"github.com/friends-of-scalability/url-shortener/cmd/config"
	endpoint "github.com/go-kit/kit/endpoint"
	sd "github.com/go-kit/kit/sd"

	"github.com/gorilla/mux"

	kitlog "github.com/go-kit/kit/log"
	dnssrv "github.com/go-kit/kit/sd/dnssrv"
	"github.com/go-kit/kit/sd/lb"
	kithttp "github.com/go-kit/kit/transport/http"
)

func MakeAPIGWHandler(ctx context.Context, us Service, logger kitlog.Logger, cfg *config.Config) http.Handler {
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
	hystrix.ConfigureCommand("shortener Request", hystrix.CommandConfig{Timeout: 100000})
	hystrix.ConfigureCommand("resolver Request", hystrix.CommandConfig{Timeout: 1000})
	hystrix.ConfigureCommand("info Request", hystrix.CommandConfig{Timeout: 1000})

	var retry endpoint.Endpoint
	instancer := dnssrv.NewInstancer(cfg.ServiceDiscovery.Resolver, 200*time.Millisecond, kitlog.NewNopLogger())
	factory := endpointFactory(ctx, "resolver", "GET", logger, cfg)
	endpointer := sd.NewEndpointer(instancer, factory, logger)
	balancer := lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	resolverEndpoint := Hystrix("resolver Request",
		"Service currently unavailable", logger)(retry)
	r.Handle("/{shortURL}", kithttp.NewServer(resolverEndpoint, decodeURLRedirectRequest, encodeResponse)).Methods("GET")

	instancer = dnssrv.NewInstancer(cfg.ServiceDiscovery.Resolver, 200*time.Millisecond, kitlog.NewNopLogger())
	factory = endpointFactory(ctx, "info", "GET", logger, cfg)
	endpointer = sd.NewEndpointer(instancer, factory, logger)
	balancer = lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	infoEndpoint := Hystrix("info Request",
		"Service currently unavailable", logger)(retry)
	r.Handle("/info/{shortURL}", kithttp.NewServer(infoEndpoint, decodeURLInfoRequest, encodeResponse)).Methods("GET")

	instancer = dnssrv.NewInstancer(cfg.ServiceDiscovery.Shortener, 200*time.Millisecond, kitlog.NewNopLogger())
	factory = endpointFactory(ctx, "shortener", "POST", logger, cfg)
	endpointer = sd.NewEndpointer(instancer, factory, logger)
	balancer = lb.NewRoundRobin(endpointer)
	retry = lb.Retry(3, 500*time.Millisecond, balancer)
	shortenerEndpoint := Hystrix("shortener Request",
		"Service currently unavailable", logger)(retry)
	r.Handle("/", kithttp.NewServer(shortenerEndpoint, decodeURLShortenerRequest, encodeResponse)).Methods("POST")

	return r
}

func endpointFactory(ctx context.Context, action, method string, logger kitlog.Logger, cfg *config.Config) sd.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		s := strings.Split(instance, ":")
		// removing port since service discovery is getting a wrong one
		if len(s) == 0 || len(s) > 2 {
			return nil, nil, fmt.Errorf("Got wrong address from service discovery, something went wrong")
		}
		instance = s[0]
		if !strings.HasPrefix(instance, "http") {
			instance = "http://" + instance + ":" + cfg.ExposedPort
		}
		tgt, err := url.Parse(instance)

		if err != nil {
			return nil, nil, err
		}
		//logger.Log("TYPE", "apigateway request", "PATH", tgt.String(), "METHOD", method, "ACTION", action)

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
	log.Printf("%q\n", dump)
}
func debugResponse(resp *http.Response) {
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%q", dump)
}

func encodeHTTPGenericRequest(_ context.Context, r *http.Request, request interface{}) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(&buf)
	return nil
}
