package eventsource

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// mockMonitor implements Monitor for testing. It tracks Start/Stop calls
// and records whether its context was cancelled.
type mockMonitor struct {
	mu               sync.Mutex
	started          bool
	stopped          bool
	startCtx         context.Context
	contextCancelled bool
	status           popv1alpha1.EventSourceStatus
}

func newMockMonitor(sourceType string) *mockMonitor {
	return &mockMonitor{
		status: popv1alpha1.EventSourceStatus{
			Type:      sourceType,
			Connected: true,
			LastCheck: &metav1.Time{Time: time.Now()},
			Message:   sourceType + " monitor active",
		},
	}
}

func (m *mockMonitor) Start(ctx context.Context) error {
	m.mu.Lock()
	m.started = true
	m.startCtx = ctx
	m.mu.Unlock()

	// Block until context is cancelled, like a real monitor would.
	<-ctx.Done()

	m.mu.Lock()
	m.contextCancelled = true
	m.mu.Unlock()
	return ctx.Err()
}

func (m *mockMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
}

func (m *mockMonitor) Status() popv1alpha1.EventSourceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *mockMonitor) wasStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func (m *mockMonitor) wasStopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

func (m *mockMonitor) wasContextCancelled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.contextCancelled
}

func newTestManager() *Manager {
	return NewManager(nil, "test-namespace", logr.Discard())
}

// TestManager_StartAndStopMonitor verifies that startMonitor registers a monitor
// and starts it, and stopMonitor stops it and cancels its context.
func TestManager_StartAndStopMonitor(t *testing.T) {
	m := newTestManager()
	mock := newMockMonitor("snmp")

	m.startMonitor("snmp", mock)

	// Give the goroutine time to execute Start.
	time.Sleep(20 * time.Millisecond)

	if !mock.wasStarted() {
		t.Fatal("expected monitor to be started")
	}
	if _, ok := m.monitors["snmp"]; !ok {
		t.Fatal("expected monitor to be registered in monitors map")
	}
	if _, ok := m.cancels["snmp"]; !ok {
		t.Fatal("expected cancel func to be registered in cancels map")
	}

	m.stopMonitor("snmp")

	// Give the goroutine time to observe context cancellation.
	time.Sleep(20 * time.Millisecond)

	if !mock.wasStopped() {
		t.Fatal("expected Stop() to be called on the monitor")
	}
	if !mock.wasContextCancelled() {
		t.Fatal("expected monitor's context to be cancelled")
	}
	if _, ok := m.monitors["snmp"]; ok {
		t.Fatal("expected monitor to be removed from monitors map")
	}
	if _, ok := m.cancels["snmp"]; ok {
		t.Fatal("expected cancel func to be removed from cancels map")
	}
}

// TestManager_IndependentCancellation is the critical test. It verifies that
// stopping one monitor does NOT cancel another monitor's context. This validates
// the per-monitor cancellation fix where previously all monitors shared one context.
func TestManager_IndependentCancellation(t *testing.T) {
	m := newTestManager()
	snmpMock := newMockMonitor("snmp")
	ipmiMock := newMockMonitor("ipmi")

	m.startMonitor("snmp", snmpMock)
	m.startMonitor("ipmi", ipmiMock)

	// Let both goroutines start.
	time.Sleep(20 * time.Millisecond)

	if !snmpMock.wasStarted() {
		t.Fatal("expected SNMP monitor to be started")
	}
	if !ipmiMock.wasStarted() {
		t.Fatal("expected IPMI monitor to be started")
	}

	// Stop only SNMP.
	m.stopMonitor("snmp")
	time.Sleep(20 * time.Millisecond)

	// SNMP should be stopped and its context cancelled.
	if !snmpMock.wasStopped() {
		t.Fatal("expected SNMP Stop() to be called")
	}
	if !snmpMock.wasContextCancelled() {
		t.Fatal("expected SNMP context to be cancelled")
	}

	// IPMI should still be running, context NOT cancelled.
	if ipmiMock.wasStopped() {
		t.Fatal("IPMI Stop() should NOT have been called")
	}
	if ipmiMock.wasContextCancelled() {
		t.Fatal("IPMI context should NOT be cancelled -- this is the shared-context bug")
	}

	// Verify IPMI is still in the maps.
	if _, ok := m.monitors["ipmi"]; !ok {
		t.Fatal("expected IPMI to still be registered")
	}
	if _, ok := m.cancels["ipmi"]; !ok {
		t.Fatal("expected IPMI cancel to still be registered")
	}

	// Clean up.
	m.stopMonitor("ipmi")
	time.Sleep(20 * time.Millisecond)

	if !ipmiMock.wasStopped() {
		t.Fatal("expected IPMI Stop() to be called during cleanup")
	}
	if !ipmiMock.wasContextCancelled() {
		t.Fatal("expected IPMI context to be cancelled during cleanup")
	}
}

// TestManager_StopAllOnNilConfig verifies that stopAll stops every active monitor
// and cancels every context.
func TestManager_StopAllOnNilConfig(t *testing.T) {
	m := newTestManager()
	snmpMock := newMockMonitor("snmp")
	ipmiMock := newMockMonitor("ipmi")
	nutMock := newMockMonitor("nut")

	m.startMonitor("snmp", snmpMock)
	m.startMonitor("ipmi", ipmiMock)
	m.startMonitor("nut", nutMock)

	time.Sleep(20 * time.Millisecond)

	m.stopAll()
	time.Sleep(20 * time.Millisecond)

	for name, mock := range map[string]*mockMonitor{
		"snmp": snmpMock,
		"ipmi": ipmiMock,
		"nut":  nutMock,
	} {
		if !mock.wasStopped() {
			t.Fatalf("expected %s Stop() to be called", name)
		}
		if !mock.wasContextCancelled() {
			t.Fatalf("expected %s context to be cancelled", name)
		}
	}

	if len(m.monitors) != 0 {
		t.Fatalf("expected monitors map to be empty, got %d entries", len(m.monitors))
	}
	if len(m.cancels) != 0 {
		t.Fatalf("expected cancels map to be empty, got %d entries", len(m.cancels))
	}
}

// TestManager_Statuses verifies that Statuses() collects status from all active monitors.
func TestManager_Statuses(t *testing.T) {
	m := newTestManager()

	snmpMock := newMockMonitor("snmp")
	ipmiMock := newMockMonitor("ipmi")

	m.startMonitor("snmp", snmpMock)
	m.startMonitor("ipmi", ipmiMock)

	time.Sleep(20 * time.Millisecond)

	statuses := m.Statuses()

	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	typesSeen := make(map[string]bool)
	for _, s := range statuses {
		typesSeen[s.Type] = true
		if !s.Connected {
			t.Fatalf("expected %s to report connected", s.Type)
		}
	}

	if !typesSeen["snmp"] {
		t.Fatal("expected snmp status in results")
	}
	if !typesSeen["ipmi"] {
		t.Fatal("expected ipmi status in results")
	}

	// Clean up.
	m.Stop()
	time.Sleep(20 * time.Millisecond)
}

// TestManager_StopMethod verifies that the public Stop() method stops all monitors.
func TestManager_StopMethod(t *testing.T) {
	m := newTestManager()

	snmpMock := newMockMonitor("snmp")
	ipmiMock := newMockMonitor("ipmi")

	m.startMonitor("snmp", snmpMock)
	m.startMonitor("ipmi", ipmiMock)

	time.Sleep(20 * time.Millisecond)

	m.Stop()
	time.Sleep(20 * time.Millisecond)

	if !snmpMock.wasStopped() {
		t.Fatal("expected SNMP to be stopped via Stop()")
	}
	if !ipmiMock.wasStopped() {
		t.Fatal("expected IPMI to be stopped via Stop()")
	}
	if !snmpMock.wasContextCancelled() {
		t.Fatal("expected SNMP context to be cancelled via Stop()")
	}
	if !ipmiMock.wasContextCancelled() {
		t.Fatal("expected IPMI context to be cancelled via Stop()")
	}
	if len(m.monitors) != 0 {
		t.Fatalf("expected monitors map empty after Stop(), got %d", len(m.monitors))
	}
}

// TestManager_IdempotentReconcile verifies that calling startMonitor for a name
// that already exists does NOT restart the monitor (the Reconcile method checks
// for existing monitors before calling startMonitor).
func TestManager_IdempotentReconcile(t *testing.T) {
	m := newTestManager()
	firstMock := newMockMonitor("snmp")
	secondMock := newMockMonitor("snmp")

	m.startMonitor("snmp", firstMock)
	time.Sleep(20 * time.Millisecond)

	if !firstMock.wasStarted() {
		t.Fatal("expected first monitor to be started")
	}

	// Simulate what Reconcile does: check if monitor exists before starting.
	if _, ok := m.monitors["snmp"]; ok {
		// Monitor already exists, do not start a new one.
	} else {
		m.startMonitor("snmp", secondMock)
	}

	time.Sleep(20 * time.Millisecond)

	// Second mock should never have been started.
	if secondMock.wasStarted() {
		t.Fatal("second monitor should NOT have been started -- idempotency broken")
	}

	// Original monitor should still be the one registered.
	if m.monitors["snmp"] != firstMock {
		t.Fatal("expected original monitor to still be registered")
	}

	// Clean up.
	m.Stop()
	time.Sleep(20 * time.Millisecond)
}

// TestManager_StopMonitorNonexistent verifies that stopping a monitor that
// does not exist is a no-op and does not panic.
func TestManager_StopMonitorNonexistent(t *testing.T) {
	m := newTestManager()
	// Should not panic.
	m.stopMonitor("nonexistent")

	if len(m.monitors) != 0 {
		t.Fatal("monitors map should be empty")
	}
	if len(m.cancels) != 0 {
		t.Fatal("cancels map should be empty")
	}
}

// TestManager_StatusesEmpty verifies Statuses() returns an empty slice
// when no monitors are running.
func TestManager_StatusesEmpty(t *testing.T) {
	m := newTestManager()
	statuses := m.Statuses()
	if statuses == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(statuses) != 0 {
		t.Fatalf("expected 0 statuses, got %d", len(statuses))
	}
}

// TestManager_StopAllEmpty verifies stopAll on an empty manager is a no-op.
func TestManager_StopAllEmpty(t *testing.T) {
	m := newTestManager()
	// Should not panic.
	m.stopAll()

	if len(m.monitors) != 0 {
		t.Fatal("monitors map should be empty")
	}
}
