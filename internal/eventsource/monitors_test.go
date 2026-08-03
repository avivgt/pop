package eventsource

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

const esNamespace = "pop-system"

func esScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(popv1alpha1.AddToScheme(s))
	return s
}

func esClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(esScheme(t)).WithObjects(objs...).Build()
}

func credSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: esNamespace},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("s3cret"),
		},
	}
}

func listEvents(t *testing.T, c client.Client) []popv1alpha1.PowerEvent {
	t.Helper()
	l := &popv1alpha1.PowerEventList{}
	if err := c.List(context.Background(), l, client.InNamespace(esNamespace)); err != nil {
		t.Fatalf("list PowerEvents: %v", err)
	}
	return l.Items
}

func countByType(events []popv1alpha1.PowerEvent, et popv1alpha1.PowerEventType) int {
	n := 0
	for i := range events {
		if events[i].Spec.EventType == et {
			n++
		}
	}
	return n
}

// --- NUT ---------------------------------------------------------------------

// fakeNUT is a minimal TCP server that speaks just enough of the NUT protocol
// for NUTMonitor.queryNUT: it reads until it sees a "GET VAR" line and replies
// with whatever line the test has configured.
type fakeNUT struct {
	ln          net.Listener
	mu          sync.Mutex
	response    string
	closeNoData bool
}

func startFakeNUT(t *testing.T, response string) *fakeNUT {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeNUT{ln: ln, response: response}
	go f.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

func (f *fakeNUT) setResponse(resp string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.response = resp
}

func (f *fakeNUT) host() string { return "127.0.0.1" }
func (f *fakeNUT) port() int32  { return int32(f.ln.Addr().(*net.TCPAddr).Port) }

func (f *fakeNUT) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeNUT) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	f.mu.Lock()
	closeNoData := f.closeNoData
	f.mu.Unlock()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "GET VAR") {
			if closeNoData {
				return
			}
			f.mu.Lock()
			resp := f.response
			f.mu.Unlock()
			_, _ = conn.Write([]byte(resp + "\n"))
			return
		}
	}
}

func TestNUTQueryParsing(t *testing.T) {
	cases := []struct {
		name     string
		response string
		want     string
		wantErr  bool
	}{
		{"online", `VAR ups ups.status "OL"`, "OL", false},
		{"on battery", `VAR ups ups.status "OB DISCHRG"`, "OB DISCHRG", false},
		{"server error", "ERR UNKNOWN-UPS", "", true},
		{"garbage", "not a var line", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := startFakeNUT(t, tc.response)
			m := NewNUTMonitor(esClient(t), esNamespace,
				&popv1alpha1.NUTSourceConfig{Host: f.host(), Port: f.port(), UPSName: "ups"},
				logr.Discard())

			got, err := m.queryNUT(context.Background(), f.port())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got status %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNUTQueryConnectionRefused(t *testing.T) {
	// Point at a closed port to exercise the dial error path.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := int32(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()

	m := NewNUTMonitor(esClient(t), esNamespace,
		&popv1alpha1.NUTSourceConfig{Host: "127.0.0.1", Port: port, UPSName: "ups"},
		logr.Discard())
	if _, err := m.queryNUT(context.Background(), port); err == nil {
		t.Fatal("expected dial error against closed port")
	}
}

func TestNUTPollTransitions(t *testing.T) {
	f := startFakeNUT(t, `VAR ups ups.status "OL"`)
	c := esClient(t)
	m := NewNUTMonitor(c, esNamespace,
		&popv1alpha1.NUTSourceConfig{Host: f.host(), Port: f.port(), UPSName: "ups"},
		logr.Discard())

	// Start online -> on battery: expect a failure PowerEvent.
	f.setResponse(`VAR ups ups.status "OB DISCHRG"`)
	m.poll(context.Background())
	if got := countByType(listEvents(t, c), popv1alpha1.PowerEventTypeFailure); got != 1 {
		t.Fatalf("after power failure: %d failure events, want 1", got)
	}
	if st := m.Status(); !st.Connected {
		t.Errorf("status should be connected after successful poll")
	}

	// Back online: expect a restored PowerEvent.
	f.setResponse(`VAR ups ups.status "OL"`)
	m.poll(context.Background())
	if got := countByType(listEvents(t, c), popv1alpha1.PowerEventTypeRestored); got != 1 {
		t.Fatalf("after restore: %d restored events, want 1", got)
	}
}

func TestNUTPollQueryFailureSetsDisconnected(t *testing.T) {
	m := NewNUTMonitor(esClient(t), esNamespace,
		&popv1alpha1.NUTSourceConfig{Host: "127.0.0.1", Port: 1, UPSName: "ups"},
		logr.Discard())
	m.poll(context.Background())
	st := m.Status()
	if st.Connected {
		t.Fatal("status should be disconnected when NUT query fails")
	}
	if !strings.Contains(st.Message, "NUT query failed") {
		t.Errorf("message = %q, want it to mention query failure", st.Message)
	}
}

func TestNUTGetCredentials(t *testing.T) {
	c := esClient(t, credSecret("nut-creds"))
	m := NewNUTMonitor(c, esNamespace,
		&popv1alpha1.NUTSourceConfig{Host: "h", AuthSecret: "nut-creds"}, logr.Discard())
	user, pass, err := m.getCredentials(context.Background())
	if err != nil {
		t.Fatalf("getCredentials: %v", err)
	}
	if user != "admin" || pass != "s3cret" {
		t.Fatalf("creds = %q/%q, want admin/s3cret", user, pass)
	}
}

func TestNUTConstructorDefaultsAndStop(t *testing.T) {
	m := NewNUTMonitor(esClient(t), esNamespace, &popv1alpha1.NUTSourceConfig{}, logr.Discard())
	st := m.Status()
	if st.Type != "nut" || st.Message != "initializing" {
		t.Fatalf("initial status = %+v, want type=nut message=initializing", st)
	}
	// Stop before Start must not panic (cancel is nil).
	m.Stop()
}

// --- IPMI / Redfish ----------------------------------------------------------

func redfishBody(t *testing.T, supplies ...[2]string) string {
	t.Helper()
	var resp redfishPowerResponse
	for _, s := range supplies {
		ps := struct {
			Status struct {
				Health string `json:"Health"`
				State  string `json:"State"`
			} `json:"Status"`
		}{}
		ps.Status.Health = s[0]
		ps.Status.State = s[1]
		resp.PowerSupplies = append(resp.PowerSupplies, ps)
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal redfish body: %v", err)
	}
	return string(b)
}

// ipmiMonitorFor wires an IPMIMonitor to a test HTTPS server.
func ipmiMonitorFor(t *testing.T, ts *httptest.Server, c client.Client) *IPMIMonitor {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test url: %v", err)
	}
	p, _ := strconv.Atoi(u.Port())
	return NewIPMIMonitor(c, esNamespace, &popv1alpha1.IPMISourceConfig{
		Host:        u.Hostname(),
		Port:        int32(p),
		AuthSecret:  "bmc-creds",
		TLSInsecure: true,
	}, logr.Discard())
}

func TestIPMIPollTransitions(t *testing.T) {
	var mu sync.Mutex
	body := redfishBody(t, [2]string{"OK", "Enabled"}, [2]string{"OK", "Enabled"})
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redfish/v1/Chassis/1/Power" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)

	c := esClient(t, credSecret("bmc-creds"))
	m := ipmiMonitorFor(t, ts, c)

	// Degrade a supply -> failure event.
	mu.Lock()
	body = redfishBody(t, [2]string{"Critical", "Enabled"}, [2]string{"OK", "Enabled"})
	mu.Unlock()
	m.poll(context.Background())
	if got := countByType(listEvents(t, c), popv1alpha1.PowerEventTypeFailure); got != 1 {
		t.Fatalf("after degrade: %d failure events, want 1", got)
	}
	if !m.Status().Connected {
		t.Errorf("status should be connected after a 200 response")
	}

	// Recover -> restored event.
	mu.Lock()
	body = redfishBody(t, [2]string{"OK", "Enabled"}, [2]string{"OK", "Enabled"})
	mu.Unlock()
	m.poll(context.Background())
	if got := countByType(listEvents(t, c), popv1alpha1.PowerEventTypeRestored); got != 1 {
		t.Fatalf("after recover: %d restored events, want 1", got)
	}
}

func TestIPMIPollHTTPError(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	c := esClient(t, credSecret("bmc-creds"))
	m := ipmiMonitorFor(t, ts, c)
	m.poll(context.Background())

	st := m.Status()
	if st.Connected {
		t.Fatal("status should be disconnected on HTTP 500")
	}
	if !strings.Contains(st.Message, "HTTP 500") {
		t.Errorf("message = %q, want it to mention HTTP 500", st.Message)
	}
	if len(listEvents(t, c)) != 0 {
		t.Errorf("no PowerEvents expected on HTTP error")
	}
}

func TestIPMIPollBadJSON(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	t.Cleanup(ts.Close)

	c := esClient(t, credSecret("bmc-creds"))
	m := ipmiMonitorFor(t, ts, c)
	m.poll(context.Background())

	if m.Status().Connected {
		t.Fatal("status should be disconnected when body is not valid JSON")
	}
}

func TestIPMIMissingCredentials(t *testing.T) {
	// No secret in the cluster -> getCredentials fails, poll reports it.
	m := NewIPMIMonitor(esClient(t), esNamespace,
		&popv1alpha1.IPMISourceConfig{Host: "bmc", AuthSecret: "does-not-exist"}, logr.Discard())
	m.poll(context.Background())
	if m.Status().Connected {
		t.Fatal("status should be disconnected when the secret is missing")
	}

	// Empty AuthSecret is an explicit error from getCredentials.
	m2 := NewIPMIMonitor(esClient(t), esNamespace,
		&popv1alpha1.IPMISourceConfig{Host: "bmc"}, logr.Discard())
	if _, _, err := m2.getCredentials(context.Background()); err == nil {
		t.Fatal("expected error when AuthSecret is empty")
	}
}

func TestIPMIConstructorDefaultsAndStop(t *testing.T) {
	m := NewIPMIMonitor(esClient(t), esNamespace, &popv1alpha1.IPMISourceConfig{}, logr.Discard())
	if st := m.Status(); st.Type != "ipmi" || st.Message != "initializing" {
		t.Fatalf("initial status = %+v", st)
	}
	m.Stop()
}

// --- SNMP --------------------------------------------------------------------

func TestSNMPConstructorAndStatus(t *testing.T) {
	m := NewSNMPMonitor(esClient(t), esNamespace, &popv1alpha1.SNMPSourceConfig{}, logr.Discard())
	st := m.Status()
	if st.Type != "snmp" || st.Message != "initializing" {
		t.Fatalf("initial status = %+v", st)
	}

	m.setStatus(true, "polling")
	st = m.Status()
	if !st.Connected || st.Message != "polling" || st.LastCheck == nil {
		t.Fatalf("setStatus did not update status: %+v", st)
	}

	// Stop before Start must not panic.
	m.Stop()
}

func TestSNMPPollConnectFailure(t *testing.T) {
	// Empty host makes gosnmp.Connect fail fast, exercising poll's error path
	// without needing a live SNMP agent.
	m := NewSNMPMonitor(esClient(t), esNamespace,
		&popv1alpha1.SNMPSourceConfig{Host: ""}, logr.Discard())
	m.poll(context.Background())
	if m.Status().Connected {
		t.Fatal("status should be disconnected when SNMP connect fails")
	}
}

// --- createPowerEvent + Reconcile -------------------------------------------

func TestCreatePowerEventCreates(t *testing.T) {
	c := esClient(t)
	if err := createPowerEvent(context.Background(), c, esNamespace, "snmp",
		popv1alpha1.PowerEventTypeFailure, "boom"); err != nil {
		t.Fatalf("createPowerEvent: %v", err)
	}
	events := listEvents(t, c)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Spec.EventType != popv1alpha1.PowerEventTypeFailure ||
		e.Spec.Source != "snmp" || e.Spec.TriggeredBy != "snmp" || e.Spec.Message != "boom" {
		t.Fatalf("unexpected event spec: %+v", e.Spec)
	}
}

func TestCreatePowerEventDedupesActiveFailure(t *testing.T) {
	active := &popv1alpha1.PowerEvent{
		ObjectMeta: metav1.ObjectMeta{Name: "active-fail", Namespace: esNamespace},
		Spec:       popv1alpha1.PowerEventSpec{EventType: popv1alpha1.PowerEventTypeFailure},
		Status:     popv1alpha1.PowerEventStatus{Phase: popv1alpha1.PowerEventPhaseDetected},
	}
	c := esClient(t, active)

	// A second failure while one is active must be suppressed.
	if err := createPowerEvent(context.Background(), c, esNamespace, "snmp",
		popv1alpha1.PowerEventTypeFailure, "again"); err != nil {
		t.Fatalf("createPowerEvent: %v", err)
	}
	if got := countByType(listEvents(t, c), popv1alpha1.PowerEventTypeFailure); got != 1 {
		t.Fatalf("expected dedup to 1 failure event, got %d", got)
	}

	// A restored event is never deduped.
	if err := createPowerEvent(context.Background(), c, esNamespace, "snmp",
		popv1alpha1.PowerEventTypeRestored, "ok"); err != nil {
		t.Fatalf("createPowerEvent restored: %v", err)
	}
	if got := countByType(listEvents(t, c), popv1alpha1.PowerEventTypeRestored); got != 1 {
		t.Fatalf("expected 1 restored event, got %d", got)
	}
}

func TestManagerReconcileStartsAndStopsMonitors(t *testing.T) {
	mgr := NewManager(esClient(t), esNamespace, logr.Discard())

	// Enable all three sources.
	cfg := &popv1alpha1.EventSourcesConfig{
		SNMP: &popv1alpha1.SNMPSourceConfig{Enabled: true, Host: "snmp-host"},
		IPMI: &popv1alpha1.IPMISourceConfig{Enabled: true, Host: "bmc", AuthSecret: "x"},
		NUT:  &popv1alpha1.NUTSourceConfig{Enabled: true, Host: "nut-host", UPSName: "ups"},
	}
	mgr.Reconcile(cfg)
	if got := len(mgr.Statuses()); got != 3 {
		t.Fatalf("expected 3 monitors after enabling all, got %d", got)
	}

	// Disable IPMI; the manager should tear just that monitor down.
	cfg.IPMI.Enabled = false
	mgr.Reconcile(cfg)
	if got := len(mgr.Statuses()); got != 2 {
		t.Fatalf("expected 2 monitors after disabling IPMI, got %d", got)
	}

	// A nil config stops everything.
	mgr.Reconcile(nil)
	if got := len(mgr.Statuses()); got != 0 {
		t.Fatalf("expected 0 monitors after nil config, got %d", got)
	}

	mgr.Stop()
}

// Guard against slow tests: monitor pollers use >=5s tickers, so a Reconcile
// followed immediately by Stop must not block.
func TestManagerReconcileIsNonBlocking(t *testing.T) {
	done := make(chan struct{})
	go func() {
		mgr := NewManager(esClient(t), esNamespace, logr.Discard())
		mgr.Reconcile(&popv1alpha1.EventSourcesConfig{
			NUT: &popv1alpha1.NUTSourceConfig{Enabled: true, Host: "h", UPSName: "ups"},
		})
		mgr.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Reconcile+Stop blocked unexpectedly")
	}
}
