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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	EtcdBackupTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pop_etcd_backup_total",
			Help: "Total number of etcd backups attempted",
		},
		[]string{"result"},
	)

	EtcdBackupLastSuccess = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "pop_etcd_backup_last_success_timestamp",
			Help: "Unix timestamp of the last successful etcd backup",
		},
	)

	EtcdBackupDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "pop_etcd_backup_duration_seconds",
			Help:    "Duration of etcd backup operations in seconds",
			Buckets: prometheus.ExponentialBuckets(10, 2, 8),
		},
	)

	BackupScheduleMissed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pop_backup_schedule_missed_total",
			Help: "Total number of missed scheduled backups",
		},
	)

	ClusterShutdownTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pop_cluster_shutdown_total",
			Help: "Total number of cluster shutdown operations",
		},
		[]string{"result", "dryrun"},
	)

	ClusterRestartTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pop_cluster_restart_total",
			Help: "Total number of cluster restart operations",
		},
		[]string{"result"},
	)

	CertificateExpiryTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "pop_certificate_expiry_timestamp",
			Help: "Unix timestamp when the kube-apiserver-to-kubelet-signer certificate expires",
		},
	)

	PowerEventTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pop_power_event_total",
			Help: "Total number of power events received",
		},
		[]string{"type"},
	)

	PowerStatus = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "pop_power_status",
			Help: "Current power status (0=normal, 1=on_battery)",
		},
	)

	AutonomousShutdownCountdown = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "pop_autonomous_shutdown_countdown_seconds",
			Help: "Seconds until autonomous shutdown triggers (0 when not active)",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		EtcdBackupTotal,
		EtcdBackupLastSuccess,
		EtcdBackupDuration,
		BackupScheduleMissed,
		ClusterShutdownTotal,
		ClusterRestartTotal,
		CertificateExpiryTimestamp,
		PowerEventTotal,
		PowerStatus,
		AutonomousShutdownCountdown,
	)
}
