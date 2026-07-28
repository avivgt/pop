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

package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := popv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add popv1alpha1 to scheme: %v", err)
	}
	return s
}

func newConfig(ns, name string, notification *popv1alpha1.NotificationConfig) *popv1alpha1.AutomatedProceduresConfig {
	return &popv1alpha1.AutomatedProceduresConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: popv1alpha1.AutomatedProceduresConfigSpec{
			ClusterPriority: popv1alpha1.ClusterPriorityCritical,
			Notification:    notification,
		},
	}
}

func newPowerEvent(ns, name string, eventType popv1alpha1.PowerEventType) *popv1alpha1.PowerEvent {
	return &popv1alpha1.PowerEvent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: popv1alpha1.PowerEventSpec{
			EventType: eventType,
			Source:    "ups-01",
		},
		Status: popv1alpha1.PowerEventStatus{
			Phase: popv1alpha1.PowerEventPhaseDetected,
		},
	}
}

func TestNotify_NilNotificationConfig(t *testing.T) {
	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", nil)
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "test message")

	if len(records) != 0 {
		t.Errorf("expected 0 records when notification config is nil, got %d", len(records))
	}
}

func TestNotify_WebhookSuccess(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		Webhook: &popv1alpha1.WebhookNotificationConfig{
			Enabled: true,
			URL:     ts.URL,
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "power failure detected")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	rec := records[0]
	if rec.Channel != "webhook" {
		t.Errorf("expected channel 'webhook', got %q", rec.Channel)
	}
	if !rec.Success {
		t.Errorf("expected success=true, got false: %s", rec.Message)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", receivedContentType)
	}

	var payload NotificationPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal received payload: %v", err)
	}
	if payload.Cluster != "test-ns/default" {
		t.Errorf("expected cluster 'test-ns/default', got %q", payload.Cluster)
	}
	if payload.Priority != "Critical" {
		t.Errorf("expected priority 'Critical', got %q", payload.Priority)
	}
	if payload.EventType != "PowerFailure" {
		t.Errorf("expected eventType 'PowerFailure', got %q", payload.EventType)
	}
	if payload.Source != "ups-01" {
		t.Errorf("expected source 'ups-01', got %q", payload.Source)
	}
	if payload.Phase != "Detected" {
		t.Errorf("expected phase 'Detected', got %q", payload.Phase)
	}
	if payload.Message != "power failure detected" {
		t.Errorf("expected message 'power failure detected', got %q", payload.Message)
	}
}

func TestNotify_WebhookWithAuth(t *testing.T) {
	var receivedAuth string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	authSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webhook-auth",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"authorization": []byte("Bearer my-secret-token"),
		},
	}

	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(authSecret).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		Webhook: &popv1alpha1.WebhookNotificationConfig{
			Enabled:    true,
			URL:        ts.URL,
			AuthSecret: "webhook-auth",
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "test")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if !records[0].Success {
		t.Errorf("expected success=true, got false: %s", records[0].Message)
	}
	if receivedAuth != "Bearer my-secret-token" {
		t.Errorf("expected Authorization 'Bearer my-secret-token', got %q", receivedAuth)
	}
}

func TestNotify_WebhookEmptyURL(t *testing.T) {
	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		Webhook: &popv1alpha1.WebhookNotificationConfig{
			Enabled: true,
			URL:     "",
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "test")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Success {
		t.Error("expected success=false for empty URL")
	}
	if records[0].Channel != "webhook" {
		t.Errorf("expected channel 'webhook', got %q", records[0].Channel)
	}
}

func TestNotify_WebhookAuthSecretMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not have been sent when auth secret is missing")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	s := newScheme(t)
	// No secret objects in the fake client.
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		Webhook: &popv1alpha1.WebhookNotificationConfig{
			Enabled:    true,
			URL:        ts.URL,
			AuthSecret: "nonexistent-secret",
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "test")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Success {
		t.Error("expected success=false when auth secret is missing")
	}
}

func TestNotify_WebhookNon2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		Webhook: &popv1alpha1.WebhookNotificationConfig{
			Enabled: true,
			URL:     ts.URL,
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "test")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Success {
		t.Error("expected success=false for HTTP 500")
	}
	if got := records[0].Message; got != "webhook returned HTTP 500" {
		t.Errorf("expected message mentioning HTTP 500, got %q", got)
	}
}

func TestNotify_SMTPNoHost(t *testing.T) {
	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		SMTP: &popv1alpha1.SMTPNotificationConfig{
			Enabled: true,
			Host:    "",
			To:      []string{"admin@example.com"},
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "test")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Success {
		t.Error("expected success=false for empty SMTP host")
	}
	if records[0].Channel != "smtp" {
		t.Errorf("expected channel 'smtp', got %q", records[0].Channel)
	}
}

func TestNotify_SMTPNoRecipients(t *testing.T) {
	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		SMTP: &popv1alpha1.SMTPNotificationConfig{
			Enabled: true,
			Host:    "mail.example.com",
			Port:    587,
			To:      nil,
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "test")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Success {
		t.Error("expected success=false for empty recipients")
	}
	if records[0].Channel != "smtp" {
		t.Errorf("expected channel 'smtp', got %q", records[0].Channel)
	}
}

func TestNotify_AlertmanagerSuccess(t *testing.T) {
	var receivedBody []byte
	var receivedPath string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		Alertmanager: &popv1alpha1.AlertmanagerNotificationConfig{
			Enabled: true,
			URL:     ts.URL,
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "power failure")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if !records[0].Success {
		t.Errorf("expected success=true, got false: %s", records[0].Message)
	}
	if records[0].Channel != "alertmanager" {
		t.Errorf("expected channel 'alertmanager', got %q", records[0].Channel)
	}
	if receivedPath != "/api/v2/alerts" {
		t.Errorf("expected path '/api/v2/alerts', got %q", receivedPath)
	}

	var alerts []alertmanagerAlert
	if err := json.Unmarshal(receivedBody, &alerts); err != nil {
		t.Fatalf("failed to unmarshal alertmanager payload: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	alert := alerts[0]
	if alert.Labels["alertname"] != "POPPowerEvent" {
		t.Errorf("expected alertname 'POPPowerEvent', got %q", alert.Labels["alertname"])
	}
	if alert.Labels["source"] != "ups-01" {
		t.Errorf("expected source label 'ups-01', got %q", alert.Labels["source"])
	}
	if alert.Labels["cluster_priority"] != "Critical" {
		t.Errorf("expected cluster_priority 'Critical', got %q", alert.Labels["cluster_priority"])
	}
	if alert.Annotations["description"] != "power failure" {
		t.Errorf("expected description annotation 'power failure', got %q", alert.Annotations["description"])
	}
	if alert.StartsAt == "" {
		t.Error("expected startsAt to be non-empty")
	}
}

func TestNotify_AlertmanagerSeverity(t *testing.T) {
	tests := []struct {
		name             string
		eventType        popv1alpha1.PowerEventType
		expectedSeverity string
	}{
		{
			name:             "failure maps to critical",
			eventType:        popv1alpha1.PowerEventTypeFailure,
			expectedSeverity: "critical",
		},
		{
			name:             "restored maps to warning",
			eventType:        popv1alpha1.PowerEventTypeRestored,
			expectedSeverity: "warning",
		},
		{
			name:             "test maps to warning",
			eventType:        popv1alpha1.PowerEventTypeTest,
			expectedSeverity: "warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedBody []byte
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				receivedBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("failed to read body: %v", err)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer ts.Close()

			s := newScheme(t)
			cl := fake.NewClientBuilder().WithScheme(s).Build()
			svc := NewService(cl, logr.Discard())

			config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
				Alertmanager: &popv1alpha1.AlertmanagerNotificationConfig{
					Enabled: true,
					URL:     ts.URL,
				},
			})
			event := newPowerEvent("test-ns", "pe-1", tt.eventType)

			records := svc.Notify(context.Background(), config, event, "test")

			if len(records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(records))
			}
			if !records[0].Success {
				t.Fatalf("expected success=true: %s", records[0].Message)
			}

			var alerts []alertmanagerAlert
			if err := json.Unmarshal(receivedBody, &alerts); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(alerts) != 1 {
				t.Fatalf("expected 1 alert, got %d", len(alerts))
			}
			if alerts[0].Labels["severity"] != tt.expectedSeverity {
				t.Errorf("expected severity %q, got %q", tt.expectedSeverity, alerts[0].Labels["severity"])
			}
		})
	}
}

func TestNotify_AlertmanagerEmptyURL(t *testing.T) {
	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		Alertmanager: &popv1alpha1.AlertmanagerNotificationConfig{
			Enabled: true,
			URL:     "",
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "test")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Success {
		t.Error("expected success=false for empty Alertmanager URL")
	}
	if records[0].Channel != "alertmanager" {
		t.Errorf("expected channel 'alertmanager', got %q", records[0].Channel)
	}
}

func TestNotify_KubernetesEventFailure(t *testing.T) {
	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		KubernetesEvents: &popv1alpha1.KubernetesEventsConfig{
			Enabled: true,
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "power failure on ups-01")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	rec := records[0]
	if rec.Channel != "kubernetes-event" {
		t.Errorf("expected channel 'kubernetes-event', got %q", rec.Channel)
	}
	if !rec.Success {
		t.Errorf("expected success=true, got false: %s", rec.Message)
	}

	// Verify the created Event object via the fake client.
	eventList := &corev1.EventList{}
	if err := cl.List(context.Background(), eventList); err != nil {
		t.Fatalf("failed to list events: %v", err)
	}
	if len(eventList.Items) != 1 {
		t.Fatalf("expected 1 k8s event, got %d", len(eventList.Items))
	}
	k8sEvent := eventList.Items[0]
	if k8sEvent.Reason != "PowerFailure" {
		t.Errorf("expected reason 'PowerFailure', got %q", k8sEvent.Reason)
	}
	if k8sEvent.Type != corev1.EventTypeWarning {
		t.Errorf("expected event type %q, got %q", corev1.EventTypeWarning, k8sEvent.Type)
	}
	if k8sEvent.Message != "power failure on ups-01" {
		t.Errorf("expected message 'power failure on ups-01', got %q", k8sEvent.Message)
	}
	if k8sEvent.InvolvedObject.Kind != "PowerEvent" {
		t.Errorf("expected involved object kind 'PowerEvent', got %q", k8sEvent.InvolvedObject.Kind)
	}
	if k8sEvent.InvolvedObject.Name != "pe-1" {
		t.Errorf("expected involved object name 'pe-1', got %q", k8sEvent.InvolvedObject.Name)
	}
}

func TestNotify_KubernetesEventRestored(t *testing.T) {
	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		KubernetesEvents: &popv1alpha1.KubernetesEventsConfig{
			Enabled: true,
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeRestored)

	records := svc.Notify(context.Background(), config, event, "power restored")

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if !records[0].Success {
		t.Errorf("expected success=true: %s", records[0].Message)
	}

	eventList := &corev1.EventList{}
	if err := cl.List(context.Background(), eventList); err != nil {
		t.Fatalf("failed to list events: %v", err)
	}
	if len(eventList.Items) != 1 {
		t.Fatalf("expected 1 k8s event, got %d", len(eventList.Items))
	}
	k8sEvent := eventList.Items[0]
	if k8sEvent.Reason != "PowerRestored" {
		t.Errorf("expected reason 'PowerRestored', got %q", k8sEvent.Reason)
	}
	if k8sEvent.Type != corev1.EventTypeNormal {
		t.Errorf("expected event type %q, got %q", corev1.EventTypeNormal, k8sEvent.Type)
	}
}

func TestNotify_MultipleChannels(t *testing.T) {
	webhookCalled := false
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	alertmanagerCalled := false
	alertmanagerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertmanagerCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer alertmanagerServer.Close()

	s := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	svc := NewService(cl, logr.Discard())

	config := newConfig("test-ns", "default", &popv1alpha1.NotificationConfig{
		Webhook: &popv1alpha1.WebhookNotificationConfig{
			Enabled: true,
			URL:     webhookServer.URL,
		},
		Alertmanager: &popv1alpha1.AlertmanagerNotificationConfig{
			Enabled: true,
			URL:     alertmanagerServer.URL,
		},
	})
	event := newPowerEvent("test-ns", "pe-1", popv1alpha1.PowerEventTypeFailure)

	records := svc.Notify(context.Background(), config, event, "outage")

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if !webhookCalled {
		t.Error("webhook server was not called")
	}
	if !alertmanagerCalled {
		t.Error("alertmanager server was not called")
	}

	channels := make(map[string]bool)
	for _, r := range records {
		channels[r.Channel] = true
		if !r.Success {
			t.Errorf("expected success=true for channel %q: %s", r.Channel, r.Message)
		}
	}
	if !channels["webhook"] {
		t.Error("missing webhook record")
	}
	if !channels["alertmanager"] {
		t.Error("missing alertmanager record")
	}
}
