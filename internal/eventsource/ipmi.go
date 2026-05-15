package eventsource

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

type redfishPowerResponse struct {
	PowerSupplies []struct {
		Status struct {
			Health string `json:"Health"`
			State  string `json:"State"`
		} `json:"Status"`
	} `json:"PowerSupplies"`
}

type IPMIMonitor struct {
	client    client.Client
	namespace string
	config    *popv1alpha1.IPMISourceConfig
	logger    logr.Logger

	mu            sync.Mutex
	status        popv1alpha1.EventSourceStatus
	lastPowerGood bool
	cancel        context.CancelFunc
}

func NewIPMIMonitor(client client.Client, namespace string, config *popv1alpha1.IPMISourceConfig, logger logr.Logger) *IPMIMonitor {
	return &IPMIMonitor{
		client:        client,
		namespace:     namespace,
		config:        config,
		logger:        logger,
		lastPowerGood: true,
		status: popv1alpha1.EventSourceStatus{
			Type:    "ipmi",
			Message: "initializing",
		},
	}
}

func (m *IPMIMonitor) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	interval := time.Duration(m.config.PollingIntervalSeconds) * time.Second
	if interval < 5*time.Second {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *IPMIMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *IPMIMonitor) Status() popv1alpha1.EventSourceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *IPMIMonitor) poll(ctx context.Context) {
	protocol := m.config.Protocol
	if protocol == "" {
		protocol = "redfish"
	}

	if protocol == "redfish" {
		m.pollRedfish(ctx)
	}
}

func (m *IPMIMonitor) pollRedfish(ctx context.Context) {
	port := m.config.Port
	if port == 0 {
		port = 443
	}

	username, password, err := m.getCredentials(ctx)
	if err != nil {
		m.setStatus(false, fmt.Sprintf("failed to read credentials: %v", err))
		return
	}

	url := fmt.Sprintf("https://%s:%d/redfish/v1/Chassis/1/Power", m.config.Host, port)
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: m.config.TLSInsecure},
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		m.setStatus(false, fmt.Sprintf("request creation failed: %v", err))
		return
	}
	req.SetBasicAuth(username, password)

	resp, err := httpClient.Do(req)
	if err != nil {
		m.setStatus(false, fmt.Sprintf("connection failed: %v", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		m.setStatus(false, fmt.Sprintf("failed to read response: %v", err))
		return
	}

	if resp.StatusCode != http.StatusOK {
		m.setStatus(false, fmt.Sprintf("HTTP %d from BMC", resp.StatusCode))
		return
	}

	var power redfishPowerResponse
	if err := json.Unmarshal(body, &power); err != nil {
		m.setStatus(false, fmt.Sprintf("failed to parse response: %v", err))
		return
	}

	m.setStatus(true, "polling")

	powerGood := true
	for _, ps := range power.PowerSupplies {
		if ps.Status.Health != "OK" || ps.Status.State != "Enabled" {
			powerGood = false
			break
		}
	}

	m.mu.Lock()
	wasGood := m.lastPowerGood
	m.lastPowerGood = powerGood
	m.mu.Unlock()

	if wasGood && !powerGood {
		msg := "Redfish: power supply degraded"
		m.logger.Info(msg)
		if err := createPowerEvent(ctx, m.client, m.namespace, "ipmi", popv1alpha1.PowerEventTypeFailure, msg); err != nil {
			m.logger.Error(err, "failed to create PowerEvent from IPMI/Redfish")
		}
	} else if !wasGood && powerGood {
		msg := "Redfish: power supply restored"
		m.logger.Info(msg)
		if err := createPowerEvent(ctx, m.client, m.namespace, "ipmi", popv1alpha1.PowerEventTypeRestored, msg); err != nil {
			m.logger.Error(err, "failed to create PowerEvent from IPMI/Redfish")
		}
	}
}

func (m *IPMIMonitor) getCredentials(ctx context.Context) (string, string, error) {
	if m.config.AuthSecret == "" {
		return "", "", fmt.Errorf("authSecret is required for IPMI/Redfish")
	}

	secret := &corev1.Secret{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: m.config.AuthSecret, Namespace: m.namespace}, secret); err != nil {
		return "", "", err
	}

	username := string(secret.Data["username"])
	password := string(secret.Data["password"])
	return username, password, nil
}

func (m *IPMIMonitor) setStatus(connected bool, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := metav1.Now()
	m.status = popv1alpha1.EventSourceStatus{
		Type:      "ipmi",
		Connected: connected,
		LastCheck: &now,
		Message:   message,
	}
}
