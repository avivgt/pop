package eventsource

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/gosnmp/gosnmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

const (
	defaultBatteryStatusOID = "1.3.6.1.2.1.33.1.2.1.0"
	defaultOutputSourceOID  = "1.3.6.1.2.1.33.1.4.1.0"

	// UPS-MIB battery status values
	batteryNormal   = 2
	batteryLow      = 3
	batteryDepleted = 4

	// UPS-MIB output source values
	outputNormal  = 3
	outputBattery = 5
)

type SNMPMonitor struct {
	client    client.Client
	namespace string
	config    *popv1alpha1.SNMPSourceConfig
	logger    logr.Logger

	mu            sync.Mutex
	status        popv1alpha1.EventSourceStatus
	lastPowerGood bool
	cancel        context.CancelFunc
}

func NewSNMPMonitor(client client.Client, namespace string, config *popv1alpha1.SNMPSourceConfig, logger logr.Logger) *SNMPMonitor {
	return &SNMPMonitor{
		client:        client,
		namespace:     namespace,
		config:        config,
		logger:        logger,
		lastPowerGood: true,
		status: popv1alpha1.EventSourceStatus{
			Type:    "snmp",
			Message: "initializing",
		},
	}
}

func (m *SNMPMonitor) Start(ctx context.Context) error {
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

func (m *SNMPMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *SNMPMonitor) Status() popv1alpha1.EventSourceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *SNMPMonitor) poll(ctx context.Context) {
	port := uint16(m.config.Port)
	if port == 0 {
		port = 161
	}
	community := m.config.Community
	if community == "" {
		community = "public"
	}

	snmpClient := &gosnmp.GoSNMP{
		Target:    m.config.Host,
		Port:      port,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   5 * time.Second,
		Retries:   1,
	}

	if err := snmpClient.Connect(); err != nil {
		m.setStatus(false, fmt.Sprintf("connection failed: %v", err))
		return
	}
	defer func() { _ = snmpClient.Conn.Close() }()

	batteryOID := m.config.BatteryStatusOID
	if batteryOID == "" {
		batteryOID = defaultBatteryStatusOID
	}
	outputOID := m.config.OutputSourceOID
	if outputOID == "" {
		outputOID = defaultOutputSourceOID
	}

	result, err := snmpClient.Get([]string{batteryOID, outputOID})
	if err != nil {
		m.setStatus(false, fmt.Sprintf("SNMP GET failed: %v", err))
		return
	}

	m.setStatus(true, "polling")

	var batteryStatus, outputSource int
	for _, v := range result.Variables {
		val, ok := v.Value.(int)
		if !ok {
			continue
		}
		switch v.Name {
		case "." + batteryOID:
			batteryStatus = val
		case "." + outputOID:
			outputSource = val
		}
	}

	powerGood := batteryStatus == batteryNormal && outputSource == outputNormal

	m.mu.Lock()
	wasGood := m.lastPowerGood
	m.lastPowerGood = powerGood
	m.mu.Unlock()

	if wasGood && !powerGood {
		msg := fmt.Sprintf("SNMP: power failure detected (battery=%d, output=%d)", batteryStatus, outputSource)
		m.logger.Info(msg)
		if err := createPowerEvent(ctx, m.client, m.namespace, "snmp", popv1alpha1.PowerEventTypeFailure, msg); err != nil {
			m.logger.Error(err, "failed to create PowerEvent from SNMP")
		}
	} else if !wasGood && powerGood {
		msg := "SNMP: power restored"
		m.logger.Info(msg)
		if err := createPowerEvent(ctx, m.client, m.namespace, "snmp", popv1alpha1.PowerEventTypeRestored, msg); err != nil {
			m.logger.Error(err, "failed to create PowerEvent from SNMP")
		}
	}
}

func (m *SNMPMonitor) setStatus(connected bool, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := metav1.Now()
	m.status = popv1alpha1.EventSourceStatus{
		Type:      "snmp",
		Connected: connected,
		LastCheck: &now,
		Message:   message,
	}
}
