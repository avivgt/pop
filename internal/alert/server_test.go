/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

const testNamespace = "pop-system"

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	utilruntime.Must(popv1alpha1.AddToScheme(s))
	return s
}

func newServer(t *testing.T, authToken string, objs ...client.Object) (*Server, client.Client) {
	t.Helper()
	s := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	return NewServer(c, testNamespace, 9443, authToken, logr.Discard()), c
}

func postEvent(t *testing.T, srv *Server, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/power-event", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	srv.handlePowerEvent(rr, req)
	return rr
}

func TestMapEventType(t *testing.T) {
	cases := []struct {
		in      string
		want    popv1alpha1.PowerEventType
		wantErr bool
	}{
		{"power_failure", popv1alpha1.PowerEventTypeFailure, false},
		{"power_restored", popv1alpha1.PowerEventTypeRestored, false},
		{"test", popv1alpha1.PowerEventTypeTest, false},
		{"", "", true},
		{"bogus", "", true},
		{"POWER_FAILURE", "", true}, // case sensitive
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := mapEventType(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("mapEventType(%q): expected error, got nil", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("mapEventType(%q): unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("mapEventType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHandlePowerEvent_MethodNotAllowed(t *testing.T) {
	srv, _ := newServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/power-event", nil)
	rr := httptest.NewRecorder()
	srv.handlePowerEvent(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET on power-event: got %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlePowerEvent_Auth(t *testing.T) {
	const token = "s3cret"
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"missing header", nil, http.StatusUnauthorized},
		{"malformed no bearer", map[string]string{"Authorization": token}, http.StatusUnauthorized},
		{"wrong token", map[string]string{"Authorization": "Bearer nope"}, http.StatusUnauthorized},
		{"valid token", map[string]string{"Authorization": "Bearer " + token}, http.StatusAccepted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newServer(t, token)
			rr := postEvent(t, srv, `{"type":"power_failure","source":"ups-1"}`, tc.headers)
			if rr.Code != tc.want {
				t.Fatalf("auth %q: got %d, want %d (body=%s)", tc.name, rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestHandlePowerEvent_BadBody(t *testing.T) {
	srv, _ := newServer(t, "")
	rr := postEvent(t, srv, `{not-json`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad body: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandlePowerEvent_UnknownType(t *testing.T) {
	srv, _ := newServer(t, "")
	rr := postEvent(t, srv, `{"type":"meltdown","source":"ups-1"}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown type: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandlePowerEvent_CreatesCR(t *testing.T) {
	cases := []struct {
		reqType        string
		wantType       popv1alpha1.PowerEventType
		wantNamePrefix string
	}{
		{"power_failure", popv1alpha1.PowerEventTypeFailure, "power-failure-"},
		{"power_restored", popv1alpha1.PowerEventTypeRestored, "power-restored-"},
		{"test", popv1alpha1.PowerEventTypeTest, "power-test-"},
	}
	for _, tc := range cases {
		t.Run(tc.reqType, func(t *testing.T) {
			srv, c := newServer(t, "")
			body := `{"type":"` + tc.reqType + `","source":"ups-1","message":"line lost"}`
			rr := postEvent(t, srv, body, nil)
			if rr.Code != http.StatusAccepted {
				t.Fatalf("create: got %d, want %d (body=%s)", rr.Code, http.StatusAccepted, rr.Body.String())
			}

			var resp powerEventResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if !strings.HasPrefix(resp.Name, tc.wantNamePrefix) {
				t.Fatalf("CR name %q missing prefix %q", resp.Name, tc.wantNamePrefix)
			}

			list := &popv1alpha1.PowerEventList{}
			if err := c.List(context.Background(), list, client.InNamespace(testNamespace)); err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(list.Items) != 1 {
				t.Fatalf("expected 1 PowerEvent, got %d", len(list.Items))
			}
			got := list.Items[0]
			if got.Spec.EventType != tc.wantType {
				t.Errorf("EventType = %q, want %q", got.Spec.EventType, tc.wantType)
			}
			if got.Spec.Source != "ups-1" {
				t.Errorf("Source = %q, want %q", got.Spec.Source, "ups-1")
			}
			if got.Spec.Message != "line lost" {
				t.Errorf("Message = %q, want %q", got.Spec.Message, "line lost")
			}
			if got.Spec.TriggeredBy != "webhook" {
				t.Errorf("TriggeredBy = %q, want %q", got.Spec.TriggeredBy, "webhook")
			}
		})
	}
}

func TestHandlePowerStatus_MethodNotAllowed(t *testing.T) {
	srv, _ := newServer(t, "")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/power-status", nil)
	rr := httptest.NewRecorder()
	srv.handlePowerStatus(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST on power-status: got %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlePowerStatus_Empty(t *testing.T) {
	srv, _ := newServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/power-status", nil)
	rr := httptest.NewRecorder()
	srv.handlePowerStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var resp powerStatusResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "Normal" {
		t.Errorf("empty status = %q, want Normal", resp.Status)
	}
	if resp.LastEvent != "" {
		t.Errorf("empty lastEvent = %q, want empty", resp.LastEvent)
	}
}

func TestHandlePowerStatus_ReflectsLatestEvent(t *testing.T) {
	now := time.Now()
	older := &popv1alpha1.PowerEvent{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "older",
			Namespace:         testNamespace,
			CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Hour)),
		},
		Spec: popv1alpha1.PowerEventSpec{EventType: popv1alpha1.PowerEventTypeRestored},
	}
	newer := &popv1alpha1.PowerEvent{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "newer",
			Namespace:         testNamespace,
			CreationTimestamp: metav1.NewTime(now),
		},
		Spec: popv1alpha1.PowerEventSpec{EventType: popv1alpha1.PowerEventTypeFailure},
	}
	// Insert out of chronological order to prove the handler sorts.
	srv, _ := newServer(t, "", older, newer)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/power-status", nil)
	rr := httptest.NewRecorder()
	srv.handlePowerStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var resp powerStatusResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Latest event is a failure -> OnBattery.
	if resp.Status != "OnBattery" {
		t.Errorf("status = %q, want OnBattery (latest event should win)", resp.Status)
	}
	if resp.LastEvent == "" {
		t.Errorf("lastEvent should be set when events exist")
	}
}

// TestWriteJSON verifies the shared JSON writer sets headers and encodes.
func TestWriteJSON(t *testing.T) {
	srv, _ := newServer(t, "")
	rr := httptest.NewRecorder()
	srv.writeJSON(rr, http.StatusTeapot, map[string]string{"k": "v"})
	if rr.Code != http.StatusTeapot {
		t.Fatalf("code = %d, want %d", rr.Code, http.StatusTeapot)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	var decoded map[string]string
	if err := json.NewDecoder(bytes.NewReader(rr.Body.Bytes())).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded["k"] != "v" {
		t.Fatalf("body = %v, want {k:v}", decoded)
	}
}
