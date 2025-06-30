package router

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestLatency_findLowLatencyEndpoint(t *testing.T) {
	t.Parallel()
	_ = os.Setenv("AWS_REGION", "")
	type args struct {
		currentLocal string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "should pick us-east because we are located in us-east",
			args: args{
				currentLocal: "us-east",
			},
		},
		{
			name: "should pick us-west because we are located in us-west",
			args: args{
				currentLocal: "us-west",
			},
		},
		{
			name: "should pick apac because we are located in apac",
			args: args{
				currentLocal: "apac",
			},
		},
		{
			name: "should pick eu because we are located in eu",
			args: args{
				currentLocal: "eu",
			},
		},
		{
			name: "should pick eu because we are located in eu",
			args: args{
				currentLocal: "eu",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if len(tt.args.currentLocal) == 0 {

				}

				switch {
				case strings.Contains(r.URL.String(), tt.args.currentLocal) || len(tt.args.currentLocal) == 0:
					// if this is the region, it is from "no latency is added."
				default:
					time.Sleep(20 * time.Millisecond)
				}
				w.WriteHeader(http.StatusOK)
			})

			httpClient, teardown := testingHTTPClient(h)
			defer teardown()

			endpoints := EndPoints{
				AsiaPacific: "http://foobar.com?region=apac",
				Europe:      "http://foobar.com?region=eu",
				Universal:   "http://foobar.com?region=universal",
				USEast:      "http://foobar.com?region=us-east",
				USWest:      "http://foobar.com?region=us-west",
				Fallback:    "http://foobar.com?region=fallback",
			}

			l := NewLatencyChecker(&endpoints,
				WithCustomClient(httpClient),
			)
			l.findLowLatencyEndpoint()
			httpClient.CloseIdleConnections()

			if !strings.Contains(l.GetFastestEndpoint(), tt.args.currentLocal) {
				t.Fatalf("Router.findLowLatencyEndpoint() got %s wanted an endpoint containing %s", l.GetFastestEndpoint(), tt.args.currentLocal)
			}
		})
	}
}

//This test needs to be part of the router if and only if latency routing is enabled
//func TestLatency_findLowLatencyEndpointWithRegion(t *testing.T) {
//	t.Parallel()
//	_ = os.Setenv("AWS_REGION", "ap-south-1")
//	type args struct {
//		currentLocal string
//	}
//	tests := []struct {
//		name string
//		args args
//	}{
//		{
//			name: "should pick ap-south-1 because AWS_REGION region is set to ap-south-1, local is set to us-east",
//			args: args{
//				currentLocal: "us-east",
//			},
//		},
//		{
//			name: "should pick ap-south-1 because AWS_REGION region is set to ap-south-1, local is set to us-west",
//			args: args{
//				currentLocal: "us-west",
//			},
//		},
//		{
//			name: "should pick ap-south-1 because AWS_REGION region is set to ap-south-1, local is set to apac",
//			args: args{
//				currentLocal: "apac",
//			},
//		},
//		{
//			name: "should pick ap-south-1 because AWS_REGION region is set to ap-south-1, local is set to eu",
//			args: args{
//				currentLocal: "eu",
//			},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//				switch {
//				case strings.Contains(r.URL.String(), tt.args.currentLocal):
//					// if this is the region, it is from "no latency is added."
//				case strings.Contains(r.URL.String(), "west"):
//					time.Sleep(20 * time.Millisecond)
//				case strings.Contains(r.URL.String(), "universal"):
//					time.Sleep(30 * time.Millisecond)
//				case strings.Contains(r.URL.String(), "fallback"):
//					time.Sleep(40 * time.Millisecond)
//				case strings.Contains(r.URL.String(), "eu"):
//					time.Sleep(50 * time.Millisecond)
//				}
//				w.WriteHeader(http.StatusOK)
//			})
//
//			httpClient, teardown := testingHTTPClient(h)
//			defer teardown()
//
//			l := NewLatencyChecker(&EndPoints{
//				AsiaPacific: "http://foobar.com?region=apac",
//				Europe:      "http://foobar.com?region=eu",
//				Universal:   "http://foobar.com?region=universal",
//				USEast:      "http://foobar.com?region=us-east",
//				USWest:      "http://foobar.com?region=us-west",
//				Fallback:    "http://foobar.com?region=fallback",
//			},
//				WithCustomClient(httpClient),
//			)
//			l.findLowLatencyEndpoint()
//
//			// should always be apac because it was set by the region
//			if !strings.Contains(l.GetFastestEndpoint(), "apac") {
//				t.Fatalf("Router.findLowLatencyEndpoint() got %s wanted an endpoint containing %s", l.GetFastestEndpoint(), "apac")
//			}
//
//		})
//	}
//}

func TestLatency_periodicallyPingEndpoints(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("testing.tRunner.func1"),
		goleak.IgnoreTopFunction("time.Sleep"),
	)

	_ = os.Setenv("AWS_REGION", "")
	type args struct {
		currentLocal        string
		simulateHighLatency bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "should pick us-east because we are located in us-east",
			args: args{
				currentLocal: "us-east",
			},
			want: "us-east",
		},
		{
			name: "should pick us-west because we are located in us-west",
			args: args{
				currentLocal: "us-west",
			},
			want: "us-west",
		},
		{
			name: "should pick apac because we are located in apac",
			args: args{
				currentLocal: "apac",
			},
			want: "apac",
		},
		{
			name: "should pick eu because we are located in eu",
			args: args{
				currentLocal: "eu",
			},
			want: "eu",
		},
		{
			name: "should pick eu because we are located in eu",
			args: args{
				currentLocal: "eu",
			},
			want: "eu",
		},
		{
			name: "should pick eu even though we are located in us-east, us-east experiencing high latency",
			args: args{
				currentLocal:        "us-east",
				simulateHighLatency: true,
			},
			want: "eu",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.String(), tt.args.currentLocal) || len(tt.args.currentLocal) == 0:
					// if this is the region, it is from "no latency is added."
					if tt.args.simulateHighLatency {
						time.Sleep(50 * time.Millisecond)
					}
				default:
					if tt.args.simulateHighLatency {
						if strings.Contains(r.URL.String(), tt.want) {
							return
						}
					}
					time.Sleep(20 * time.Millisecond)
				}
				w.WriteHeader(http.StatusOK)
			})

			httpClient, teardown := testingHTTPClient(h)
			defer teardown()

			endpoints := EndPoints{
				AsiaPacific: "http://foobar.com?region=apac",
				Europe:      "http://foobar.com?region=eu",
				Universal:   "http://foobar.com?region=universal",
				USEast:      "http://foobar.com?region=us-east",
				USWest:      "http://foobar.com?region=us-west",
				Fallback:    "http://foobar.com?region=fallback",
			}

			l := NewLatencyChecker(&endpoints,
				WithCustomClient(httpClient),
				WithCustomPingInterval(500*time.Millisecond),
			)
			l.StopPingingEndpoints()
			httpClient.CloseIdleConnections()

			time.Sleep(2500 * time.Millisecond)
			if !strings.Contains(l.GetFastestEndpoint(), tt.want) {
				t.Fatalf("Router.findLowLatencyEndpoint() got %s wanted an endpoint containing %s", l.GetFastestEndpoint(), tt.want)
			}
		})
	}
}

func TestResourcesAreReleased(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("testing.tRunner.func1"),
		goleak.IgnoreTopFunction("time.Sleep"),
	)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	httpClient, teardown := testingHTTPClient(h)
	defer teardown()

	endpoints := EndPoints{
		AsiaPacific: "http://foobar.com?region=apac",
		Europe:      "http://foobar.com?region=eu",
		Universal:   "http://foobar.com?region=universal",
		USEast:      "http://foobar.com?region=us-east",
		USWest:      "http://foobar.com?region=us-west",
		Fallback:    "http://foobar.com?region=fallback",
	}

	for i := 0; i < 10; i++ {
		l := NewLatencyChecker(&endpoints,
			WithCustomClient(httpClient),
			WithCustomPingInterval(500*time.Millisecond),
		)
		l.StopPingingEndpoints()
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(1000 * time.Millisecond)
	httpClient.CloseIdleConnections()
}

func testingHTTPClient(handler http.Handler) (*http.Client, func()) {
	s := httptest.NewServer(handler)
	cli := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
				return net.Dial(network, s.Listener.Addr().String())
			},
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			DisableKeepAlives: true,
		},
		Timeout: 2 * time.Second,
	}
	return cli, s.Close
}
