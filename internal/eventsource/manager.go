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

	mu       sync.Mutex
	monitors map[string]Monitor
	cancel   context.CancelFunc
}

func NewManager(client client.Client, namespace string, logger logr.Logger) *Manager {
	return &Manager{
		client:    client,
		namespace: namespace,
		logger:    logger,
		monitors:  make(map[string]Monitor),
	}
}

func (m *Manager) Reconcile(config *popv1alpha1.EventSourcesConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	desired := make(map[string]bool)

	if config == nil {
		m.stopAll()
		return
	}

	if config.SNMP != nil && config.SNMP.Enabled {
		desired["snmp"] = true
		if _, ok := m.monitors["snmp"]; !ok {
			mon := NewSNMPMonitor(m.client, m.namespace, config.SNMP, m.logger.WithName("snmp"))
			m.monitors["snmp"] = mon
			go func() { _ = mon.Start(ctx) }()
		}
	}

	if config.IPMI != nil && config.IPMI.Enabled {
		desired["ipmi"] = true
		if _, ok := m.monitors["ipmi"]; !ok {
			mon := NewIPMIMonitor(m.client, m.namespace, config.IPMI, m.logger.WithName("ipmi"))
			m.monitors["ipmi"] = mon
			go func() { _ = mon.Start(ctx) }()
		}
	}

	if config.NUT != nil && config.NUT.Enabled {
		desired["nut"] = true
		if _, ok := m.monitors["nut"]; !ok {
			mon := NewNUTMonitor(m.client, m.namespace, config.NUT, m.logger.WithName("nut"))
			m.monitors["nut"] = mon
			go func() { _ = mon.Start(ctx) }()
		}
	}

	for name, mon := range m.monitors {
		if !desired[name] {
			mon.Stop()
			delete(m.monitors, name)
		}
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
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Manager) stopAll() {
	for name, mon := range m.monitors {
		mon.Stop()
		delete(m.monitors, name)
	}
}

func createPowerEvent(ctx context.Context, k8sClient client.Client, namespace string, source string, eventType popv1alpha1.PowerEventType, message string) error {
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
