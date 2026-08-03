package eventsource

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

type Monitor interface {
	Start(ctx context.Context) error
	Stop()
	Status() popv1alpha1.EventSourceStatus
}

type Manager struct {
	client    client.Client
	namespace string
	logger    logr.Logger

	mu         sync.Mutex
	monitors   map[string]Monitor
	cancels    map[string]context.CancelFunc
	lastConfig *popv1alpha1.EventSourcesConfig
}

func NewManager(client client.Client, namespace string, logger logr.Logger) *Manager {
	return &Manager{
		client:    client,
		namespace: namespace,
		logger:    logger,
		monitors:  make(map[string]Monitor),
		cancels:   make(map[string]context.CancelFunc),
	}
}

func (m *Manager) Reconcile(config *popv1alpha1.EventSourcesConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if config == nil {
		m.stopAll()
		m.lastConfig = nil
		return
	}

	desired := make(map[string]bool)

	if config.SNMP != nil && config.SNMP.Enabled {
		desired["snmp"] = true
		if _, ok := m.monitors["snmp"]; !ok {
			m.startMonitor("snmp", NewSNMPMonitor(m.client, m.namespace, config.SNMP, m.logger.WithName("snmp")))
		}
	}

	if config.IPMI != nil && config.IPMI.Enabled {
		desired["ipmi"] = true
		if _, ok := m.monitors["ipmi"]; !ok {
			m.startMonitor("ipmi", NewIPMIMonitor(m.client, m.namespace, config.IPMI, m.logger.WithName("ipmi")))
		}
	}

	if config.NUT != nil && config.NUT.Enabled {
		desired["nut"] = true
		if _, ok := m.monitors["nut"]; !ok {
			m.startMonitor("nut", NewNUTMonitor(m.client, m.namespace, config.NUT, m.logger.WithName("nut")))
		}
	}

	for name := range m.monitors {
		if !desired[name] {
			m.stopMonitor(name)
		}
	}

	m.lastConfig = config
}

func (m *Manager) startMonitor(name string, mon Monitor) {
	ctx, cancel := context.WithCancel(context.Background())
	m.monitors[name] = mon
	m.cancels[name] = cancel
	go func() { _ = mon.Start(ctx) }()
}

func (m *Manager) stopMonitor(name string) {
	if mon, ok := m.monitors[name]; ok {
		mon.Stop()
		delete(m.monitors, name)
	}
	if cancel, ok := m.cancels[name]; ok {
		cancel()
		delete(m.cancels, name)
	}
}

func (m *Manager) Statuses() []popv1alpha1.EventSourceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	statuses := make([]popv1alpha1.EventSourceStatus, 0, len(m.monitors))
	for _, mon := range m.monitors {
		statuses = append(statuses, mon.Status())
	}
	return statuses
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopAll()
}

func (m *Manager) stopAll() {
	for name := range m.monitors {
		m.stopMonitor(name)
	}
}

func createPowerEvent(ctx context.Context, k8sClient client.Client, namespace string, source string, eventType popv1alpha1.PowerEventType, message string) error {
	if eventType == popv1alpha1.PowerEventTypeFailure {
		existing := &popv1alpha1.PowerEventList{}
		if err := k8sClient.List(ctx, existing, client.InNamespace(namespace)); err == nil {
			for i := range existing.Items {
				e := &existing.Items[i]
				if e.Spec.EventType != popv1alpha1.PowerEventTypeFailure {
					continue
				}
				switch e.Status.Phase {
				case popv1alpha1.PowerEventPhaseDetected,
					popv1alpha1.PowerEventPhaseBackingUp,
					popv1alpha1.PowerEventPhaseAwaitingConfirmation,
					popv1alpha1.PowerEventPhaseConfirmed,
					popv1alpha1.PowerEventPhaseShuttingDown:
					return nil
				}
			}
		}
	}

	event := &popv1alpha1.PowerEvent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: source + "-",
			Namespace:    namespace,
		},
		Spec: popv1alpha1.PowerEventSpec{
			EventType:   eventType,
			Source:      source,
			TriggeredBy: source,
			Message:     message,
		},
	}
	return k8sClient.Create(ctx, event)
}
