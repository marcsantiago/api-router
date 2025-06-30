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

func TestEndPoints_validate(t *testing.T) {
	_ = os.Setenv("AWS_REGION", "")
	type fields struct {
		AsiaPacific string
		Europe      string
		Universal   string
		USEast      string
		USWest      string
		Fallback    string
		FastestURL  string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name:    "should fail, no endpoints were passed in",
			fields:  fields{},
			wantErr: true,
		},
		{
			name: "should fail, at least endpoint is not proper",
			fields: fields{
				AsiaPacific: "://apac.foobar.com",
				Europe:      "https://eu.foobar.com",
				Universal:   "https://universal.foobar.com",
				USEast:      "https://us-east.foobar.com",
				USWest:      "https://us-west.foobar.com",
				Fallback:    "https://fallback.foobar.com",
			},
			wantErr: true,
		},
		{
			name: "should fail, at least endpoint is missing the protocal",
			fields: fields{
				AsiaPacific: "https://apac.foobar.com",
				Europe:      "eu.foobar.com",
				Universal:   "https://universal.foobar.com",
				USEast:      "https://us-east.foobar.com",
				USWest:      "https://us-west.foobar.com",
				Fallback:    "https://fallback.foobar.com",
			},
			wantErr: true,
		},
		{
			name: "should fail, a fallback was not set",
			fields: fields{
				USWest: "https://us-west.foobar.com",
			},
			wantErr: true,
		},
		{
			name: "should not fail fallback it automatically set if universal",
			fields: fields{
				Universal: "https://universal.foobar.com",
			},
			wantErr: false,
		},
		{
			name: "should pass all endpoints are proper",
			fields: fields{
				AsiaPacific: "https://apac.foobar.com",
				Europe:      "https://eu.foobar.com",
				Universal:   "https://universal.foobar.com",
				USEast:      "https://us-east.foobar.com",
				USWest:      "https://us-west.foobar.com",
				Fallback:    "https://fallback.foobar.com",
			},
			wantErr: false,
		},
		{
			name: "should pass, there is at least one endpoint",
			fields: fields{
				Universal: "https://universal.foobar.com",
				Fallback:  "https://fallback.foobar.com",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := EndPoints{
				AsiaPacific: tt.fields.AsiaPacific,
				Europe:      tt.fields.Europe,
				Universal:   tt.fields.Universal,
				USEast:      tt.fields.USEast,
				USWest:      tt.fields.USWest,
				Fallback:    tt.fields.Fallback,
				FastestURL:  tt.fields.FastestURL,
			}
			if err := e.validate(); (err != nil) != tt.wantErr {
				t.Errorf("EndPoints.validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLatency_findLowLatencyEndpoint(t *testing.T) {
	_ = os.Setenv("AWS_REGION", "")
	type args struct {
		currentLocal string
		useFallback  bool
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
		{
			name: "should use the fallback",
			args: args{
				currentLocal: "fallback",
				useFallback:  true,
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
					// if this is the region it is from "no latency is added"
				default:
					time.Sleep(20 * time.Millisecond)
				}
				w.WriteHeader(http.StatusOK)
			})

			httpClient, teardown := testingHTTPClient(h)
			defer teardown()

			client := func(l *Latency) {
				l.Client = httpClient
			}

			endpoints := EndPoints{
				AsiaPacific: "http://foobar.com?region=apac",
				Europe:      "http://foobar.com?region=eu",
				Universal:   "http://foobar.com?region=universal",
				USEast:      "http://foobar.com?region=us-east",
				USWest:      "http://foobar.com?region=us-west",
				Fallback:    "http://foobar.com?region=fallback",
			}

			if tt.args.useFallback {
				endpoints = EndPoints{
					Fallback: "http://foobar.com?region=fallback",
				}
			}

			l, _ := NewLatencyRouter(endpoints, client)
			l.findLowLatencyEndpoint()

			if !strings.Contains(l.GetURL(), tt.args.currentLocal) {
				t.Fatalf("Latency.findLowLatencyEndpoint() got %s wanted an endpoint containing %s", l.FastestURL, tt.args.currentLocal)
			}

		})
	}
}

func TestLatency_findLowLatencyEndpointWithRegion(t *testing.T) {
	_ = os.Setenv("AWS_REGION", "ap-south-1")
	type args struct {
		currentLocal string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "should pick ap-south-1 because AWS_REGION region is set to ap-south-1, local is set to us-east",
			args: args{
				currentLocal: "us-east",
			},
		},
		{
			name: "should pick ap-south-1 because AWS_REGION region is set to ap-south-1, local is set to us-west",
			args: args{
				currentLocal: "us-west",
			},
		},
		{
			name: "should pick ap-south-1 because AWS_REGION region is set to ap-south-1, local is set to apac",
			args: args{
				currentLocal: "apac",
			},
		},
		{
			name: "should pick ap-south-1 because AWS_REGION region is set to ap-south-1, local is set to eu",
			args: args{
				currentLocal: "eu",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.String(), tt.args.currentLocal):
					// if this is the region it is from "no latency is added"
				case strings.Contains(r.URL.String(), "west"):
					time.Sleep(10 * time.Millisecond)
				case strings.Contains(r.URL.String(), "universal"):
					time.Sleep(20 * time.Millisecond)
				case strings.Contains(r.URL.String(), "fallback"):
					time.Sleep(30 * time.Millisecond)
				case strings.Contains(r.URL.String(), "eu"):
					time.Sleep(40 * time.Millisecond)
				}
				w.WriteHeader(http.StatusOK)
			})

			httpClient, teardown := testingHTTPClient(h)
			defer teardown()

			client := func(l *Latency) {
				l.Client = httpClient
			}

			l, _ := NewLatencyRouter(EndPoints{
				AsiaPacific: "http://foobar.com?region=apac",
				Europe:      "http://foobar.com?region=eu",
				Universal:   "http://foobar.com?region=universal",
				USEast:      "http://foobar.com?region=us-east",
				USWest:      "http://foobar.com?region=us-west",
				Fallback:    "http://foobar.com?region=fallback",
			}, client)
			l.findLowLatencyEndpoint()

			// should always be apac because it was set by the region
			if !strings.Contains(l.GetURL(), "apac") {
				t.Fatalf("Latency.findLowLatencyEndpoint() got %s wanted an endpoint containing %s", l.FastestURL, "apac")
			}

		})
	}
}

func TestLatency_periodicallyPingEndpoints(t *testing.T) {
	defer goleak.VerifyNone(t)
	if testing.Short() {
		t.Skip("skipping")
	}

	_ = os.Setenv("AWS_REGION", "")
	type args struct {
		currentLocal string
		useFallback  bool
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
		{
			name: "should use the fallback",
			args: args{
				currentLocal: "fallback",
				useFallback:  true,
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
					// if this is the region it is from "no latency is added"
				default:
					time.Sleep(20 * time.Millisecond)
				}
				w.WriteHeader(http.StatusOK)
			})

			httpClient, teardown := testingHTTPClient(h)
			defer teardown()

			client := func(l *Latency) {
				l.Client = httpClient
			}

			endpoints := EndPoints{
				AsiaPacific: "http://foobar.com?region=apac",
				Europe:      "http://foobar.com?region=eu",
				Universal:   "http://foobar.com?region=universal",
				USEast:      "http://foobar.com?region=us-east",
				USWest:      "http://foobar.com?region=us-west",
				Fallback:    "http://foobar.com?region=fallback",
			}

			if tt.args.useFallback {
				endpoints = EndPoints{
					Fallback: "http://foobar.com?region=fallback",
				}
			}

			refresh := func(l *Latency) {
				l.PingInterval = 500 * time.Millisecond
			}

			l, _ := NewLatencyRouter(endpoints, client, refresh)
			l.StopPingingEndpoints()
			time.Sleep(2500 * time.Millisecond)

			if !strings.Contains(l.GetURL(), tt.args.currentLocal) {
				t.Fatalf("Latency.findLowLatencyEndpoint() got %s wanted an endpoint containing %s", l.FastestURL, tt.args.currentLocal)
			}

		})
	}
}

func TestResourcesAreReleased(t *testing.T) {
	defer goleak.VerifyNone(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	httpClient, teardown := testingHTTPClient(h)
	defer teardown()

	client := func(l *Latency) {
		l.Client = httpClient
	}

	endpoints := EndPoints{
		AsiaPacific: "http://foobar.com?region=apac",
		Europe:      "http://foobar.com?region=eu",
		Universal:   "http://foobar.com?region=universal",
		USEast:      "http://foobar.com?region=us-east",
		USWest:      "http://foobar.com?region=us-west",
		Fallback:    "http://foobar.com?region=fallback",
	}

	refresh := func(l *Latency) {
		l.PingInterval = 500 * time.Millisecond
	}

	for i := 0; i < 10; i++ {
		l, _ := NewLatencyRouter(endpoints, client, refresh)
		l.StopPingingEndpoints()
	}
	time.Sleep(1000 * time.Millisecond)
}

func testingHTTPClient(handler http.Handler) (*http.Client, func()) {
	s := httptest.NewServer(handler)
	cli := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
				return net.Dial(network, s.Listener.Addr().String())
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 2 * time.Second,
	}
	return cli, s.Close
}
