package router

import (
	"os"
	"testing"
)

func TestEndPoints_validate(t *testing.T) {
	t.Parallel()
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
				ClosestURL:  tt.fields.FastestURL,
			}
			if err := e.validate(); (err != nil) != tt.wantErr {
				t.Fatalf("EndPoints.validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRouter_GetURL(t *testing.T) {
	type fields struct {
		AWSRegion string
		EndPoints EndPoints
	}
	tests := []struct {
		name   string
		fields fields
		wantU  string
	}{
		{
			name: "the closest endpoint should be us-east",
			fields: fields{
				EndPoints: EndPoints{
					AsiaPacific: "https://apac.foobar.com",
					Europe:      "https://eu.foobar.com",
					Universal:   "https://universal.foobar.com",
					USEast:      "https://us-east.foobar.com",
					USWest:      "https://us-west.foobar.com",
					Fallback:    "https://fallback.foobar.com",
				},
				AWSRegion: "us-east-1",
			},
			wantU: "https://us-east.foobar.com",
		},
		{
			name: "select the universal endpoint, even though we are in us-east, the endpoint is missing",
			fields: fields{
				EndPoints: EndPoints{
					AsiaPacific: "https://apac.foobar.com",
					Europe:      "https://eu.foobar.com",
					Universal:   "https://universal.foobar.com",
					USWest:      "https://us-west.foobar.com",
					Fallback:    "https://fallback.foobar.com",
				},
				AWSRegion: "us-east-1",
			},
			wantU: "https://universal.foobar.com",
		},
		{
			name: "select the fallback endpoint, even though we are in us-east, us-east and universal are missing",
			fields: fields{
				EndPoints: EndPoints{
					AsiaPacific: "https://apac.foobar.com",
					Europe:      "https://eu.foobar.com",
					USWest:      "https://us-west.foobar.com",
					Fallback:    "https://fallback.foobar.com",
				},
				AWSRegion: "us-east-1",
			},
			wantU: "https://fallback.foobar.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AWS_REGION", tt.fields.AWSRegion)
			r, _ := NewEnvironmentRouter(tt.fields.EndPoints)

			if gotU := r.GetRouterURL(); gotU != tt.wantU {
				t.Fatalf("GetRouterURL() = %v, want %v", gotU, tt.wantU)
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
//			l := NewLatencyCheckerModifier(&EndPoints{
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
//			if !strings.Contains(l.GetEndpoint(), "apac") {
//				t.Fatalf("Router.findLowLatencyEndpoint() got %s wanted an endpoint containing %s", l.GetEndpoint(), "apac")
//			}
//
//		})
//	}
//}
