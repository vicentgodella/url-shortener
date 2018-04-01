package urlshortener

import (
	"time"

	"github.com/go-kit/kit/metrics"
)

func NewMetricsService(requestCount metrics.Counter,
	requestLatency metrics.Histogram, s Service) Service {
	return &metricsMiddleware{
		s,
		requestCount,
		requestLatency,
	}

}

type metricsMiddleware struct {
	Service
	requestCount   metrics.Counter
	requestLatency metrics.Histogram
}

func (mw *metricsMiddleware) Shortify(longURL string) (mapping *shortURL, err error) {
	defer func(begin time.Time) {
		lvs := []string{"method", "Shortify"}
		mw.requestCount.With(lvs...).Add(1)
		mw.requestLatency.With(lvs...).Observe(time.Since(begin).Seconds())
	}(time.Now())
	return mw.Service.Shortify(longURL)
}

func (mw *metricsMiddleware) Resolve(shortURL string) (mapping *shortURL, err error) {
	defer func(begin time.Time) {
		lvs := []string{"method", "Resolve"}
		mw.requestCount.With(lvs...).Add(1)
		mw.requestLatency.With(lvs...).Observe(time.Since(begin).Seconds())
	}(time.Now())
	return mw.Service.Resolve(shortURL)
}

func (mw *metricsMiddleware) GetInfo(shortURL string) (mapping *shortURL, err error) {
	defer func(begin time.Time) {
		lvs := []string{"method", "Info"}
		mw.requestCount.With(lvs...).Add(1)
		mw.requestLatency.With(lvs...).Observe(time.Since(begin).Seconds())
	}(time.Now())
	return mw.Service.GetInfo(shortURL)
}
