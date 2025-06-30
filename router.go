package router

import (
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
)

// EndPoints belonging the API service that is being used
type EndPoints struct {
	AsiaPacific string `json:"asia_pacific,omitempty"` // APAC
	Europe      string `json:"europe,omitempty"`       // EU
	Universal   string `json:"universal,omitempty"`    // Some APIs contain a single endpoint, which is a latency load balanced by the DNS and load balancer
	USEast      string `json:"us_east,omitempty"`      // us-east-1
	USWest      string `json:"us_west,omitempty"`      // us-west-1
	Fallback    string `json:"fallback,omitempty"`     // provides an optional endpoint to fall back to in emergencies
	ClosestURL  string `json:"closest_url,omitempty"`
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

	if atLeastOne == 1 && len(e.Universal) > 0 {
		e.Fallback = e.Universal
	}

	if len(e.Fallback) == 0 {
		// this endpoint should always work
		return ErrFallbackUnset
	}

	return nil
}

// Router creates a router based on API latency, in order for endpoints to be checked.
// PingInterval must be set, otherwise it will fall back to relying on AWS regional information if set
// and lastly to the fallback URL if none of the above is set
type Router struct {
	// in case AWS_REGION is present, we will default to that region
	AWSRegion string
	EndPoints

	routerModifier IRouterModifier
}

// NewEnvironmentRouter returns a fully initialized network based API router
// if the inputted client is nil, the default client will be used underneath, which has a 500 ms timeout
func NewEnvironmentRouter(endpoints EndPoints) (*Router, error) {
	if err := endpoints.validate(); err != nil {
		return nil, err
	}

	region := strings.ToLower(os.Getenv("AWS_REGION"))
	if len(region) > 0 {
		switch region {
		case "us-east-1", "us-east-2":
			endpoints.ClosestURL = endpoints.USEast
		case "us-west-1", "us-west-2":
			endpoints.ClosestURL = endpoints.USWest
		case "ap-south-1", "  ap-southeast-1", "ap-southeast-2":
			endpoints.ClosestURL = endpoints.AsiaPacific
		case "eu-central-1":
			endpoints.ClosestURL = endpoints.Europe
		}
	}

	r := &Router{
		AWSRegion: region,
		EndPoints: endpoints,
	}

	return r, nil
}

// GetRouterURL returns the fastest API endpoint from the inputted latency configuration
func (r *Router) GetRouterURL() (u string) {
	if len(r.ClosestURL) != 0 {
		return r.ClosestURL
	}

	if len(r.Universal) != 0 {
		return r.Universal
	}

	if len(r.Fallback) != 0 {
		return r.Fallback
	}
	return
}

// GetModifierURL uses url from the supplied modifier and falls back to GetRouterURL is the returned endpoint is empty
func (r *Router) GetModifierURL() (u string) {
	if r.routerModifier != nil {
		endpoint := r.routerModifier.GetEndpoint()
		if len(endpoint) == 0 {
			return r.GetRouterURL()
		}
		return endpoint
	}
	return r.GetRouterURL()
}

// AddRouterModifier assigns a modifier to change the internal logic of the router
// only 1 modifier may be used per instance of a router
func (r *Router) AddRouterModifier(routerModifier IRouterModifier) {
	if r.routerModifier == nil {
		r.routerModifier = routerModifier
	}
}
