package router

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

var (
	// defaultClient provides a network client with a the timeout set to 2seconds and 0 keep-alives
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
	// ErrAtLeastOne at least one field of EndPoints needs to initialized
	ErrAtLeastOne = errors.New("at least one endpoint has to be passed in")
	// ErrBadStatus notifies the user that the status code is not a 200
	ErrBadStatus = errors.New("received a non 200 status code")
	// ErrFallbackUnset notifties that the fallback should be sent, even if it's a duplicative endpoint
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

// EndPoints belonging the the API service that is being used
type EndPoints struct {
	AsiaPacific string `json:"asia_pacific,omitempty"` // APAC
	Europe      string `json:"europe,omitempty"`       // EU
	Universal   string `json:"universal,omitempty"`    // Some APIs contain a single endpoint, which is latency load balanced by the DNS and load balancer
	USEast      string `json:"us_east,omitempty"`      // us-east-1
	USWest      string `json:"us_west,omitempty"`      // us-west-1
	Fallback    string `json:"fallback,omitempty"`     // provides an optional endpoint to fallback to in emergencies
	FastestURL  string `json:"fastest_url,omitempty"`  // is the fastest endpoint based on a head request
}

// normally reflection should be avoided because it's very slow
// however, because this method is called once at initialization, this should be okay
func (e EndPoints) validate() error {
	var atLeastOne int
	v := reflect.ValueOf(e)
	for i := 0; i < v.NumField(); i++ {
		if endpoint := v.Field(i).Interface(); len(endpoint.(string)) > 1 {
			u, err := url.Parse(endpoint.(string))
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("error with %v: %v", v.Field(i), endpoint))
			}

			if len(u.Scheme) == 0 {
				return errors.Wrap(ErrMissingProtocol, fmt.Sprintf("missing protocol, with %v: %v", v.Field(i), endpoint))
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

// Latency creates a router based on API latency
type Latency struct {
	// incase AWS_REGION is present we will default to that region
	AWSRegion string
	client    *http.Client
	preset    bool

	mu *sync.RWMutex
	EndPoints
	updated bool
}

// NewLatencyRouter returns a fully initialized network based API router
// if the inputted client is nil, the default client will be used underneath, which has a 500ms timeout
func NewLatencyRouter(client *http.Client, endpoints EndPoints) (*Latency, error) {
	if client == nil {
		client = defaultClient
	}

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

	return &Latency{
		AWSRegion: region,
		mu:        new(sync.RWMutex),
		client:    client,
		preset:    len(endpoints.FastestURL) > 0,
		EndPoints: endpoints,
	}, nil
}

// GetURL returns the fasters API endpoint from the inputted latency configuration
func (l Latency) GetURL() (u string) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.FastestURL) != 0 {
		return l.FastestURL
	}

	if len(l.Fallback) != 0 {
		return l.Fallback
	}
	return
}

func (l *Latency) findLowLatencyEndpoint() {
	l.mu.Lock()
	l.updated = false
	l.mu.Unlock()

	// the container is equal to the number of endpoints to hit
	// we only care about the first one, this is to help with dead locking
	quickestEndpointCh := make(chan string, 1)
	defer close(quickestEndpointCh)

	ctx, cancel := context.WithTimeout(context.Background(), l.client.Timeout)

	if l.preset {
	loop:
		// if the preset URL fails
		for i := 0; i < 3; i++ {
			// this is a blocking call
			statusCode, err := l.headRequestPresetEndpoint(l.FastestURL)
			err = checkResponseError(err)
			switch err {
			case nil:
				if (statusCode == http.StatusOK) && err == nil {
					quickestEndpointCh <- l.FastestURL
					break loop
				}
			case ErrTimeout, ErrConnectionReset, ErrNoSuchHost:
				// do nothing, let the for loop try again
			case ErrNoSuchHost:
				break loop
			}
		}
	}

	// if new endpoints are added the below needs to be updated as well
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
		case l.FastestURL = <-quickestEndpointCh:
			quickestEndpointCh = nil
			break waiting
		case <-time.After(l.client.Timeout): // incase something happens, this function call shouldn't panic
			break waiting
		}
	}

	cancel()
	return
}

func (l *Latency) headRequest(ctx context.Context, endpoint string, quickestEndpoint chan string) {
	if len(endpoint) == 0 {
		return
	}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return
	}
	req.WithContext(ctx)

	res, err := l.client.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()

	// trust no one
	go io.Copy(ioutil.Discard, res.Body)

	if res.StatusCode != http.StatusOK {
		return
	}

	l.mu.RLock()
	if l.updated {
		return
	}
	l.mu.RUnlock()

	// update, update before sending back the channel to block subsequent channel sends
	l.mu.Lock()
	l.updated = true
	l.mu.Unlock()

	// send back the fast endpoint
	quickestEndpoint <- endpoint
	return
}

func (l *Latency) headRequestPresetEndpoint(endpoint string) (int, error) {
	if len(endpoint) == 0 {
		return 0, ErrNoSuchHost
	}

	res, err := l.client.Head(endpoint)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	// trust no one
	go io.Copy(ioutil.Discard, res.Body)
	if res.StatusCode != http.StatusOK {
		return res.StatusCode, ErrBadStatus
	}

	return res.StatusCode, nil
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
