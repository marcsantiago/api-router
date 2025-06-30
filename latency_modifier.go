package router

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	defaultClient = &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   1000 * time.Millisecond,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 500 * time.Millisecond,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     10 * time.Second,
		},
		Timeout: 1000 * time.Millisecond,
	}
	defaultPingInterval = 1 * time.Hour
)

type LatencyCheckModifier struct {
	// if a client is not passed in as an optional, the default network client will be used
	Client *http.Client
	// EndPoints is passed through as a reference from Router
	EndPoints *EndPoints
	// if DebugMode is set, logs from the standard log package will be displayed
	DebugMode bool
	// if PingInterval is not set as optional, endpoints will not be checked for latency periodically
	PingInterval time.Duration

	mu         sync.RWMutex
	fastestURL string // is the fastest endpoint based on a head request
	stopTicker chan struct{}
}

func NewLatencyCheckerModifier(endpoints *EndPoints, options ...func(*LatencyCheckModifier)) *LatencyCheckModifier {
	l := &LatencyCheckModifier{
		Client:       defaultClient,
		PingInterval: defaultPingInterval,
		DebugMode:    false,
		EndPoints:    endpoints,
		mu:           sync.RWMutex{},
		stopTicker:   make(chan struct{}, 1),
	}

	if len(endpoints.ClosestURL) != 0 {
		l.fastestURL = endpoints.ClosestURL
	} else if len(endpoints.Universal) != 0 {
		l.fastestURL = endpoints.Universal
	} else if len(endpoints.Fallback) != 0 {
		l.fastestURL = endpoints.Fallback
	}

	for _, option := range options {
		option(l)
	}
	// starts a long-lived goroutine
	l.periodicallyPingEndpoints()
	return l
}

// GetEndpoint returns the fastestURL
//
// defaults to the closestURL from the default router and changes based on latency checks
func (l *LatencyCheckModifier) GetEndpoint() (endpoint string) {
	l.mu.RLock()
	endpoint = l.fastestURL
	l.mu.RUnlock()
	return
}

// StopPingingEndpoints terminates the ticker used to periodically check endpoints for latency and status
// it's important this function is called to clean up ticker resources
func (l *LatencyCheckModifier) StopPingingEndpoints() {
	select {
	case l.stopTicker <- struct{}{}:
	default:
	}
}

func WithCustomClient(client *http.Client) func(*LatencyCheckModifier) {
	return func(l *LatencyCheckModifier) {
		l.Client = client
	}
}

func WithCustomPingInterval(interval time.Duration) func(*LatencyCheckModifier) {
	return func(l *LatencyCheckModifier) {
		l.PingInterval = interval
	}
}

func WithDebugMode(debug bool) func(*LatencyCheckModifier) {
	return func(l *LatencyCheckModifier) {
		l.DebugMode = debug
	}
}

type latencyResult struct {
	URL      string
	Duration time.Duration
}

func (l *LatencyCheckModifier) findLowLatencyEndpoint() {
	ctx, cancel := context.WithTimeout(context.Background(), l.Client.Timeout)
	defer cancel()

	endpoints := []string{
		l.EndPoints.Universal,
		l.EndPoints.USEast,
		l.EndPoints.USWest,
		l.EndPoints.Europe,
		l.EndPoints.AsiaPacific,
	}

	results := make(chan latencyResult, len(endpoints))
	var wg sync.WaitGroup
	for i := range endpoints {
		wg.Add(1)
		go l.headRequest(ctx, &wg, endpoints[i], results)
	}
	wg.Wait()
	close(results)

	l.mu.Lock()
	defer l.mu.Unlock()
	fastest := latencyResult{URL: "", Duration: time.Hour}
	for res := range results {
		if res.Duration < fastest.Duration {
			fastest = res
		}
	}

	if len(fastest.URL) == 0 {
		return
	}
	l.fastestURL = fastest.URL
	return
}

func (l *LatencyCheckModifier) headRequest(ctx context.Context, wg *sync.WaitGroup, endpoint string, results chan latencyResult) {
	defer wg.Done()

	if len(endpoint) == 0 {
		results <- latencyResult{endpoint, time.Hour}
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
	if err != nil {
		return
	}

	start := time.Now()
	res, err := l.Client.Do(req)
	if err != nil {
		results <- latencyResult{endpoint, time.Hour}
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		results <- latencyResult{endpoint, time.Hour}
		return
	}

	duration := time.Since(start)
	results <- latencyResult{endpoint, duration}
	return
}

func (l *LatencyCheckModifier) log(v ...interface{}) {
	if l.DebugMode {
		log.Println(v...)
	}
}

func (l *LatencyCheckModifier) logf(format string, v ...interface{}) {
	if l.DebugMode {
		log.Printf(format, v...)
	}
}

func (l *LatencyCheckModifier) periodicallyPingEndpoints() {
	l.findLowLatencyEndpoint()
	ticker := time.NewTicker(l.PingInterval)
	go func() {
		for {
			select {
			case <-l.stopTicker:
				ticker.Stop()
				close(l.stopTicker)
				return
			case <-ticker.C:
				l.findLowLatencyEndpoint()
			}
		}
	}()
}
