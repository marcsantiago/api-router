package router

import (
	"net/http"
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

			l := NewLatencyCheckerModifier(&endpoints,
				WithCustomClient(httpClient),
			)
			l.findLowLatencyEndpoint()
			httpClient.CloseIdleConnections()

			if !strings.Contains(l.GetEndpoint(), tt.args.currentLocal) {
				t.Fatalf("Router.findLowLatencyEndpoint() got %s wanted an endpoint containing %s", l.GetEndpoint(), tt.args.currentLocal)
			}
		})
	}
}

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

			l := NewLatencyCheckerModifier(&endpoints,
				WithCustomClient(httpClient),
				WithCustomPingInterval(500*time.Millisecond),
			)
			l.StopPingingEndpoints()
			httpClient.CloseIdleConnections()

			time.Sleep(2500 * time.Millisecond)
			if !strings.Contains(l.GetEndpoint(), tt.want) {
				t.Fatalf("Router.findLowLatencyEndpoint() got %s wanted an endpoint containing %s", l.GetEndpoint(), tt.want)
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
		l := NewLatencyCheckerModifier(&endpoints,
			WithCustomClient(httpClient),
			WithCustomPingInterval(500*time.Millisecond),
		)
		l.StopPingingEndpoints()
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(1000 * time.Millisecond)
	httpClient.CloseIdleConnections()
}
