# POP Architecture

## Overview

POP (Platform Outage Provisioner) is an OpenShift operator that automates etcd backup, graceful cluster shutdown, and power failure response. It consists of a Go operator managing Kubernetes CRDs and a React console plugin providing the admin UI.

## Components

```
+---------------------+     +------------------------+     +------------------+
|  Console Plugin     |     |  Operator (Go)         |     |  Alert Server    |
|  (React/TypeScript) |     |  8 controllers         |     |  (HTTP webhook)  |
|  nginx on port 9443 |     |  event source monitors |     |  port 8082       |
+---------------------+     +------------------------+     +------------------+
         |                            |                            |
         v                            v                            v
   OpenShift Console           K8s API Server              UPS/PDU devices
```

## CRDs

### ClusterLifecycleConfig (singleton)
Central configuration for backup scheduling, storage, shutdown defaults, and notifications. One per cluster, named "default" in the operator namespace.

### AutomatedProceduresConfig (singleton)
Standalone CRD for automated event response. Configures power event detection via multiple sources (webhook, SNMP, IPMI/Redfish, NUT), battery parameters, confirmation delays, and autonomous shutdown behavior.

### EtcdBackup
One per backup operation. Created by scheduled backups, manual triggers, or power event response. Manages a Job that runs the backup script on a control plane node.

### ClusterShutdown
One per shutdown operation. Progresses through 8 phases: Pending, Scheduled, Validating, Cordoning, RunningHooks, NotifyingApps, BackingUp, ShuttingDown. Supports dry-run mode.

### PowerEvent
One per detected power event. Types: PowerFailure, PowerRestored, Test. PowerFailure events drive the backup-confirm-shutdown flow. PowerRestored events cancel active failures.

### BackupSchedule, ClusterRestart, WorkloadEvacuation
Supporting CRDs for scheduled backups, post-shutdown restart orchestration, and workload migration.

## Controller State Machines

### ClusterShutdown
```
Pending -> Scheduled (if future time) -> Validating -> Cordoning ->
RunningHooks -> NotifyingApps -> BackingUp -> ShuttingDown -> Complete
                                                              |
Any phase may -> Failed                                       |
DryRun skips Cordoning and ShuttingDown ----------------------+
```

### EtcdBackup
```
Pending -> Validating (PVC + node check) -> Running (Job) -> Completed
Any phase may -> Failed
```

### PowerEvent (PowerFailure)
```
Detected -> BackingUp -> AwaitingConfirmation -> Confirmed -> ShuttingDown -> Complete
                                                    |
                              AutonomousShutdown=false -> Complete (manual)
Any phase may -> Cancelled (if PowerRestored received)
Any phase may -> Failed
```

## Event Sources

The AutomatedProceduresConfig controller manages polling monitors for power event detection:

| Source | Protocol | What it watches |
|--------|----------|-----------------|
| Webhook | HTTP POST to /api/v1/power-event | UPS/PDU alert webhooks |
| SNMP | UDP polling UPS-MIB OIDs | Battery status, output source |
| IPMI/Redfish | HTTPS GET to BMC | Power supply health/state |
| NUT | TCP to NUT server (port 3493) | ups.status variable (OL/OB/LB/FSD) |

Multiple sources can run simultaneously. The event source manager deduplicates: if an active PowerFailure event already exists, additional sources skip creating duplicates.

## Deployment

```
Namespace: pop-system
+----------------------------------+  +---------------------------+
| pop-controller-manager           |  | pop-console-plugin        |
| - 8 CRD controllers             |  | - nginx serving React app |
| - alert webhook server (:8082)   |  | - TLS on port 9443        |
| - event source monitors          |  | - registered as            |
| - leader election enabled        |  |   ConsolePlugin CR         |
| OLM-managed via CSV              |  +---------------------------+
+----------------------------------+
```

## Monitoring

ServiceMonitor scrapes operator metrics at /metrics (HTTPS). PrometheusRule defines 11 alerts:

- **Backup**: failed, no recent (24h), schedule missed, slow (>10min)
- **Cluster**: cert expiring (<7d warning, <24h critical)
- **Shutdown**: failed, in progress
- **Power**: failure detected, autonomous shutdown imminent, event unhandled

## Release Process

```bash
# Run tests, lint, build, and push all images
make release VERSION=0.2.0 IMAGE_TAG_BASE=registry.example.com/pop

# Generate OLM bundle
make bundle VERSION=0.2.0

# Migration between versions
./scripts/migrate-v0.1-to-v0.2.sh --namespace pop-system
```
