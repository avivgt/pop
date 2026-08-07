# POP Multicluster / ACM Design

- Status: Draft for review
- Date: 2026-08-04
- Component: POP operator for OpenShift (see `docs/architecture.md`)
- Note: the repo uses two names for POP — "Platform Outage Provisioner"
  (`docs/architecture.md`) and "Power Outage Provisioner" (console plugin
  `displayName`). Pre-existing inconsistency to reconcile; not settled here.
- Scope: extend POP from a single-cluster operator to an ACM-managed fleet capability

---

## 1. Context

POP today is a single-cluster OpenShift operator: etcd backup, scheduled backups,
graceful cluster shutdown (cordon / PDB-aware drain / pre-shutdown hooks / app
notification), cluster restart with CSR approval, power-event response
(SNMP / IPMI-Redfish / NUT), workload evacuation, and an OpenShift console plugin.
8 CRDs under `pop.openshift.io/v1alpha1`. It has no multicluster awareness.

The operational reality that forces this design: a fleet of 200+ OpenShift clusters
across many physical sites cannot be managed per-site by humans. Configuration,
upgrades, fleet visibility, and planned/coordinated operations must be driven from
an ACM hub. At the same time, the safety-critical execution path (a site losing
power) must run locally and must not depend on the hub being reachable.

## 2. Goals / non-goals

### Goals

1. **Spoke-autonomous power resilience.** Every managed cluster's local POP detects
   power loss and shuts its own cluster down correctly with zero hub involvement.
2. **Central management + visibility.** Configure POP and observe power / backup /
   lifecycle state across the whole fleet from the ACM hub (single pane of glass).
3. **Hub-orchestrated operations.** Initiate coordinated fleet operations (e.g.
   "gracefully shut down these N sites now") from the hub.
4. **Unattended cold-start recovery.** Clusters power-cycled or dark past their
   certificate window come back **without a human approving CSRs**. This is the
   linchpin: without it, driver 1 and driver 3 are worthless.
5. **Black-start.** Bring a fully-dark fleet back with predefined, health-gated boot
   sequences across four layers (hardware, cluster, workload, fleet).

### Non-goals (this iteration)

- **Cross-cluster DR / failover** (workload migration between clusters on failure).
  Deferred. Note: cross-cluster **ordering** for black-start *is* in scope; DR is not.
- **MicroShift.** The fleet is full OpenShift (large clusters first) + SNO. MicroShift
  at the edge is owned by Red Hat Edge Manager (RHEM), image-based, out of scope here.
- **Reinventing application data protection.** That is Veeam/Kasten/OADP territory
  (see Section 3). POP does not back up application PVs.

### Guiding principle

**ACM-first, standalone-by-exception.** The hub is the default and only routine
management surface. The single-cluster POP remains fully usable on its own, because
(a) the safety path must run locally regardless, and (b) day-0 / disconnected / dev
use. Standalone is a supported fallback, not a parallel workflow anyone maintains by
hand. **The hub is never required for safety.**

## 3. Prior art & justification

Three adversarially-verified research passes back the build decision. Full artifacts
referenced in Section 14.

### 3.1 Veeam / Kasten (does it make POP redundant?) — No.

- Veeam Kasten (K10) and Veeam Backup & Replication are **application-data
  protection** (namespaces, workloads, PVCs/PVs). They do **not** back up etcd /
  the OpenShift control plane by design; etcd backup exists only as a manual Kanister
  add-on, not a product feature. **Zero native control-plane overlap.**
- No Veeam product has power-event / UPS integration or native whole-cluster graceful
  shutdown+restart+CSR. Any such behavior would be customer-built external glue.
- **Correction we accepted:** POP's *etcd backup* is its weakest differentiator — it
  overlaps native OpenShift `cluster-backup.sh`. POP should **wrap native tooling**,
  not reinvent it.

### 3.2 Ecosystem cold-start / cert handling — the two-cert-class insight

- **Leaf certs** (kubelet serving/client): single-binary edge distros (MicroShift,
  k3s, RKE2, Talos-client) re-sign expired leaf certs from a local CA on boot, no
  human. **This class is solved by the ecosystem.**
- **The CA itself expiring**: solved by **nobody**. RKE2 boot-loops, k3s needs manual
  intervention, MicroShift/Talos undocumented. Unrecoverable time-bomb everywhere.
- **The pre-API control-plane cert path is the real gap.** Control-plane cert recovery
  must happen *before* the API server starts, so no in-cluster pod and no remote hub
  can do it — it must be node-local. In-cluster CSR approvers (kubelet-csr-approver
  etc.) only handle leaf serving CSRs and need a healthy API.
- **POP's defensible gaps, ranked:** (1) declarative PDB/etcd-quorum-aware
  **whole-cluster shutdown+restart CRDs** — CAPI has none, OpenShift is a runbook
  (strongest); (2) **multi-cluster priority tiering** — no prior art; (3)
  **power-event UPS/Redfish/IPMI/NUT trigger + etcd backup** — only homelab prior art
  (crowsnest, single-node, uncoordinated, experimental); (4) **node-local cold-start
  cert recovery** incl. the fully-expired-CA path — real but narrow, "completion not
  reinvention."
- **Rule:** do **not** reinvent leaf-cert auto-rotation / kubelet-serving CSR approval.
  POP owns CSR approval end-to-end anyway (Section 9) because it is critical to
  unattended recovery, but the *novel* value is the control-plane / CA path + the
  orchestration + the fleet layer.

### 3.3 ACM fleet UI extensibility (2026)

- POP's `@openshift-console/dynamic-plugin-sdk` v4.22 is single-cluster (no `cluster`
  param on watch hooks).
- **Base console extension points are GA** (dynamic plugins since OCP 4.11): pages,
  nav, and targeting the ACM "Fleet Management" perspective by ID. A third-party
  plugin can register fleet UI with the same GA mechanism POP already uses.
- **Preview risk is confined to managed-cluster data access** from the hub console —
  the separate `@stolostron/multicluster-sdk` (v0.10.3, pre-1.0, **not GA**), gated by
  `useIsFleetAvailable` + provider flag.
- Hub→spoke plumbing: `cluster-proxy-addon` GA-grade (konnectivity, since RHACM 2.7);
  `managed-serviceaccount` beta (`v1beta1`).
- **Consequence:** the fleet single-pane-of-glass is built over POP's **hub CRDs**
  using the **GA base SDK**; the preview SDK is used only for optional spoke
  drill-down, defensively gated with a hub-CRD fallback.

## 4. Requirements

### Functional

- FR1  Local POP shuts its own cluster down on power loss with no hub dependency.
- FR2  Shutdown behavior adapts to detected cluster type (full vs SNO).
- FR3  Node-local, unattended CSR approval + control-plane cert recovery on cold start.
- FR4  CSR approval is gated by a captured, signed node inventory (no blanket approve).
- FR5  Optional automated fully-expired-CA re-issue (opt-in, caveated).
- FR6  Config + POP delivery to spokes via GitOps (git = source of truth).
- FR7  Fleet visibility: metrics (ACM Observability) + `FleetPowerStatus` hub CRD.
- FR8  Hub-orchestrated fleet operations via `FleetShutdown` (Placement-selected).
- FR9  Black-start across 4 layers: staggered hardware power-on, cluster bring-up,
       workload ordering (`BootSequence`), cross-cluster ordering (`FleetBootSequence`).
- FR10 Fleet UI in the ACM console over hub CRDs; Grafana fleet dashboards.

### Non-functional

- NFR1 Safety path works fully hub-disconnected and behind NAT.
- NFR2 CSR auto-approval must be scoped to prevent rogue-node join (security-critical).
- NFR3 Stuck handling must never leave a cluster in a hard-poweroff-on-dead-battery
       state; force-progress to safe poweroff before the battery deadline.
- NFR4 Fleet UI core must not depend on preview APIs.
- NFR5 Wrap native OpenShift tooling (etcd backup, machine-approver) where it exists.

## 5. Architecture overview

```
                              ACM HUB (OpenShift)
  ┌─────────────────────────────────────────────────────────────────────┐
  │  GitOps (ArgoCD)        POP-Fleet Operator          ACM Observability │
  │  ApplicationSet         CRDs:                        Thanos / Grafana │
  │  + Placement            FleetShutdown                Alertmanager     │
  │  (operator + per-       FleetPowerStatus  ◄── materialize status from │
  │   cluster config)       FleetBootSequence     metrics + ManagedCluster│
  │        │                controllers +          lease (liveness)       │
  │        │ git→spoke      BMC/Redfish power-on          ▲               │
  │        │                       │ OOB mgmt net         │ outbound      │
  └────────┼───────────────────────┼─────────────────────┼───────────────┘
           │                        │ (OOB-capable sites) │ metrics push
           ▼                        ▼                     │ (NAT-safe)
  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────────┐
  │ SPOKE: full OCP  │  │ SPOKE: full OCP  │  │ SPOKE: SNO           │
  │  POP operator    │  │  POP operator    │  │  POP operator        │
  │  + boot-agent    │  │  + boot-agent    │  │  + boot-agent        │
  │  (static pod/MCO)│  │  (static pod/MCO)│  │  (static pod/MCO)    │
  │  UPS: SNMP/IPMI/ │  │  UPS per site    │  │  UPS per site        │
  │       NUT        │  │                  │  │                      │
  └──────────────────┘  └──────────────────┘  └──────────────────────┘
        autonomous on power loss even if hub/git unreachable
```

Data directions:
- **Config down:** hub → spoke via GitOps only.
- **Status up:** spoke → hub **outbound** (metrics via MCO + ManagedCluster lease).
  The hub controller *materializes* `FleetPowerStatus`; it does **not** pull from
  spokes (managed-serviceaccount pull fails for NAT'd edge).
- **Orchestration down:** `FleetShutdown` / `FleetBootSequence` reconciled on the hub;
  imperative BMC/Redfish power-on to OOB-capable sites over the mgmt network.

## 6. Components

### Runtime layers

The design spans three distinct runtime layers. Confusing them (especially placing
cert recovery at the operator layer) breaks cold-start, so they are called out
explicitly:

- **Fleet / hub layer** — POP-fleet operator + hub CRDs. Runs on the ACM hub.
- **Operator layer** — POP controller-manager. Runs as a Deployment on each spoke;
  needs the API server, scheduler, and Ready nodes.
- **Node / platform layer** — boot-agent. A static pod placed via MachineConfig on
  control-plane nodes; runs pre-API during boot, at the same layer as the
  control-plane static pods (etcd, kube-apiserver), deliberately *outside* the
  operator's runtime.

The boot-agent is **operator-provisioned but node-level**: the POP operator (or
GitOps) renders and applies the MachineConfig that delivers it, but the running
component's lifecycle is bound to the node, not to the operator Deployment. It is
**not** an operator-runtime component — it must exist precisely when the operator
cannot run.

### Spoke side

1. **POP operator** — existing operator, plus **cluster-type auto-detection** driving
   shutdown behavior:
   - full OCP → cordon / PDB-aware drain / evict (existing `reconcileDraining`).
   - SNO → quiesce workloads + etcd backup + clean poweroff (no drain-to-peer).
2. **boot-agent** — **node-local, cold-start-safe** (static pod delivered via
   MachineConfig on control-plane nodes). Runs independent of the operator and the
   full API. Owns **all** cert healing (Section 9). This is the piece that cannot live
   on the hub or in a normal pod.
3. **Metrics/heartbeat** — POP's existing Prometheus metrics (reused), federated to
   the hub by MCO. Outbound, NAT-safe.

### Hub side

4. **POP-fleet operator** — new, lightweight. Reconciles `FleetShutdown` and
   `FleetBootSequence`; materializes `FleetPowerStatus`; drives BMC/Redfish power-on
   for OOB-capable sites; verifies recovery closure.
5. **GitOps ApplicationSet** — delivers the POP operator + per-cluster config overlays
   (`AutomatedProceduresConfig`, `ClusterLifecycleConfig`, `BackupSchedule`,
   `BootSequence`) selected by Placement.
6. **ACM Observability (MCO)** — federates POP metrics; hub Alertmanager fires fleet
   alerts ("site on battery", "backup failing", "shutdown in progress", "site nearing
   cert-validity breach").
7. **Fleet UI** — hub-side dynamic plugin over hub CRDs (GA base SDK), registered in
   the ACM Fleet Management perspective; Grafana dashboards.

## 7. API / CRD design

New hub-scoped CRDs (`pop.openshift.io/v1alpha1` on the hub) and spoke CRDs. Shapes
are illustrative, not final field-by-field.

### 7.1 `FleetShutdown` (hub) — imperative fleet operation

```yaml
spec:
  clusterSelector:            # ACM Placement reference or label selector
    placementRef: { name: edge-tier-1 }
  reason: "planned maintenance window 2026-08-10"
  dryRun: false
  shutdownPolicy:             # inherited by per-spoke ClusterShutdown
    onStuck: force            # force | skip | haltAndAlert | escalateToHub
    hardDeadlineSeconds: 900
status:
  phase: InProgress           # Pending|InProgress|Completed|PartiallyFailed
  clusters:
    - name: site-042
      phase: ShuttingDown
      message: "draining: 3 pods pending eviction"
```

The hub controller fans this out by rendering a `ClusterShutdown` onto each selected
spoke. For planned ops the render path can be GitOps (auditable) or direct via the
hub controller for live incident response; both terminate in a spoke `ClusterShutdown`.

### 7.2 `FleetPowerStatus` (hub) — aggregated current state (one per managed cluster)

```yaml
status:
  clusterName: site-042
  reachable: true             # from ManagedCluster lease
  powerStatus: OnBattery      # Normal|OnBattery|ShuttingDown|Down|Recovering
  batterySecondsRemaining: 480
  lastBackup: { time: "...", result: Success }
  shutdownPhase: ""
  recovery: { phase: "", nodesReady: 0, nodesExpected: 0 }
  breachRisk: false           # dark site nearing cert/CA validity
  source: metrics+lease       # materialized, not pulled
```

Materialized by the hub controller from federated metrics + ManagedCluster lease.
This CRD is also the **GA data source for the fleet UI**.

### 7.3 `BootSequence` (spoke) — intra-cluster black-start

```yaml
spec:
  powerOn:                    # hardware layer (used where OOB reachable)
    order: [control-plane, worker]
    maxConcurrent: 4          # inrush / power-budget limit
    staggerSeconds: 30
  stages:                     # workload layer, health-gated
    - name: storage
      selector: { namespaces: [openshift-storage] }
      healthGate: AllPodsReady
      timeoutSeconds: 600
      onStuck: force
    - name: networking
      dependsOn: [storage]
    - name: data-services
      dependsOn: [networking]
    - name: apps
      dependsOn: [data-services]
```

### 7.4 `FleetBootSequence` (hub) — cross-cluster black-start ordering

```yaml
spec:
  tiers:
    - name: shared-services   # identity, registry, DNS clusters first
      placementRef: { name: hub-region }
      healthGate: AllClustersReady
      timeoutSeconds: 1800
    - name: tier-1-edge
      dependsOn: [shared-services]
      placementRef: { name: edge-tier-1 }
    - name: tier-2-edge
      dependsOn: [tier-1-edge]
```

### 7.5 Recovery manifest (spoke, on-disk) — CSR approval gating

Not a CRD; a signed on-disk artifact written at shutdown time, consumed by boot-agent
before the API is up. See Section 9.

### 7.6 Per-site power/OOB config

Part of per-cluster GitOps config: `oob: redfish|ipmi|none`, BMC endpoint + credentials
secret ref, per-site power budget. Tells the hub which sites it can actively power on.

## 8. Data flows

### 8.1 Config down
Git commit → ArgoCD ApplicationSet renders per-cluster overlays via Placement generator
→ synced to each spoke → local POP reconciles. Once synced, POP runs standalone.

### 8.2 Status up
POP emits metrics → MCO federates outbound to hub Thanos → hub controller materializes
`FleetPowerStatus` (+ ManagedCluster lease for liveness) → fleet UI + alerts.

### 8.3 Power-loss shutdown (autonomous)
UPS event (SNMP/IPMI/NUT) → local POP → `ClusterShutdown` → cluster-type-adapted
shutdown with stuck policy + battery-derived hard deadline → **capture recovery
manifest** → safe poweroff. No hub involvement required.

### 8.4 Black-start / restart
Power returns → (staggered BMC power-on by hub for OOB sites, else BIOS auto) →
boot-agent heals control-plane certs + approves inventory-matched CSRs → API up,
nodes Ready → `BootSequence` runs workload stages health-gated → klusterlet reconnects
→ hub sees `Recovering→Normal`; `FleetBootSequence` gates cross-cluster tier ordering.

## 9. Cert recovery & CSR security model (central pillar)

**Why central:** unattended CSR approval is what makes "200 clusters, power-cycled,
no per-site humans" real. If it isn't guaranteed, every long outage ends in manual
`oc adm certificate approve` per site, which detonates the premise.

**Why node-local (not hub):** control-plane cert recovery must happen before the API
server starts. A powered-off cluster has no API and no klusterlet, so the hub is blind
and powerless; ACM is pull-based and can't reach NAT'd sites; the klusterlet has the
same cert time-bomb. The only actor that can heal is on the node.

**boot-agent responsibilities (node-local, static pod via MachineConfig):**
1. **Control-plane cert recovery** — re-sign expired short-lived control-plane/signer
   certs from on-disk signing material so kube-apiserver can start. Automates
   OpenShift's own documented expired-cert recovery, unattended.
2. **Leaf CSR approval** — actively approve pending node CSRs (does not wait on
   `cluster-machine-approver` health/timing), so recovery cannot silently stall.
3. **CA re-issue (opt-in)** — if the CA itself expired and `caReissue: enabled`,
   re-issue from the on-disk CA private key and cascade trust. Heavy, narrow; default
   off. Prevention-first otherwise.

**Security — inventory-gated approval (NFR2):** an always-on auto-approver is exactly
the backdoor Kubernetes gates. So:
- At shutdown, POP writes a **signed recovery manifest** on-disk: expected nodes and
  their cert identities (CN / SAN / machine IDs), a monotonic sequence, and an expiry.
- On restart, boot-agent approves **only** CSRs matching that manifest, **within a
  bounded recovery window**, then reverts to no-auto-approve.
- Result: "approve exactly the nodes we deliberately shut down," not "approve all
  pending." Manifest is signed to prevent tampering; window-bounded to limit exposure.

**ACM's role (orchestration, not approval):** BMC/Redfish power-on for OOB sites;
liveness via lease; recovery tracking; **breach-prevention alerting** (dark site
nearing signer/CA validity); **closure verification** (all expected nodes Ready within
deadline, else raise a fleet alert and flag the site for a human).

## 10. Failure handling

- **Stuck phase (shutdown or black-start):** each phase has timeout + retries +
  `onStuck` (`force|skip|haltAndAlert|escalateToHub`). A global **hard deadline**
  (battery-derived for power events) overrides everything and force-progresses to a
  safe poweroff. **Default for power-triggered = force-progress on deadline** (NFR3).
- **Hub down:** spokes fully autonomous; config already synced; status resumes when
  hub returns.
- **Spoke disconnected / NAT:** status via outbound metrics only; hub marks
  `reachable:false` on missed lease.
- **CA expired (prevention-first):** validity ≫ max off-duration + breach alerting; if
  reached, opt-in CA re-issue or flag for manual rebuild.
- **Partial recovery:** boot-agent approves what it can; ACM closure-check flags
  sites where `nodesReady < nodesExpected` for human attention.
- **Power-on inrush:** `maxConcurrent` + `staggerSeconds` bound simultaneous power-on.

## 11. Security & RBAC

- boot-agent: minimal, node-local; approval scoped by signed recovery manifest +
  bounded window; access to on-disk signing material protected by node file perms.
- Hub BMC/Redfish credentials: per-site secrets, referenced not inlined.
- Status channel: outbound metrics only; no hub→spoke pull requirement for safety.
- `cluster-proxy-addon` (GA) for any hub→spoke console access; `managed-serviceaccount`
  (beta) only where reachable and non-safety-critical.

## 12. UI approach

- **Phase 1 (GA):** hub-side dynamic plugin over `FleetPowerStatus` / `FleetShutdown`
  / `FleetBootSequence` using the base SDK POP already ships; register pages/nav and
  target the ACM Fleet Management perspective by ID. Grafana fleet dashboards via MCO.
- **Phase 2 (optional, preview):** spoke drill-down (live per-cluster CRs) via
  `@stolostron/multicluster-sdk`, **gated by `useIsFleetAvailable` with hub-CRD
  fallback**. Never a hard dependency (NFR4).

## 13. Phasing (decomposition into sub-specs)

This is larger than one implementation plan. Each phase gets its own spec → plan.

1. **Spoke core** — cluster-type-aware shutdown + boot-agent (control-plane cert heal
   + inventory-gated CSR approval) + `BootSequence` (intra-cluster) + recovery manifest.
2. **Hub fleet layer** — GitOps delivery, `FleetPowerStatus` + `FleetShutdown`,
   observability, hub-CRD fleet UI.
3. **Fleet black-start** — `FleetBootSequence` cross-cluster ordering + staggered
   BMC/Redfish power-on.
4. **Advanced / opt-in** — CA re-issue, spoke drill-down UI, cross-cluster DR.

## 14. References

- Research: Veeam/Kasten vs POP (control-plane/etcd overlap) — adversarially verified.
- Research: ecosystem cold-start / cert handling (leaf vs CA cert classes; four gaps).
- Research: ACM console multicluster UI extensibility (2026 GA vs preview).
- OpenShift: graceful cluster shutdown/restart procedure; expired-cert recovery;
  `cluster-machine-approver`; `cluster-backup.sh`.
- ACM: ApplicationSet + Placement generator; MCO; `cluster-proxy-addon`;
  `managed-serviceaccount`; Fleet Management perspective.
- `@stolostron/multicluster-sdk` v0.10.3 (preview).

## 15. Open questions / risks

- Exact OpenShift cert/signer lifetimes and the precise expired-cert recovery steps to
  automate in boot-agent (verify before implementation).
- MachineConfig delivery of a static-pod boot-agent on SNO vs multi-node control planes.
- CA re-issue trust-cascade blast radius — confirm operational procedure before enabling.
- Whether live `FleetShutdown` fan-out should ever bypass GitOps for incident speed,
  or always route through git for auditability.
- `@stolostron/multicluster-sdk` maturing to GA before we depend on drill-down.
