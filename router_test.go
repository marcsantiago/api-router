package router

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
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

func TestRouter_Latency_GetModifierURL(t *testing.T) {
	type fields struct {
		AWSRegion string
		EndPoints EndPoints
	}
	tests := []struct {
		name          string
		fields        fields
		latencyChecks bool
		currentLocal  string
		wantU         string
	}{
		{
			name: "the closest endpoint should be us-east, no latency checks",
			fields: fields{
				EndPoints: EndPoints{
					AsiaPacific: "http://foobar.com?region=apac",
					Europe:      "http://foobar.com?region=eu",
					Universal:   "http://foobar.com?region=universal",
					USEast:      "http://foobar.com?region=us-east",
					USWest:      "http://foobar.com?region=us-west",
					Fallback:    "http://foobar.com?region=fallback",
				},
				AWSRegion: "us-east-1",
			},
			wantU:        "http://foobar.com?region=us-east",
			currentLocal: "us-east",
		},
		{
			name: "select the universal endpoint, even though we are in us-east, the endpoint is missing, no latency checks",
			fields: fields{
				EndPoints: EndPoints{
					AsiaPacific: "http://foobar.com?region=apac",
					Europe:      "http://foobar.com?region=eu",
					Universal:   "http://foobar.com?region=universal",
					USWest:      "http://foobar.com?region=us-west",
					Fallback:    "http://foobar.com?region=fallback",
				},
				AWSRegion: "us-east-1",
			},
			wantU:        "http://foobar.com?region=universal",
			currentLocal: "universal",
		},
		{
			name: "select the eu endpoint, even though we are in us-east, us-east and universal are missing",
			fields: fields{
				EndPoints: EndPoints{
					AsiaPacific: "http://foobar.com?region=apac",
					Europe:      "http://foobar.com?region=eu",
					USWest:      "http://foobar.com?region=us-west",
					Fallback:    "http://foobar.com?region=fallback",
				},
				AWSRegion: "us-east-1",
			},
			wantU:        "http://foobar.com?region=eu",
			currentLocal: "eu",
		},
		{
			name: "there is only one endpoint fallback",
			fields: fields{
				EndPoints: EndPoints{
					Fallback: "http://foobar.com?region=fallback",
				},
				AWSRegion: "us-east-1",
			},
			wantU:        "http://foobar.com?region=fallback",
			currentLocal: "eu",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AWS_REGION", tt.fields.AWSRegion)
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.String(), tt.currentLocal) || len(tt.currentLocal) == 0:
					// if this is the region, it is from "no latency is added."
				default:
					switch {
					case strings.Contains(r.URL.String(), "eu") && strings.Contains(r.URL.String(), tt.currentLocal):
						return
					default:
						time.Sleep(20 * time.Millisecond)
					}
				}

				w.WriteHeader(http.StatusOK)
			})
			httpClient, teardown := testingHTTPClient(h)
			defer func() {
				teardown()
				httpClient.CloseIdleConnections()
			}()

			r, _ := NewEnvironmentRouter(tt.fields.EndPoints)
			latencyModifier := NewLatencyCheckerModifier(&r.EndPoints,
				WithCustomClient(httpClient),
			)
			r.AddRouterModifier(latencyModifier)
			time.Sleep(100 * time.Millisecond)
			if gotU := r.GetModifierURL(); gotU != tt.wantU {
				t.Fatalf("GetModifierURL() = %v, want %v", gotU, tt.wantU)
			}
		})
	}
}
