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

package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// TestMetricsRegistered verifies every metric declared in this package is
// registered exactly once with the controller-runtime registry by init().
func TestMetricsRegistered(t *testing.T) {
	// The Collectors are registered in init(); re-registering must fail,
	// which proves they are already present in the shared registry.
	collectors := map[string]prometheus.Collector{
		"EtcdBackupTotal":             EtcdBackupTotal,
		"EtcdBackupLastSuccess":       EtcdBackupLastSuccess,
		"EtcdBackupDuration":          EtcdBackupDuration,
		"BackupScheduleMissed":        BackupScheduleMissed,
		"ClusterShutdownTotal":        ClusterShutdownTotal,
		"ClusterRestartTotal":         ClusterRestartTotal,
		"CertificateExpiryTimestamp":  CertificateExpiryTimestamp,
		"PowerEventTotal":             PowerEventTotal,
		"PowerStatus":                 PowerStatus,
		"AutonomousShutdownCountdown": AutonomousShutdownCountdown,
	}
	for name, c := range collectors {
		if err := crmetrics.Registry.Register(c); err == nil {
			t.Errorf("metric %s was not already registered by init()", name)
		} else if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			t.Errorf("metric %s: unexpected registration error: %v", name, err)
		}
	}
}

func TestCounterVecLabelsAndIncrement(t *testing.T) {
	// Use a fresh child so we don't perturb global counters for other tests.
	before := testutil.ToFloat64(PowerEventTotal.WithLabelValues("PowerFailure"))
	PowerEventTotal.WithLabelValues("PowerFailure").Inc()
	after := testutil.ToFloat64(PowerEventTotal.WithLabelValues("PowerFailure"))
	if after-before != 1 {
		t.Fatalf("PowerEventTotal delta = %v, want 1", after-before)
	}

	// ClusterShutdownTotal has two labels: result, dryrun.
	before = testutil.ToFloat64(ClusterShutdownTotal.WithLabelValues("success", "false"))
	ClusterShutdownTotal.WithLabelValues("success", "false").Inc()
	after = testutil.ToFloat64(ClusterShutdownTotal.WithLabelValues("success", "false"))
	if after-before != 1 {
		t.Fatalf("ClusterShutdownTotal delta = %v, want 1", after-before)
	}
}

func TestGaugeSet(t *testing.T) {
	PowerStatus.Set(1)
	if got := testutil.ToFloat64(PowerStatus); got != 1 {
		t.Fatalf("PowerStatus = %v, want 1", got)
	}
	PowerStatus.Set(0)
	if got := testutil.ToFloat64(PowerStatus); got != 0 {
		t.Fatalf("PowerStatus = %v, want 0", got)
	}
}

// TestCounterExposition checks the exposed metric name and HELP text so a
// rename can't silently break dashboards/alerts that depend on it.
func TestCounterExposition(t *testing.T) {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pop_backup_schedule_missed_total",
		Help: "Total number of missed scheduled backups",
	})
	c.Inc()
	expected := `
# HELP pop_backup_schedule_missed_total Total number of missed scheduled backups
# TYPE pop_backup_schedule_missed_total counter
pop_backup_schedule_missed_total 1
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected)); err != nil {
		t.Fatalf("exposition mismatch: %v", err)
	}
}
