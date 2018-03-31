package urlshortener

import (
	"context"

	"github.com/afex/hystrix-go/hystrix"
	endpoint "github.com/go-kit/kit/endpoint"
	kitlog "github.com/go-kit/kit/log"
)

func Hystrix(commandName string, fallbackMesg string, logger kitlog.Logger) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request interface{}) (response interface{}, err error) {
			var resp interface{}
			if err := hystrix.Do(commandName, func() (err error) {
				resp, err = next(ctx, request)
				return err
			}, func(err error) error {
				logger.Log("fallbackErrorDesc", err.Error(), "RETURN", fallbackMesg)
				resp = struct {
					Fallback string `json:"fallback"`
				}{
					fallbackMesg,
				}
				return nil
			}); err != nil {
				return nil, err
			}
			return resp, nil
		}
	}
}
