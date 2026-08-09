# POP Multicluster / ACM Design

- Status: Draft for review
- Date: 2026-08-04 (revised 2026-08-09)
- Component: POP operator for OpenShift (see `docs/architecture.md`)
- Note: the repo uses two names for POP — "Platform Outage Provisioner"
  (`docs/architecture.md`) and "Power Outage Provisioner" (console plugin
  `displayName`). Pre-existing inconsistency to reconcile; not settled here.

---

## Revision note (2026-08-09) — cold-start recovery re-scoped

A verified technical spike on OpenShift cold-start certificate behavior changed
the cert-recovery design substantially. Summary (details in §3.2 and §9):

- **OpenShift self-heals its own control plane** on cold boot. The
  `cert-regeneration-controller` re-signs expired **leaf** certs from the
  still-valid signing CAs; the API server comes up; operators reissue node
  kubeconfigs. This works for clusters off **weeks to ~1 year** (bounded by the
  shortest long-lived signer, ~1yr). POP must **not** reimplement this.
- The **only** cold-start gap is **kubelet CSR approval on SNO / UPI clusters**
  (no Machine object, so `cluster-machine-approver` structurally won't
  auto-approve). IPI clusters recover fully unattended.
- Therefore the node-local "boot-agent" is **removed from the core design.**
  Cold-start recovery is handled by OCP self-heal + **hub-side prevention**
  (dark-duration tracking + breach alerting) + a manual CSR runbook for the rare
  breach. A **secure SNO/UPI CSR-approver add-on** is kept as an **optional,
  default-off** deliverable for the *unmanned* SNO/UPI subset only (§9, Phase 4).
- **Automated CA re-issue is dropped** (reversing an earlier decision): expired
  signing CAs / >~1yr outages are out of scope for everyone — detect-and-refuse.
- **Security fix:** POP already ships a blanket CSR approver
  (`internal/controller/clusterrestart_controller.go:210-237`) that approves any
  kubelet CSR with no identity check — a real vulnerability, being hotfixed
  independently of this design.

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
4. **Reliable unattended restart.** After power returns, clusters come back without
   per-site humans. For IPI this is OCP-native. For SNO/UPI the residual gap is
   kubelet CSR approval, addressed by hub-side prevention (alert before the cert
   window) plus, for unmanned sites, an optional secure CSR-approver add-on.
5. **Black-start.** Bring a fully-dark fleet back with predefined, health-gated boot
   sequences across four layers (hardware, cluster, workload, fleet).

### Non-goals (this iteration)

- **Cross-cluster DR / failover** (workload migration between clusters on failure).
  Deferred. Cross-cluster **ordering** for black-start *is* in scope; DR is not.
- **MicroShift.** The fleet is full OpenShift (large clusters first) + SNO. MicroShift
  at the edge is owned by Red Hat Edge Manager (RHEM), image-based, out of scope here.
- **Reimplementing OpenShift cert regeneration.** OCP self-heals leaf certs; POP
  delegates to it and never re-signs control-plane certs or re-issues CAs.
- **Recovering CA-expired / >~1yr-dark clusters unattended.** Out of scope for
  everyone; detect-and-refuse + alert.
- **Reinventing application data protection.** That is Veeam/Kasten/OADP territory
  (see §3). POP does not back up application PVs.

### Guiding principle

**ACM-first, standalone-by-exception.** The hub is the default and only routine
management surface. The single-cluster POP remains fully usable on its own, because
(a) the safety path must run locally regardless, and (b) day-0 / disconnected / dev
use. Standalone is a supported fallback, not a parallel workflow anyone maintains by
hand. **The hub is never required for safety.**

## 3. Prior art & justification

Four adversarially-verified research passes back the build decision. Full artifacts
referenced in §14.

### 3.1 Veeam / Kasten (does it make POP redundant?) — No.

- Veeam Kasten (K10) and Veeam Backup & Replication are **application-data
  protection** (namespaces, workloads, PVCs/PVs). They do **not** back up etcd /
  the OpenShift control plane by design. **Zero native control-plane overlap.**
- No Veeam product has power-event / UPS integration or native whole-cluster graceful
  shutdown+restart+CSR. Any such behavior would be customer-built external glue.
- **Correction accepted:** POP's *etcd backup* is its weakest differentiator — it
  overlaps native OpenShift `cluster-backup.sh`. POP should **wrap native tooling**,
  not reinvent it.

### 3.2 Cold-start cert behavior — what OCP does vs the real gap

- **Leaf certs self-heal.** On cold boot the `cert-regeneration-controller` re-signs
  expired leaf certs (kube-apiserver serving/client ~30d, node kubeconfigs ~90d)
  from the still-valid signing CAs; the API server starts; operators reissue node
  kubeconfigs. This is automatic and works for outages of **weeks to ~1 year**.
- **The bound is the shortest long-lived signer (~1yr).** kube-apiserver-to-kubelet-
  signer ~1yr, etcd leaf ~3yr, node-system-admin ~2-3yr, roots longer. These are
  issued near install and do not reset per boot. If one expires (old cluster near
  EOL, or off >~1yr), OCP cannot self-heal and **no one** recovers it unattended
  safely — detect-and-refuse.
- **The "24h" guidance is conservative, not the technical cliff.** The real recovery
  window is CA/signer-bounded (~1yr), not 24h.
- **The one residual gap: SNO / UPI kubelet CSR approval.** `cluster-machine-approver`
  matches kubelet-serving CSR SANs against the **Machine** object (an independent,
  out-of-band source). SNO/UPI clusters have **no Machine object**, so it
  structurally refuses to auto-approve → node CSRs sit Pending until a human runs
  `oc adm certificate approve`. IPI clusters have Machines → fully automatic.
- **Security constraint (verified):** never approve a kubelet-serving CSR by matching
  node-self-reported addresses (`Node.Status.Addresses`) — that is circular and is
  exactly the hole in POP's current blanket approver. Safe matching: client CSRs by
  CN/group/usage; serving CSRs by renewal-continuity (new SANs exactly equal the
  node's existing valid cert) or an operator-provided out-of-band allow-list; else
  leave Pending and alert.
- **POP's defensible gaps, ranked:** (1) declarative PDB/etcd-quorum-aware
  **whole-cluster shutdown+restart** — CAPI has none, OpenShift is a runbook
  (strongest); (2) **multi-cluster priority tiering** — no prior art; (3)
  **power-event UPS/Redfish/IPMI/NUT trigger + etcd backup** — only homelab prior
  art; (4) **secure SNO/UPI cold-start CSR approval** — narrow, optional.

### 3.3 ACM fleet UI extensibility (2026)

- POP's `@openshift-console/dynamic-plugin-sdk` v4.22 is single-cluster.
- **Base console extension points are GA** (dynamic plugins since OCP 4.11): pages,
  nav, and targeting the ACM "Fleet Management" perspective by ID.
- **Preview risk is confined to managed-cluster data access** — the separate
  `@stolostron/multicluster-sdk` (v0.10.3, pre-1.0, **not GA**), gated by
  `useIsFleetAvailable`.
- Hub→spoke plumbing: `cluster-proxy-addon` GA-grade; `managed-serviceaccount` beta.
- **Consequence:** the fleet single-pane-of-glass is built over POP's **hub CRDs**
  using the **GA base SDK**; the preview SDK is used only for optional spoke
  drill-down, defensively gated with a hub-CRD fallback.

## 4. Requirements

### Functional

- FR1  Local POP shuts its own cluster down on power loss with no hub dependency.
- FR2  Shutdown behavior adapts to detected cluster type (full vs SNO).
- FR3  Cold-start restart relies on OCP-native self-heal; POP does not re-sign
       control-plane certs or re-issue CAs.
- FR4  Hub tracks per-site dark duration and alerts before the node-cert window;
       rare breaches are flagged for a documented manual CSR-approval runbook.
- FR5  (Optional add-on) A secure, default-off SNO/UPI kubelet-CSR approver for the
       unmanned subset: client CSRs by CN/group/usage; serving CSRs by
       renewal-continuity or operator allow-list; detector-first, bail-to-human.
- FR6  Config + POP delivery to spokes via GitOps (git = source of truth).
- FR7  Fleet visibility: metrics (ACM Observability) + `FleetPowerStatus` hub CRD.
- FR8  Hub-orchestrated fleet operations via `FleetShutdown` (Placement-selected).
- FR9  Black-start across 4 layers: staggered hardware power-on, cluster bring-up,
       workload ordering (`BootSequence`), cross-cluster ordering (`FleetBootSequence`).
- FR10 Fleet UI in the ACM console over hub CRDs; Grafana fleet dashboards.
- FR11 Replace the existing blanket CSR approver with the safe matching model.

### Non-functional

- NFR1 Safety path works fully hub-disconnected and behind NAT.
- NFR2 Any CSR auto-approval must be scoped to prevent rogue-node join
       (never node-self-reported identity).
- NFR3 Stuck handling must never leave a cluster in a hard-poweroff-on-dead-battery
       state; force-progress to safe poweroff before the battery deadline.
- NFR4 Fleet UI core must not depend on preview APIs.
- NFR5 Wrap native OpenShift tooling (etcd backup, cert regeneration,
       machine-approver) — never reimplement it.

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
  │        │ git→spoke      BMC/Redfish power-on   + dark-duration        │
  │        │                       │ OOB mgmt net    breach alerting      │
  └────────┼───────────────────────┼─────────────────────┼───────────────┘
           │                        │ (OOB-capable sites) │ metrics push
           ▼                        ▼                     │ (NAT-safe)
  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────────┐
  │ SPOKE: full OCP  │  │ SPOKE: full OCP  │  │ SPOKE: SNO           │
  │  POP operator    │  │  POP operator    │  │  POP operator        │
  │  UPS: SNMP/IPMI/ │  │  UPS per site    │  │  UPS per site        │
  │       NUT        │  │                  │  │  [opt: CSR approver  │
  │                  │  │                  │  │   add-on if unmanned]│
  └──────────────────┘  └──────────────────┘  └──────────────────────┘
        autonomous on power loss even if hub/git unreachable
        cold-start: OCP self-heals; hub alerts before cert window
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

- **Fleet / hub layer** — POP-fleet operator + hub CRDs. Runs on the ACM hub.
- **Operator layer** — POP controller-manager. Runs as a Deployment on each spoke;
  needs the API server, scheduler, and Ready nodes.
- **Node / platform layer (optional add-on only)** — the SNO/UPI CSR-approver, if
  deployed, is a static pod via MachineConfig, operator-provisioned but node-level.
  Not part of the core design; see §9.

### Spoke side

1. **POP operator** — existing operator, plus **cluster-type auto-detection** driving
   shutdown behavior:
   - full OCP → cordon / PDB-aware drain / evict (existing `reconcileDraining`).
   - SNO → quiesce workloads + etcd backup + clean poweroff (no drain-to-peer).
2. **Metrics/heartbeat** — POP's existing Prometheus metrics, federated to the hub by
   MCO. Outbound, NAT-safe. Includes the shutdown timestamp so the hub can compute
   dark duration.
3. **(Optional) SNO/UPI CSR-approver add-on** — see §9. Default-off; only for the
   unmanned SNO/UPI subset.

### Hub side

4. **POP-fleet operator** — new, lightweight. Reconciles `FleetShutdown` and
   `FleetBootSequence`; materializes `FleetPowerStatus`; drives BMC/Redfish power-on
   for OOB-capable sites; tracks dark duration and fires breach-prevention alerts;
   verifies recovery closure.
5. **GitOps ApplicationSet** — delivers the POP operator + per-cluster config overlays
   (`AutomatedProceduresConfig`, `ClusterLifecycleConfig`, `BackupSchedule`,
   `BootSequence`) selected by Placement.
6. **ACM Observability (MCO)** — federates POP metrics; hub Alertmanager fires fleet
   alerts ("site on battery", "backup failing", "shutdown in progress", "site dark N
   days — approaching cert-recovery window").
7. **Fleet UI** — hub-side dynamic plugin over hub CRDs (GA base SDK), registered in
   the ACM Fleet Management perspective; Grafana dashboards.

## 7. API / CRD design

New hub-scoped CRDs and spoke CRDs. Shapes are illustrative, not final.

### 7.1 `FleetShutdown` (hub) — imperative fleet operation

```yaml
spec:
  clusterSelector:
    placementRef: { name: edge-tier-1 }
  reason: "planned maintenance window 2026-08-10"
  dryRun: false
  shutdownPolicy:
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
spoke (GitOps for planned ops, or a direct hub path for live incident response — see
§15).

### 7.2 `FleetPowerStatus` (hub) — aggregated current state (one per managed cluster)

```yaml
status:
  clusterName: site-042
  reachable: true             # from ManagedCluster lease
  powerStatus: OnBattery      # Normal|OnBattery|ShuttingDown|Down|Recovering
  batterySecondsRemaining: 480
  darkSinceTimestamp: "..."   # for dark-duration / breach-window computation
  lastBackup: { time: "...", result: Success }
  shutdownPhase: ""
  recovery: { phase: "", nodesReady: 0, nodesExpected: 0 }
  breachRisk: false           # dark site approaching node-cert window (~90d)
  source: metrics+lease
```

Materialized by the hub controller from federated metrics + ManagedCluster lease.
Also the **GA data source for the fleet UI**.

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
    - name: shared-services
      placementRef: { name: hub-region }
      healthGate: AllClustersReady
      timeoutSeconds: 1800
    - name: tier-1-edge
      dependsOn: [shared-services]
      placementRef: { name: edge-tier-1 }
    - name: tier-2-edge
      dependsOn: [tier-1-edge]
```

### 7.5 Recovery allow-list (optional add-on only)

If the SNO/UPI CSR-approver add-on is deployed, POP writes a **signed on-disk
allow-list** at shutdown: the expected node identities (CN / SAN) as an independent,
pre-captured source. The approver matches CSRs against it (never against
node-self-reported addresses). Not part of the core design.

### 7.6 Per-site power/OOB config

Part of per-cluster GitOps config: `oob: redfish|ipmi|none`, BMC endpoint + credentials
secret ref, per-site power budget. Tells the hub which sites it can actively power on.

## 8. Data flows

### 8.1 Config down
Git commit → ArgoCD ApplicationSet renders per-cluster overlays via Placement generator
→ synced to each spoke → local POP reconciles. Once synced, POP runs standalone.

### 8.2 Status up
POP emits metrics (incl. shutdown timestamp) → MCO federates outbound to hub Thanos →
hub controller materializes `FleetPowerStatus` (+ ManagedCluster lease for liveness) →
fleet UI + alerts (incl. dark-duration breach warnings).

### 8.3 Power-loss shutdown (autonomous)
UPS event (SNMP/IPMI/NUT) → local POP → `ClusterShutdown` → cluster-type-adapted
shutdown with stuck policy + battery-derived hard deadline → safe poweroff. No hub
involvement required.

### 8.4 Black-start / restart
Power returns → (staggered BMC power-on by hub for OOB sites, else BIOS auto) →
**OCP self-heals leaf certs and the API comes up** → IPI: machine-approver approves
node CSRs; SNO/UPI: node CSRs Pending (approved by the optional add-on if deployed,
else by a human) → nodes Ready → `BootSequence` runs workload stages health-gated →
klusterlet reconnects → hub sees `Recovering→Normal`; `FleetBootSequence` gates
cross-cluster tier ordering. Hub raises an alert for any site that does not reach
`Normal` within its deadline.

## 9. Cold-start recovery (re-scoped)

**What OCP does (delegate, do not reimplement):** on cold boot the
`cert-regeneration-controller` re-signs expired leaf certs from the still-valid
signing CAs; the API server starts; operators reissue node kubeconfigs. Works for
outages up to ~1yr (CA/signer-bounded). POP relies on this and never re-signs
control-plane certs. Reimplementing signing is a known trap (legacy
`regenerate-certificates` bug bricks kube-controller-manager).

**The residual gap:** on **SNO / UPI** (no Machine object) the
`cluster-machine-approver` will not auto-approve kubelet CSRs, so nodes sit Pending
after self-heal. IPI clusters recover fully unattended.

**Core approach — prevention, not a node agent:**
- The hub knows each site's shutdown timestamp, so it computes **dark duration** and
  **alerts before the node-cert window** (~90d) so a site is serviced before it
  becomes unrecoverable.
- The rare breach is flagged "needs manual CSR approval" with an exact runbook
  (`oc adm certificate approve`) — a one-command, per-site, infrequent action.
- **CA/signer expiry (>~1yr) and etcd-quorum / multi-node cert surgery are out of
  scope** — detect-and-refuse + alert (matches Red Hat's manual-only etcd DR
  posture). Automated CA re-issue is not built (catastrophic trust-cascade).

**Optional add-on — secure SNO/UPI CSR-approver (default off):** for the *unmanned*
SNO/UPI subset where no human can reach a dark site within the window. Constraints:
- Node-level static pod via MachineConfig; poll-until-API-up (acts after OCP
  self-heal, with full TLS verification against the on-disk trusted CA bundle — never
  `--insecure`).
- **Safe matching only:** client CSRs by CN/group/usage; serving CSRs by
  renewal-continuity (new SANs exactly equal the node's existing valid cert) or the
  operator-signed allow-list (§7.5). **Never** node-self-reported addresses.
- **Detector-first, bail-to-human** on: etcd quorum loss, multi-node cert expiry,
  CA/signer expiry, past-window node-kubeconfig expiry, any regeneration-controller
  anomaly. Rides OCP's existing pinned-image mechanism for offline availability.
- Prototype gates before shipping: confirm expiry ordering empirically (day
  20/45/100); validate admin-from-disk kubeconfig with full TLS verification;
  prove secure serving-CSR approval AND refusal of a non-matching CSR on SNO;
  static-pod boot race safety; offline image across upgrade; SELinux/mount.

**Immediate security fix (independent of the above):** POP's shipped blanket approver
(`internal/controller/clusterrestart_controller.go:210-237`) approves any kubelet CSR
with no identity check. Replace with the safe matching model (client CSRs by
CN/group/usage; serving CSRs by renewal-continuity or allow-list; else leave Pending).

## 10. Failure handling

- **Stuck phase (shutdown or black-start):** each phase has timeout + retries +
  `onStuck` (`force|skip|haltAndAlert|escalateToHub`). A global **hard deadline**
  (battery-derived for power events) overrides everything and force-progresses to a
  safe poweroff. **Default for power-triggered = force-progress on deadline** (NFR3).
- **Hub down:** spokes fully autonomous; config already synced; status resumes when
  hub returns.
- **Spoke disconnected / NAT:** status via outbound metrics only; hub marks
  `reachable:false` on missed lease.
- **Cold-start / cert window:** hub dark-duration alert before ~90d; OCP self-heals
  within ~1yr; CA/signer expiry → detect-and-refuse + manual rebuild.
- **Partial recovery:** hub closure-check flags sites where `nodesReady <
  nodesExpected` for human attention.
- **Power-on inrush:** `maxConcurrent` + `staggerSeconds` bound simultaneous power-on.

## 11. Security & RBAC

- No node-local cert authority in the core design. If the optional CSR-approver
  add-on is deployed: minimal, node-local, default-off; approval scoped to the safe
  matching model + bounded window; full TLS verification; on-disk material protected
  by node file perms.
- Hub BMC/Redfish credentials: per-site secrets, referenced not inlined.
- Status channel: outbound metrics only; no hub→spoke pull requirement for safety.
- `cluster-proxy-addon` (GA) for any hub→spoke console access; `managed-serviceaccount`
  (beta) only where reachable and non-safety-critical.

## 12. UI approach

- **Phase 1 (GA):** hub-side dynamic plugin over `FleetPowerStatus` / `FleetShutdown`
  / `FleetBootSequence` using the base SDK POP already ships; register pages/nav and
  target the ACM Fleet Management perspective by ID. Grafana fleet dashboards via MCO.
- **Phase 2 (optional, preview):** spoke drill-down via `@stolostron/multicluster-sdk`,
  **gated by `useIsFleetAvailable` with hub-CRD fallback**. Never a hard dependency.

## 13. Phasing (decomposition into sub-specs)

Larger than one implementation plan. Each phase gets its own spec → plan.

0. **Security hotfix (now)** — replace the blanket CSR approver with the safe matching
   model (`clusterrestart_controller.go`). Independent of the fleet work.
1. **Spoke core** — cluster-type-aware shutdown + intra-cluster `BootSequence`.
2. **Hub fleet layer** — GitOps delivery, `FleetPowerStatus` + `FleetShutdown`,
   observability, dark-duration breach alerting, hub-CRD fleet UI.
3. **Fleet black-start** — `FleetBootSequence` cross-cluster ordering + staggered
   BMC/Redfish power-on.
4. **Optional add-ons** — secure SNO/UPI CSR-approver (+ signed allow-list) for the
   unmanned subset; spoke drill-down UI; cross-cluster DR.

## 14. References

- Research: Veeam/Kasten vs POP (control-plane/etcd overlap).
- Research: ecosystem cold-start / cert handling (leaf vs CA cert classes).
- Research: ACM console multicluster UI extensibility (2026 GA vs preview).
- Spike: OCP cold-start cert internals (self-heal, ~1yr bound, SNO/UPI CSR gap,
  machine-approver identity model, static-pod delivery). Verdict: delegate cert
  regeneration to OCP; boot-agent only as an optional secure SNO/UPI CSR approver.
- OpenShift: graceful shutdown/restart; expired-cert recovery; `cert-regeneration-
  controller`; `cluster-machine-approver` (`csr_check.go`); `cluster-backup.sh`.
- ACM: ApplicationSet + Placement; MCO; `cluster-proxy-addon`;
  `managed-serviceaccount`; Fleet Management perspective.
- `@stolostron/multicluster-sdk` v0.10.3 (preview).

## 15. Open questions / risks

- Empirically confirm OCP cert/signer expiry ordering and the exact recovery window
  before stating hard day-counts (prototype gate).
- Whether live `FleetShutdown` fan-out should ever bypass GitOps for incident speed,
  or always route through git for auditability.
- Sizing of the unmanned SNO/UPI subset — whether the optional CSR-approver add-on is
  worth building at all, or prevention + manual runbook suffices.
- `@stolostron/multicluster-sdk` maturing to GA before we depend on drill-down.
