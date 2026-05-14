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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// NotificationPayload is the JSON payload sent to webhook endpoints
// and used as the basis for other notification channel messages.
type NotificationPayload struct {
	Cluster   string `json:"cluster"`
	Priority  string `json:"priority"`
	EventType string `json:"eventType"`
	Source    string `json:"source"`
	Phase     string `json:"phase"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// Service sends notifications through multiple channels configured in
// a PowerProtectionConfig.
type Service struct {
	client client.Client
	logger logr.Logger
}

// NewService creates a new notification Service.
func NewService(client client.Client, logger logr.Logger) *Service {
	return &Service{
		client: client,
		logger: logger,
	}
}

// Notify sends a notification through all enabled channels configured in the
// PowerProtectionConfig. Each channel is tried independently; a failure in one
// channel does not block the others. Returns a slice of NotificationRecord
// describing the outcome of each channel attempt.
func (s *Service) Notify(ctx context.Context, config *popv1alpha1.PowerProtectionConfig, event *popv1alpha1.PowerEvent, message string) []popv1alpha1.NotificationRecord {
	var records []popv1alpha1.NotificationRecord

	payload := NotificationPayload{
		Cluster:   config.Namespace + "/" + config.Name,
		Priority:  string(config.Spec.ClusterPriority),
		EventType: string(event.Spec.EventType),
		Source:    event.Spec.Source,
		Phase:     string(event.Status.Phase),
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	notifConfig := config.Spec.Notification
	if notifConfig == nil {
		return records
	}

	// Webhook notifications.
	if notifConfig.Webhook != nil && notifConfig.Webhook.Enabled {
		record := s.sendWebhook(ctx, notifConfig.Webhook, payload, config.Namespace)
		records = append(records, record)
	}

	// Kubernetes Event notifications.
	if notifConfig.KubernetesEvents != nil && notifConfig.KubernetesEvents.Enabled {
		record := s.sendKubernetesEvent(ctx, event, message)
		records = append(records, record)
	}

	// SMTP email notifications.
	if notifConfig.SMTP != nil && notifConfig.SMTP.Enabled {
		record := s.sendSMTP(ctx, notifConfig.SMTP, payload, config.Namespace)
		records = append(records, record)
	}

	// Alertmanager notifications.
	if notifConfig.Alertmanager != nil && notifConfig.Alertmanager.Enabled {
		record := s.sendAlertmanager(ctx, notifConfig.Alertmanager, payload, config)
		records = append(records, record)
	}

	return records
}

// sendWebhook sends a POST request with the notification payload to the
// configured webhook URL.
func (s *Service) sendWebhook(ctx context.Context, webhookConfig *popv1alpha1.WebhookNotificationConfig, payload NotificationPayload, namespace string) popv1alpha1.NotificationRecord {
	record := popv1alpha1.NotificationRecord{
		Channel: "webhook",
		SentAt:  metav1.Now(),
	}

	if webhookConfig.URL == "" {
		record.Success = false
		record.Message = "webhook URL is not configured"
		s.logger.Error(fmt.Errorf("missing webhook URL"), "Webhook notification skipped")
		return record
	}

	body, err := json.Marshal(payload)
	if err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("failed to marshal payload: %v", err)
		s.logger.Error(err, "Failed to marshal webhook payload")
		return record
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookConfig.URL, bytes.NewReader(body))
	if err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("failed to create request: %v", err)
		s.logger.Error(err, "Failed to create webhook request")
		return record
	}
	req.Header.Set("Content-Type", "application/json")

	// Read authorization from Secret if configured.
	if webhookConfig.AuthSecret != "" {
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{Name: webhookConfig.AuthSecret, Namespace: namespace}
		if err := s.client.Get(ctx, secretKey, secret); err != nil {
			record.Success = false
			record.Message = fmt.Sprintf("failed to read auth secret %q: %v", webhookConfig.AuthSecret, err)
			s.logger.Error(err, "Failed to read webhook auth secret", "secret", webhookConfig.AuthSecret)
			return record
		}
		authValue, ok := secret.Data["authorization"]
		if !ok {
			record.Success = false
			record.Message = fmt.Sprintf("auth secret %q missing 'authorization' key", webhookConfig.AuthSecret)
			s.logger.Error(fmt.Errorf("missing key"), "Webhook auth secret missing 'authorization' key", "secret", webhookConfig.AuthSecret)
			return record
		}
		req.Header.Set("Authorization", string(authValue))
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("webhook request failed: %v", err)
		s.logger.Error(err, "Webhook request failed", "url", webhookConfig.URL)
		return record
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		record.Success = true
		record.Message = fmt.Sprintf("webhook delivered (HTTP %d)", resp.StatusCode)
		s.logger.Info("Webhook notification sent", "url", webhookConfig.URL, "status", resp.StatusCode)
	} else {
		record.Success = false
		record.Message = fmt.Sprintf("webhook returned HTTP %d", resp.StatusCode)
		s.logger.Error(fmt.Errorf("unexpected status code"), "Webhook returned non-2xx", "url", webhookConfig.URL, "status", resp.StatusCode)
	}

	return record
}

// sendKubernetesEvent creates a Kubernetes Event on the PowerEvent CR.
func (s *Service) sendKubernetesEvent(ctx context.Context, powerEvent *popv1alpha1.PowerEvent, message string) popv1alpha1.NotificationRecord {
	record := popv1alpha1.NotificationRecord{
		Channel: "kubernetes-event",
		SentAt:  metav1.Now(),
	}

	eventType := corev1.EventTypeNormal
	reason := "PowerRestored"
	if powerEvent.Spec.EventType == popv1alpha1.PowerEventTypeFailure {
		eventType = corev1.EventTypeWarning
		reason = "PowerFailure"
	} else if powerEvent.Spec.EventType == popv1alpha1.PowerEventTypeTest {
		reason = "PowerTest"
	}

	k8sEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: powerEvent.Name + "-",
			Namespace:    powerEvent.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: popv1alpha1.GroupVersion.String(),
			Kind:       "PowerEvent",
			Name:       powerEvent.Name,
			Namespace:  powerEvent.Namespace,
			UID:        powerEvent.UID,
		},
		Reason:  reason,
		Message: message,
		Type:    eventType,
		Source: corev1.EventSource{
			Component: "pop-notification-service",
		},
		FirstTimestamp: metav1.Now(),
		LastTimestamp:  metav1.Now(),
		Count:          1,
	}

	if err := s.client.Create(ctx, k8sEvent); err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("failed to create Kubernetes event: %v", err)
		s.logger.Error(err, "Failed to create Kubernetes event", "powerEvent", powerEvent.Name)
		return record
	}

	record.Success = true
	record.Message = "Kubernetes event created"
	s.logger.Info("Kubernetes event created", "powerEvent", powerEvent.Name, "type", eventType)
	return record
}

// sendSMTP sends an email notification using the configured SMTP settings.
func (s *Service) sendSMTP(ctx context.Context, smtpConfig *popv1alpha1.SMTPNotificationConfig, payload NotificationPayload, namespace string) popv1alpha1.NotificationRecord {
	record := popv1alpha1.NotificationRecord{
		Channel: "smtp",
		SentAt:  metav1.Now(),
	}

	if smtpConfig.Host == "" {
		record.Success = false
		record.Message = "SMTP host is not configured"
		s.logger.Error(fmt.Errorf("missing SMTP host"), "SMTP notification skipped")
		return record
	}

	if len(smtpConfig.To) == 0 {
		record.Success = false
		record.Message = "no SMTP recipients configured"
		s.logger.Error(fmt.Errorf("no recipients"), "SMTP notification skipped")
		return record
	}

	// Read SMTP credentials from Secret.
	var username, password string
	if smtpConfig.CredentialsSecret != "" {
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{Name: smtpConfig.CredentialsSecret, Namespace: namespace}
		if err := s.client.Get(ctx, secretKey, secret); err != nil {
			record.Success = false
			record.Message = fmt.Sprintf("failed to read SMTP credentials secret %q: %v", smtpConfig.CredentialsSecret, err)
			s.logger.Error(err, "Failed to read SMTP credentials secret", "secret", smtpConfig.CredentialsSecret)
			return record
		}
		username = string(secret.Data["username"])
		password = string(secret.Data["password"])
	}

	// Format the email body.
	subject := fmt.Sprintf("POP Alert: %s from %s", payload.EventType, payload.Source)
	body := fmt.Sprintf("Cluster: %s\nPriority: %s\nEvent Type: %s\nSource: %s\nPhase: %s\nMessage: %s\nTimestamp: %s",
		payload.Cluster, payload.Priority, payload.EventType, payload.Source, payload.Phase, payload.Message, payload.Timestamp)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		smtpConfig.From, strings.Join(smtpConfig.To, ", "), subject, body)

	addr := fmt.Sprintf("%s:%d", smtpConfig.Host, smtpConfig.Port)

	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, smtpConfig.Host)
	}

	if err := smtp.SendMail(addr, auth, smtpConfig.From, smtpConfig.To, []byte(msg)); err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("failed to send email: %v", err)
		s.logger.Error(err, "Failed to send SMTP notification", "host", smtpConfig.Host)
		return record
	}

	record.Success = true
	record.Message = fmt.Sprintf("email sent to %s", strings.Join(smtpConfig.To, ", "))
	s.logger.Info("SMTP notification sent", "to", smtpConfig.To)
	return record
}

// alertmanagerAlert represents a single alert in the Alertmanager API format.
type alertmanagerAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
}

// sendAlertmanager pushes an alert to the Alertmanager API.
func (s *Service) sendAlertmanager(ctx context.Context, amConfig *popv1alpha1.AlertmanagerNotificationConfig, payload NotificationPayload, config *popv1alpha1.PowerProtectionConfig) popv1alpha1.NotificationRecord {
	record := popv1alpha1.NotificationRecord{
		Channel: "alertmanager",
		SentAt:  metav1.Now(),
	}

	if amConfig.URL == "" {
		record.Success = false
		record.Message = "Alertmanager URL is not configured"
		s.logger.Error(fmt.Errorf("missing Alertmanager URL"), "Alertmanager notification skipped")
		return record
	}

	severity := "warning"
	if payload.EventType == string(popv1alpha1.PowerEventTypeFailure) {
		severity = "critical"
	}

	alerts := []alertmanagerAlert{
		{
			Labels: map[string]string{
				"alertname":        "POPPowerEvent",
				"severity":         severity,
				"cluster_priority": string(config.Spec.ClusterPriority),
				"source":           payload.Source,
			},
			Annotations: map[string]string{
				"summary":     fmt.Sprintf("%s detected", payload.EventType),
				"description": payload.Message,
			},
			StartsAt: payload.Timestamp,
		},
	}

	body, err := json.Marshal(alerts)
	if err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("failed to marshal Alertmanager payload: %v", err)
		s.logger.Error(err, "Failed to marshal Alertmanager payload")
		return record
	}

	url := strings.TrimRight(amConfig.URL, "/") + "/api/v2/alerts"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("failed to create Alertmanager request: %v", err)
		s.logger.Error(err, "Failed to create Alertmanager request")
		return record
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("Alertmanager request failed: %v", err)
		s.logger.Error(err, "Alertmanager request failed", "url", url)
		return record
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		record.Success = true
		record.Message = fmt.Sprintf("alert pushed to Alertmanager (HTTP %d)", resp.StatusCode)
		s.logger.Info("Alertmanager notification sent", "url", url, "status", resp.StatusCode)
	} else {
		record.Success = false
		record.Message = fmt.Sprintf("Alertmanager returned HTTP %d", resp.StatusCode)
		s.logger.Error(fmt.Errorf("unexpected status code"), "Alertmanager returned non-2xx", "url", url, "status", resp.StatusCode)
	}

	return record
}
