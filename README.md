# POP — Power Outage Provisioner

An OpenShift operator that protects clusters from power outages. Backs up etcd automatically, shuts down gracefully when UPS battery kicks in, and recovers after restart.

## What it does

- **etcd Backup** — on-demand and scheduled backups to PVC or S3, with retention
- **Graceful Shutdown** — cordons nodes, runs pre-shutdown hooks, backs up, powers down
- **Power Protection** — receives PSU/UPS alerts via webhook, auto-backs-up immediately, optionally auto-shuts-down after a configurable confirmation period
- **Cluster Priority** — Critical / Standard / BestEffort tiers control shutdown delay in multi-cluster datacenters
- **Post-Restart Recovery** — automatically approves CSRs and verifies operator health after boot
- **Console Plugin** — OpenShift dashboard under "Backup and Restore" with Backup Now button, shutdown controls, power status, and settings

## Quick start

### Prerequisites

- OpenShift 4.14+
- `oc` CLI logged in as cluster-admin
- `podman`
- A PVC for backup storage (10Gi+ recommended)

### Deploy

```bash
# Build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/manager-linux-amd64 cmd/main.go
podman build --platform linux/amd64 -f Containerfile.deploy -t <registry>/pop-operator:latest .
podman push <registry>/pop-operator:latest

# Deploy
make deploy IMG=<registry>/pop-operator:latest

# Grant privileged SCC for backup jobs
oc adm policy add-scc-to-user privileged system:serviceaccount:pop-system:default -n pop-system
```

### Configure

Create the singleton config:

```yaml
apiVersion: pop.openshift.io/v1alpha1
kind: ClusterLifecycleConfig
metadata:
  name: default
  namespace: pop-system
spec:
  backup:
    storage:
      pvcName: etcd-backup-storage
    schedule:
      enabled: true
      frequency: Daily       # Every4Hours | Every8Hours | Every12Hours | Daily | Weekly | Monthly
      time: "02:00"
    retention:
      count: 5
      days: 30
    timeoutMinutes: 15
  powerProtection:
    enabled: true
    clusterPriority: Standard  # Critical | Standard | BestEffort
    autonomousShutdown: false
    alertEndpoint:
      port: 8082
      authTokenSecret: psu-alert-token
```

### PSU alert integration

Configure your UPS/PDU to POST to the operator when power fails:

```bash
curl -X POST http://<cluster-node>:8082/api/v1/power-event \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"type": "power_failure", "source": "ups-rack-42"}'
```

The operator immediately starts an etcd backup. If `autonomousShutdown` is true and power stays off past the confirmation period, it gracefully shuts down the cluster.

Send `{"type": "power_restored"}` to cancel a pending shutdown.

## Architecture

One user-facing CRD (`ClusterLifecycleConfig`) configures everything. Internal CRDs handle execution:

| Internal CRD | Created by | Purpose |
|---|---|---|
| EtcdBackup | Operator / Console | Runs backup job on a control plane node |
| ClusterShutdown | Operator / Console | Cordons, hooks, backs up, shuts down nodes |
| ClusterRestart | Operator | Post-boot CSR approval + operator health check |
| PowerEvent | Alert server | Tracks power failure lifecycle |

## Console plugin

Shows under **Backup and Restore > POP Dashboard** in the OpenShift admin console:

- Status bar: power state, certificate expiry, node count
- etcd Backup: Backup Now button, schedule info, backup history
- Cluster Operations: Shutdown button with confirmation modal
- Power Protection: status, Test Alert button, event history
- Settings: backup schedule, S3 storage, PSU management URL, notifications

## Uninstall

```bash
make undeploy
```

## License

Apache License 2.0
