package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	// defaultClient provides a network client with the timeout set to 2 seconds and 0 keep-alive
	defaultClient = &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   2000 * time.Millisecond,
				KeepAlive: 0,
			}).DialContext,
			TLSHandshakeTimeout: 2000 * time.Millisecond,
		},
		Timeout: 2000 * time.Millisecond,
	}
)

var (
	// ErrAtLeastOne at least one field of EndPoints needs to initialize
	ErrAtLeastOne = errors.New("at least one endpoint has to be passed in")
	// ErrBadStatus notifies the user that the status code is not a 200
	ErrBadStatus = errors.New("received a non 200 status code")
	// ErrFallbackUnset notifies that the fallback should be sent, even if it's a duplicative endpoint
	ErrFallbackUnset = errors.New("a fallback endpoint should be sent as a safety mechanism")
	// ErrMissingProtocol a protocol must be present with each endpoint
	ErrMissingProtocol = errors.New("missing http or https")
	// ErrTimeout indicates a network timeout
	ErrTimeout = errors.New("the network request timed out")
	// ErrConnectionReset represents a connection was reset during a network call
	ErrConnectionReset = errors.New("the connection was reset by host")
	// ErrNoSuchHost the host could not be found on the endpoint
	ErrNoSuchHost = errors.New("the endpoint's host could not be found")
)

// EndPoints belonging the API service that is being used
type EndPoints struct {
	AsiaPacific string `json:"asia_pacific,omitempty"` // APAC
	Europe      string `json:"europe,omitempty"`       // EU
	Universal   string `json:"universal,omitempty"`    // Some APIs contain a single endpoint, which is a latency load balanced by the DNS and load balancer
	USEast      string `json:"us_east,omitempty"`      // us-east-1
	USWest      string `json:"us_west,omitempty"`      // us-west-1
	Fallback    string `json:"fallback,omitempty"`     // provides an optional endpoint to fall back to in emergencies
	FastestURL  string `json:"fastest_url,omitempty"`  // is the fastest endpoint based on a head request
}

// normally, reflection should be avoided because it's very slow,
// however, because this method is called once at initialization, this should be okay
func (e EndPoints) validate() error {
	var atLeastOne int
	v := reflect.ValueOf(e)
	for i := 0; i < v.NumField(); i++ {
		if endpoint := v.Field(i).Interface(); len(endpoint.(string)) > 1 {
			u, err := url.Parse(endpoint.(string))
			if err != nil {
				return fmt.Errorf("url parsing error %w on %v: %v", err, v.Field(i), endpoint)
			}

			if len(u.Scheme) == 0 {
				return fmt.Errorf("missing protocol %w, on %v: %v", ErrMissingProtocol, v.Field(i), endpoint)
			}
			atLeastOne++
		}
	}

	if atLeastOne == 0 {
		return ErrAtLeastOne
	}

	// if there is only a single endpoint for an API and that endpoint is used no matter what part of the world you are in
	// that is then the fastest endpoint that can be used
	if atLeastOne == 1 && len(e.Universal) > 0 {
		e.FastestURL = e.Universal
		e.Fallback = e.Universal
	}

	if len(e.Fallback) == 0 {
		// this endpoint should always work
		return ErrFallbackUnset
	}

	return nil
}

// Latency creates a router based on API latency, in order for endpoints to be checked.
// PingInterval must be set, otherwise it will fall back to relying on AWS regional information if set
// and lastly to the fallback URL if none of the above is set
type Latency struct {
	// in case AWS_REGION is present, we will default to that region
	AWSRegion string
	// if a client is not passed in as an optional, the default network client will be used
	Client *http.Client
	// if DebugMode is set, logs from the standard log package will be displayed
	DebugMode bool
	// if PingInterval is not set as optional, endpoints will not be checked for latency periodically
	PingInterval time.Duration
	preset       bool
	shouldGuard  bool
	stopTicker   chan struct{}

	mu sync.RWMutex
	EndPoints
}

// NewLatencyRouter returns a fully initialized network based API router
// if the inputted client is nil, the default client will be used underneath, which has a 500 ms timeout
func NewLatencyRouter(endpoints EndPoints, options ...func(*Latency)) (*Latency, error) {
	if err := endpoints.validate(); err != nil {
		return nil, err
	}

	region := strings.ToLower(os.Getenv("AWS_REGION"))
	if len(region) > 0 {
		switch region {
		case "us-east-1", "us-east-2":
			endpoints.FastestURL = endpoints.USEast
		case "us-west-1", "us-west-2":
			endpoints.FastestURL = endpoints.USWest
		case "ap-south-1", "  ap-southeast-1", "ap-southeast-2":
			endpoints.FastestURL = endpoints.AsiaPacific
		case "eu-central-1":
			endpoints.FastestURL = endpoints.Europe
		}
	}

	l := &Latency{
		AWSRegion:  region,
		Client:     defaultClient,
		EndPoints:  endpoints,
		mu:         sync.RWMutex{},
		stopTicker: make(chan struct{}, 1),
		preset:     len(endpoints.FastestURL) > 0,
	}

	for _, option := range options {
		option(l)
	}

	if l.PingInterval.Nanoseconds() > 0.0 {
		l.shouldGuard = true
		go l.periodicallyPingEndpoints()
	}

	return l, nil
}

// GetURL returns the fastest API endpoint from the inputted latency configuration
func (l *Latency) GetURL() (u string) {
	// we only need to guard if and only if the data is being periodically refreshed
	if l.shouldGuard {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}

	if len(l.FastestURL) != 0 {
		return l.FastestURL
	}

	if len(l.Universal) != 0 {
		return l.Universal
	}

	if len(l.Fallback) != 0 {
		return l.Fallback
	}
	return
}

// StopPingingEndpoints terminates the ticker used to periodically check endpoints for latency and status
// it's important this function is called to clean up ticker resources
func (l *Latency) StopPingingEndpoints() {
	if l.PingInterval.Nanoseconds() == 0.0 {
		return
	}

	select {
	case l.stopTicker <- struct{}{}:
	default:
	}
}

func (l *Latency) findLowLatencyEndpoint() {
	// the container is equal to the number of endpoints to hit
	// we only care about the first one, this is to help with deadlocking
	quickestEndpointCh := make(chan string, 1)

	ctx, cancel := context.WithTimeout(context.Background(), l.Client.Timeout)
	defer cancel()
	if l.preset {
	loop:
		// if the preset URL fails
		for i := 0; i < 3; i++ {
			// this is a blocking call
			statusCode, err := l.headRequestPresetEndpoint(l.FastestURL)
			err = checkResponseError(err)
			switch err {
			case nil:
				if (statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices) && err == nil {
					quickestEndpointCh <- l.FastestURL
					l.logf("present URL %s is still good\n", l.FastestURL)
					break loop
				}
			case ErrTimeout, ErrConnectionReset:
				l.logf("present URL %s timed out or had it's connection reset\n", l.FastestURL)
				// do nothing, let the for loop try again
			case ErrNoSuchHost:
				l.logf("present URL %s host could not be found\n", l.FastestURL)
				break loop
			}
		}
	}

	// if new endpoints are added, the below needs to be updated as well
	// we could use reflection here to iterate over the items in the struct, but it isn't worth the performance cost
	if len(quickestEndpointCh) == 0 {
		// the first one to return is the quickest endpoint
		go l.headRequest(ctx, l.Universal, quickestEndpointCh)
		go l.headRequest(ctx, l.USEast, quickestEndpointCh)
		go l.headRequest(ctx, l.USWest, quickestEndpointCh)
		go l.headRequest(ctx, l.Europe, quickestEndpointCh)
		go l.headRequest(ctx, l.AsiaPacific, quickestEndpointCh)
	}

waiting:
	for {
		select {
		case endpoint := <-quickestEndpointCh:
			l.mu.Lock()
			l.FastestURL = endpoint
			l.mu.Unlock()
			l.logf("fastest chosen URL: %s\n", l.FastestURL)
			quickestEndpointCh = nil
			break waiting
		case <-time.After(l.Client.Timeout): // in-case something happens, this function call shouldn't panic
			l.logf("all endpoints took longer than : %v, a fast URL could not be chosen\n", l.Client.Timeout)
			break waiting
		}
	}
	quickestEndpointCh = nil
	return
}

func (l *Latency) headRequest(ctx context.Context, endpoint string, quickestEndpoint chan string) {
	if len(endpoint) == 0 {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
	if err != nil {
		return
	}

	res, err := l.Client.Do(req)
	if err != nil {
		return
	}

	// trust no one
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	if !(res.StatusCode >= http.StatusOK && res.StatusCode < http.StatusMultipleChoices) {
		return
	}

	// send back the fast endpoint
	select {
	case quickestEndpoint <- endpoint:
	default:
	}

	return
}

func (l *Latency) headRequestPresetEndpoint(endpoint string) (int, error) {
	if len(endpoint) == 0 {
		return 0, ErrNoSuchHost
	}

	res, err := l.Client.Head(endpoint)
	if err != nil {
		return 0, err
	}
	// trust no one
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return res.StatusCode, ErrBadStatus
	}

	return res.StatusCode, nil
}

func (l *Latency) log(v ...interface{}) {
	if l.DebugMode {
		log.Println(v...)
	}
}

func (l *Latency) logf(format string, v ...interface{}) {
	if l.DebugMode {
		log.Printf(format, v...)
	}
}

func (l *Latency) periodicallyPingEndpoints() {
	// do an initial check before ticking
	l.findLowLatencyEndpoint()
	// then tick away for potential updates
	ticker := time.NewTicker(l.PingInterval)
	for {
		select {
		case <-ticker.C:
			l.log("pinging endpoints for latency")
			l.findLowLatencyEndpoint()
		case <-l.stopTicker:
			ticker.Stop()
			return
		}
	}
}

func checkResponseError(err error) error {
	if err != nil {
		if tErr, ok := err.(net.Error); ok && tErr.Timeout() {
			return ErrTimeout
		} else if opErr, ok := err.(*net.OpError); ok && ((opErr.Err.Error() == syscall.ECONNRESET.Error()) || strings.Contains(opErr.Err.Error(), "connection reset by peer")) {
			return ErrConnectionReset
		} else if dnsErr, ok := err.(*net.DNSError); ok && strings.Contains(dnsErr.Err, "no such host") {
			return ErrNoSuchHost
		}
		return err
	}
	return nil
}
