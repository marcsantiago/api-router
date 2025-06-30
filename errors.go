package router

import "errors"

var (
	// ErrAtLeastOne at least one field of EndPoints needs to initialize
	ErrAtLeastOne = errors.New("at least one endpoint has to be passed in")
	// ErrFallbackUnset notifies that the fallback should be sent, even if it's a duplicative endpoint
	ErrFallbackUnset = errors.New("a fallback endpoint should be sent as a safety mechanism")
	// ErrMissingProtocol a protocol must be present with each endpoint
	ErrMissingProtocol = errors.New("missing http or https")
)
