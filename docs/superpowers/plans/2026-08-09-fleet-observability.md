# Fleet Observability (Phase 2a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the ACM hub fleet-wide visibility and alerting for POP — federate POP's spoke metrics to the hub, alert on them, alert on cluster liveness, and ship a fleet dashboard — using only supported ACM Observability + platform-monitoring mechanisms.

**Architecture:** Two hub observability stacks (verified by the 2026-08-09 spike). (1) MCO-federated: POP's `pop_*` spoke metrics via the `observability-metrics-custom-allowlist` ConfigMap → alerts via `thanos-ruler-custom-rules` → MCO Grafana/Alertmanager. (2) Hub platform-monitoring: fleet liveness via the hub-native `acm_managed_cluster_status_condition` metric → a `PrometheusRule` in `openshift-monitoring`. These cannot be joined in one PromQL expression.

**Tech Stack:** YAML config (ConfigMaps, PrometheusRule, Grafana dashboard JSON). No Go. Validation via `promtool` and `oc apply --dry-run=server`.

**POP metric names (from `internal/metrics/metrics.go`):** `pop_power_status`, `pop_autonomous_shutdown_countdown_seconds`, `pop_certificate_expiry_timestamp`, `pop_etcd_backup_last_success_timestamp`, `pop_backup_schedule_missed_total`, `pop_etcd_backup_total`, `pop_cluster_shutdown_total`, `pop_cluster_restart_total`, `pop_power_event_total`.

---

## Prerequisites (document, do not automate here)

- ACM Observability (MCO) installed on the hub (`MultiClusterObservability` CR present); managed clusters have the observability addon enabled.
- These artifacts are hub-cluster config; deliver them via the same GitOps/ApplicationSet path used for the rest of the hub config. This plan produces the manifests; wiring them into GitOps is a one-line kustomization add per file.

---

## File structure

- `config/observability/metrics-allowlist.yaml` — MCO custom allowlist ConfigMap (federate `pop_*`). (Create)
- `config/observability/thanos-ruler-custom-rules.yaml` — MCO custom alerting rules over federated POP metrics. (Create)
- `config/observability/fleet-liveness-prometheusrule.yaml` — hub platform-monitoring PrometheusRule for "dark N days". (Create)
- `config/observability/fleet-dashboard-configmap.yaml` — Grafana dashboard ConfigMap. (Create)
- `config/observability/kustomization.yaml` — kustomize these four. (Create)

---

## Task 1: Federate POP metrics to the hub (MCO allowlist)

**Files:**
- Create: `config/observability/metrics-allowlist.yaml`

- [ ] **Step 1: Write the allowlist ConfigMap**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: observability-metrics-custom-allowlist
  namespace: open-cluster-management-observability
data:
  metrics_list.yaml: |
    names:
      - pop_power_status
      - pop_autonomous_shutdown_countdown_seconds
      - pop_certificate_expiry_timestamp
      - pop_etcd_backup_last_success_timestamp
      - pop_backup_schedule_missed_total
      - pop_cluster_shutdown_total
      - pop_cluster_restart_total
      - pop_power_event_total
```

- [ ] **Step 2: Validate the YAML server-side**

Run (against a hub, or skip if no hub yet): `oc apply --dry-run=server -f config/observability/metrics-allowlist.yaml`
Expected: `configmap/observability-metrics-custom-allowlist configured (server dry run)`. If no hub is available, validate locally: `oc apply --dry-run=client -f config/observability/metrics-allowlist.yaml` → no schema errors.

- [ ] **Step 3: Commit**

```bash
git add config/observability/metrics-allowlist.yaml
git commit -m "feat(observability): federate pop_* metrics to hub via MCO allowlist"
```

## Task 2: Alerts over federated POP metrics (thanos-ruler-custom-rules)

**Files:**
- Create: `config/observability/thanos-ruler-custom-rules.yaml`

- [ ] **Step 1: Write the custom rules ConfigMap**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: thanos-ruler-custom-rules
  namespace: open-cluster-management-observability
data:
  custom_rules.yaml: |
    groups:
      - name: pop-fleet-power
        rules:
          - alert: PopFleetSiteOnBattery
            expr: pop_power_status == 1
            for: 1m
            labels:
              severity: critical
            annotations:
              summary: "POP: site {{ $labels.cluster }} is on battery"
              description: "pop_power_status=1 on cluster {{ $labels.cluster }}."
          - alert: PopFleetBackupStale
            expr: (time() - pop_etcd_backup_last_success_timestamp) > 86400
            for: 10m
            labels:
              severity: warning
            annotations:
              summary: "POP: etcd backup stale on {{ $labels.cluster }}"
              description: "No successful etcd backup in >24h on cluster {{ $labels.cluster }}."
          - alert: PopFleetShutdownInProgress
            expr: pop_cluster_shutdown_total > 0 and pop_autonomous_shutdown_countdown_seconds > 0
            for: 0m
            labels:
              severity: warning
            annotations:
              summary: "POP: autonomous shutdown counting down on {{ $labels.cluster }}"
              description: "Autonomous shutdown countdown active on cluster {{ $labels.cluster }}."
```

- [ ] **Step 2: Validate rule syntax**

Extract and check with promtool:
Run: `python3 -c "import yaml,sys; print(yaml.safe_load(open('config/observability/thanos-ruler-custom-rules.yaml'))['data']['custom_rules.yaml'])" > /tmp/pop-rules.yaml && promtool check rules /tmp/pop-rules.yaml`
Expected: `SUCCESS: 3 rules found`. (If `promtool` is absent, install via `go install github.com/prometheus/prometheus/cmd/promtool@latest` or skip with a note.)

- [ ] **Step 3: Commit**

```bash
git add config/observability/thanos-ruler-custom-rules.yaml
git commit -m "feat(observability): hub alerts for fleet power/backup over federated metrics"
```

## Task 3: Hub-native fleet-liveness alert (platform-monitoring)

**Files:**
- Create: `config/observability/fleet-liveness-prometheusrule.yaml`

- [ ] **Step 1: Write the PrometheusRule**

This targets the hub platform-monitoring stack and the hub-native metric. Place it in the namespace whose `ServiceMonitor` for `clusterlifecycle-state-metrics` is scraped by platform monitoring (confirm at implementation — see Task 5).

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: pop-fleet-liveness
  namespace: open-cluster-management
  labels:
    app.kubernetes.io/managed-by: pop-operator
spec:
  groups:
    - name: pop-fleet-liveness
      rules:
        - alert: PopFleetClusterDark
          expr: acm_managed_cluster_status_condition{condition="ManagedClusterConditionAvailable",status="true"} == 0
          for: 168h   # N days; set below the cluster's cert-recovery window
          labels:
            severity: warning
          annotations:
            summary: "POP: managed cluster {{ $labels.managed_cluster_id }} dark >7d"
            description: "Cluster unavailable beyond the fleet liveness threshold; approaching the cert-recovery window. Investigate before it becomes unrecoverable."
```

- [ ] **Step 2: Validate rule syntax**

Run: `python3 -c "import yaml; d=yaml.safe_load(open('config/observability/fleet-liveness-prometheusrule.yaml')); import json; open('/tmp/liveness.yaml','w').write(yaml.safe_dump({'groups': d['spec']['groups']}))" && promtool check rules /tmp/liveness.yaml`
Expected: `SUCCESS: 1 rules found`.

- [ ] **Step 3: Commit**

```bash
git add config/observability/fleet-liveness-prometheusrule.yaml
git commit -m "feat(observability): hub-native fleet-liveness (dark cluster) alert"
```

## Task 4: Fleet Grafana dashboard

**Files:**
- Create: `config/observability/fleet-dashboard-configmap.yaml`

- [ ] **Step 1: Write a minimal dashboard ConfigMap**

ACM Grafana loads dashboards from ConfigMaps labelled `grafana-custom-dashboard` in `open-cluster-management-observability`. Keep the dashboard minimal (power status table + backup-age + on-battery count); it can be expanded later.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: pop-fleet-dashboard
  namespace: open-cluster-management-observability
  labels:
    grafana-custom-dashboard: "true"
data:
  pop-fleet.json: |
    {
      "title": "POP Fleet",
      "uid": "pop-fleet",
      "schemaVersion": 39,
      "panels": [
        {
          "type": "stat", "title": "Sites on battery",
          "targets": [{ "expr": "count(pop_power_status == 1)" }],
          "gridPos": { "h": 4, "w": 6, "x": 0, "y": 0 }
        },
        {
          "type": "table", "title": "Power status by cluster",
          "targets": [{ "expr": "pop_power_status", "format": "table", "instant": true }],
          "gridPos": { "h": 8, "w": 18, "x": 6, "y": 0 }
        },
        {
          "type": "table", "title": "Etcd backup age (s) by cluster",
          "targets": [{ "expr": "time() - pop_etcd_backup_last_success_timestamp", "format": "table", "instant": true }],
          "gridPos": { "h": 8, "w": 24, "x": 0, "y": 8 }
        }
      ]
    }
```

- [ ] **Step 2: Validate JSON + YAML**

Run: `python3 -c "import yaml,json; d=yaml.safe_load(open('config/observability/fleet-dashboard-configmap.yaml')); json.loads(d['data']['pop-fleet.json']); print('dashboard JSON valid')"`
Expected: `dashboard JSON valid`.

- [ ] **Step 3: Commit**

```bash
git add config/observability/fleet-dashboard-configmap.yaml
git commit -m "feat(observability): POP fleet Grafana dashboard"
```

## Task 5: Kustomization + implementation-gate verification

**Files:**
- Create: `config/observability/kustomization.yaml`

- [ ] **Step 1: Write the kustomization**

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - metrics-allowlist.yaml
  - thanos-ruler-custom-rules.yaml
  - fleet-liveness-prometheusrule.yaml
  - fleet-dashboard-configmap.yaml
```

- [ ] **Step 2: Validate the kustomization renders**

Run: `kubectl kustomize config/observability/`
Expected: all four objects rendered, no errors.

- [ ] **Step 3: Commit**

```bash
git add config/observability/kustomization.yaml
git commit -m "feat(observability): kustomize fleet observability manifests"
```

- [ ] **Step 4: Record the implementation gates (do NOT skip on a real hub)**

Before relying on these on a live hub, verify:
1. `pop_*` series appear in hub Thanos after allowlist apply: query `pop_power_status` in the ACM Grafana explore / Thanos querier.
2. The custom rules load: MCO thanos-ruler picks up `thanos-ruler-custom-rules` (check thanos-ruler pod logs / rules API).
3. **Liveness gate (from the spike):** confirm `acm_managed_cluster_status_condition` is present in the **hub platform-monitoring Prometheus** and that a `PrometheusRule` in the chosen namespace is actually evaluated (platform monitoring only watches certain namespaces). If the exporter isn't wired to platform monitoring, fall back to a small hub controller that watches `ManagedCluster` conditions — but try the no-code path first.
4. Dashboard appears in ACM Grafana.

Document results in the PR description.

---

## Self-review

- **Spec coverage:** implements design FR7 (fleet visibility) and the Phase-2a observability decomposition, incl. the two-stack correction (MCO-federated power/backup + hub-native liveness).
- **Placeholders:** none — every manifest is complete with exact names/namespaces/PromQL. The dashboard is intentionally minimal but valid and expandable.
- **Consistency:** metric names match `internal/metrics/metrics.go`; namespaces/ConfigMap names match the verified MCO mechanisms; the liveness rule uses the hub-native metric in platform-monitoring, not MCO.

## Notes

- No Go code and no unit tests — these are config artifacts; verification is `promtool`/`kubectl kustomize`/server dry-run plus the live-hub gates in Task 5.
- The liveness threshold (`for: 168h`) is a placeholder value to tune per fleet; it must stay below the cluster cert-recovery window.
- This plan does not wire the manifests into the GitOps repo — that's an ApplicationSet/kustomization add handled with the rest of hub delivery in 2b.
