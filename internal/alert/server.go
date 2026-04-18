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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// powerEventRequest represents the incoming webhook payload for a power event.
type powerEventRequest struct {
	Type    string `json:"type"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

// powerEventResponse is the response returned after creating a PowerEvent CR.
type powerEventResponse struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// powerStatusResponse is the response for the power status endpoint.
type powerStatusResponse struct {
	Status    string `json:"status"`
	LastEvent string `json:"lastEvent"`
}

// Server is an HTTP server that receives PSU/UPS power event webhooks and
// creates PowerEvent custom resources in the cluster.
type Server struct {
	client    client.Client
	namespace string
	port      int32
	authToken string
	logger    logr.Logger
}

// NewServer creates a new alert Server.
func NewServer(client client.Client, namespace string, port int32, authToken string, logger logr.Logger) *Server {
	return &Server{
		client:    client,
		namespace: namespace,
		port:      port,
		authToken: authToken,
		logger:    logger,
	}
}

// Start starts the HTTP server and blocks until the context is cancelled.
// It implements the manager.Runnable interface.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/power-event", s.handlePowerEvent)
	mux.HandleFunc("/api/v1/power-status", s.handlePowerStatus)

	addr := fmt.Sprintf(":%d", s.port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start the server in a goroutine so we can wait for context cancellation.
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("Starting alert webhook server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for context cancellation or server error.
	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down alert webhook server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// NeedLeaderElection returns true because only the leader should handle alerts
// to avoid duplicate PowerEvent CR creation.
func (s *Server) NeedLeaderElection() bool {
	return true
}

// handlePowerEvent handles POST /api/v1/power-event.
func (s *Server) handlePowerEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate bearer token if auth is configured.
	if s.authToken != "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader || token != s.authToken {
			s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization token"})
			return
		}
	}

	// Parse request body.
	var req powerEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error(err, "Failed to decode power event request")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Map request type to PowerEventType.
	eventType, err := mapEventType(req.Type)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Build the CR name with type and timestamp.
	timestamp := time.Now().Format("20060102-150405")
	typeSuffix := strings.ToLower(strings.TrimPrefix(string(eventType), "Power"))
	if eventType == popv1alpha1.PowerEventTypeTest {
		typeSuffix = "test"
	}
	crName := fmt.Sprintf("power-%s-%s", typeSuffix, timestamp)

	// Create the PowerEvent CR.
	powerEvent := &popv1alpha1.PowerEvent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: s.namespace,
		},
		Spec: popv1alpha1.PowerEventSpec{
			EventType:   eventType,
			Source:      req.Source,
			Message:     req.Message,
			TriggeredBy: "webhook",
		},
	}

	if err := s.client.Create(r.Context(), powerEvent); err != nil {
		s.logger.Error(err, "Failed to create PowerEvent CR", "name", crName)
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create power event"})
		return
	}

	s.logger.Info("Created PowerEvent CR", "name", crName, "type", eventType, "source", req.Source)
	s.writeJSON(w, http.StatusAccepted, powerEventResponse{
		Name:    crName,
		Message: "Power event created",
	})
}

// handlePowerStatus handles GET /api/v1/power-status.
func (s *Server) handlePowerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// List all PowerEvents in the namespace.
	eventList := &popv1alpha1.PowerEventList{}
	if err := s.client.List(r.Context(), eventList, client.InNamespace(s.namespace)); err != nil {
		s.logger.Error(err, "Failed to list PowerEvents")
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list power events"})
		return
	}

	// Determine status from the latest event.
	status := "Normal"
	lastEvent := ""

	if len(eventList.Items) > 0 {
		// Sort by creation timestamp descending to find the latest event.
		sort.Slice(eventList.Items, func(i, j int) bool {
			return eventList.Items[i].CreationTimestamp.After(eventList.Items[j].CreationTimestamp.Time)
		})

		latest := eventList.Items[0]
		lastEvent = latest.CreationTimestamp.Format(time.RFC3339)

		if latest.Spec.EventType == popv1alpha1.PowerEventTypeFailure {
			status = "OnBattery"
		}
	}

	s.writeJSON(w, http.StatusOK, powerStatusResponse{
		Status:    status,
		LastEvent: lastEvent,
	})
}

// writeJSON writes a JSON response with the given status code.
func (s *Server) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error(err, "Failed to encode JSON response")
	}
}

// mapEventType maps a webhook event type string to a PowerEventType.
func mapEventType(t string) (popv1alpha1.PowerEventType, error) {
	switch t {
	case "power_failure":
		return popv1alpha1.PowerEventTypeFailure, nil
	case "power_restored":
		return popv1alpha1.PowerEventTypeRestored, nil
	case "test":
		return popv1alpha1.PowerEventTypeTest, nil
	default:
		return "", fmt.Errorf("unknown event type: %q, must be one of: power_failure, power_restored, test", t)
	}
}
