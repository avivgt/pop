package eventsource

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

type NUTMonitor struct {
	client    client.Client
	namespace string
	config    *popv1alpha1.NUTSourceConfig
	logger    logr.Logger

	mu         sync.Mutex
	status     popv1alpha1.EventSourceStatus
	lastStatus string
	cancel     context.CancelFunc
}

func NewNUTMonitor(client client.Client, namespace string, config *popv1alpha1.NUTSourceConfig, logger logr.Logger) *NUTMonitor {
	return &NUTMonitor{
		client:     client,
		namespace:  namespace,
		config:     config,
		logger:     logger,
		lastStatus: "OL",
		status: popv1alpha1.EventSourceStatus{
			Type:    "nut",
			Message: "initializing",
		},
	}
}

func (m *NUTMonitor) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	interval := time.Duration(m.config.PollingIntervalSeconds) * time.Second
	if interval < 5*time.Second {
		interval = 10 * time.Second
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

func (m *NUTMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *NUTMonitor) Status() popv1alpha1.EventSourceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *NUTMonitor) poll(ctx context.Context) {
	port := m.config.Port
	if port == 0 {
		port = 3493
	}

	upsStatus, err := m.queryNUT(ctx, port)
	if err != nil {
		m.setStatus(false, fmt.Sprintf("NUT query failed: %v", err))
		return
	}

	m.setStatus(true, fmt.Sprintf("ups.status=%s", upsStatus))

	isOnline := strings.Contains(upsStatus, "OL")
	isOnBattery := strings.Contains(upsStatus, "OB")
	isLowBattery := strings.Contains(upsStatus, "LB")
	isFSD := strings.Contains(upsStatus, "FSD")

	m.mu.Lock()
	prevStatus := m.lastStatus
	m.lastStatus = upsStatus
	m.mu.Unlock()

	prevOnline := strings.Contains(prevStatus, "OL")

	if prevOnline && (isOnBattery || isLowBattery || isFSD) {
		msg := fmt.Sprintf("NUT: power failure detected (status=%s)", upsStatus)
		m.logger.Info(msg)
		if err := createPowerEvent(ctx, m.client, m.namespace, "nut", popv1alpha1.PowerEventTypeFailure, msg); err != nil {
			m.logger.Error(err, "failed to create PowerEvent from NUT")
		}
	} else if !prevOnline && isOnline && !isOnBattery {
		msg := fmt.Sprintf("NUT: power restored (status=%s)", upsStatus)
		m.logger.Info(msg)
		if err := createPowerEvent(ctx, m.client, m.namespace, "nut", popv1alpha1.PowerEventTypeRestored, msg); err != nil {
			m.logger.Error(err, "failed to create PowerEvent from NUT")
		}
	}
}

func (m *NUTMonitor) queryNUT(ctx context.Context, port int32) (string, error) {
	addr := fmt.Sprintf("%s:%d", m.config.Host, port)
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return "", err
	}

	if m.config.AuthSecret != "" {
		username, password, err := m.getCredentials(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to read NUT credentials: %w", err)
		}
		if _, err := fmt.Fprintf(conn, "USERNAME %s\n", username); err != nil {
			return "", err
		}
		if _, err := fmt.Fprintf(conn, "PASSWORD %s\n", password); err != nil {
			return "", err
		}
	}

	upsName := m.config.UPSName
	if upsName == "" {
		upsName = "ups"
	}

	if _, err := fmt.Fprintf(conn, "GET VAR %s ups.status\n", upsName); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		line := scanner.Text()
		// Response format: VAR <upsname> ups.status "<value>"
		if strings.HasPrefix(line, "VAR") {
			parts := strings.SplitN(line, "\"", 3)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
		if strings.HasPrefix(line, "ERR") {
			return "", fmt.Errorf("NUT error: %s", line)
		}
	}

	return "", fmt.Errorf("no response from NUT server")
}

func (m *NUTMonitor) getCredentials(ctx context.Context) (string, string, error) {
	secret := &corev1.Secret{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: m.config.AuthSecret, Namespace: m.namespace}, secret); err != nil {
		return "", "", err
	}
	return string(secret.Data["username"]), string(secret.Data["password"]), nil
}

func (m *NUTMonitor) setStatus(connected bool, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := metav1.Now()
	m.status = popv1alpha1.EventSourceStatus{
		Type:      "nut",
		Connected: connected,
		LastCheck: &now,
		Message:   message,
	}
}
