package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	bindAddress := os.Getenv("BIND_ADDRESS")
	urls := strings.Split(os.Getenv("URLS"), ",")
	steps := strings.Split(os.Getenv("STEPS"), ",")
	qrp, err := newQueryRangeProxy(steps, urls)
	if err != nil {
		panic(err)
	}
	http.Handle("/api/v1/queryRange", qrp)
	http.Handle("/api/", qrp.Prometheus[0].Proxy)
	http.Handle("/metrics", prometheus.Handler())
	panic(http.ListenAndServe(bindAddress, nil))
}

type QueryRangeProxy struct {
	Prometheus []Prometheus
}

func (qrp *QueryRangeProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	step, err := parseDuration(req.FormValue("step"))
	if err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, "Invalid 'step': %s", err.Error())
		return
	}
	var lastStep time.Duration
	for _, prometheus := range qrp.Prometheus {
		if step < prometheus.MaxStep {
			prometheus.Proxy.ServeHTTP(w, req)
			return
		}
		lastStep = prometheus.MaxStep
	}
	w.WriteHeader(400)
	fmt.Fprintf(w, "Step too large to handle: %v > %v", step, lastStep)
}

func newQueryRangeProxy(steps, urls []string) (*QueryRangeProxy, error) {
	var proxy QueryRangeProxy
	if len(steps) != len(urls) {
		return nil, fmt.Errorf("different number of urls (%d) and steps (%d)", len(urls), len(steps))
	}
	var lastStep time.Duration
	for i := range steps {
		prometheus, err := newPrometheus(steps[i], urls[i])
		if err != nil {
			return nil, err
		}
		if prometheus.MaxStep < lastStep {
			return nil, fmt.Errorf("step is smaller than previous step. (%v < %v)", prometheus.MaxStep, lastStep)
		}
		proxy.Prometheus = append(proxy.Prometheus, *prometheus)
	}
	return &proxy, nil
}

type Prometheus struct {
	MaxStep time.Duration
	Proxy   http.Handler
}

func newPrometheus(maxStep, urlStr string) (*Prometheus, error) {
	maxStepDuration, err := parseDuration(maxStep)
	if err != nil {
		return nil, err
	}
	proxyURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	return &Prometheus{
		MaxStep: maxStepDuration,
		Proxy:   httputil.NewSingleHostReverseProxy(proxyURL),
	}, nil
}

// Copied from https://github.com/prometheus/prometheus/web/api/v1/api.go
func parseDuration(s string) (time.Duration, error) {
	if d, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(d * float64(time.Second)), nil
	}
	if d, err := model.ParseDuration(s); err == nil {
		return time.Duration(d), nil
	}
	return 0, fmt.Errorf("cannot parse %q to a valid duration", s)
}
