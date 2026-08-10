# FleetBootSequence — Health-Gated Tier Ordering (Phase 3a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A hub controller that walks a declarative `FleetBootSequence` of cluster tiers in dependency order, marking each tier ready only when all its clusters report `ManagedClusterConditionAvailable`, so fleet black-start progresses predictably (shared-services clusters before dependent edge). Zero BMC access — works for the entire fleet.

**Architecture:** Runs in the existing `cmd/hub` binary. Reuses the Placement→PlacementDecision resolution from the FleetShutdown controller. Availability comes from `ManagedCluster.status.conditions[type=ManagedClusterConditionAvailable]` (`open-cluster-management.io/api/cluster/v1`). This is the universal core; hub-driven power-on is a separate, prototype-gated add-on (Phase 3b, not this plan — see design revision note 2026-08-10).

**Tech Stack:** Go, controller-runtime, `open-cluster-management.io/api` (`cluster/v1`, `cluster/v1beta1`), envtest with vendored ACM CRDs (already present in `test/acm-crds/`).

**Scope boundary:** This plan does NOT power anything on. It observes availability and enforces a dependency-ordered readiness barrier + status. Actuation (Metal3/Redfish power-on) is Phase 3b.

---

## File structure

- `api/v1alpha1/fleetbootsequence_types.go` — CRD types. (Create)
- `internal/hub/availability.go` — ManagedCluster availability helper. (Create)
- `internal/hub/availability_test.go` — unit test (fake client). (Create)
- `internal/hub/fleetbootsequence_controller.go` — the controller. (Create)
- `internal/hub/fleetbootsequence_controller_test.go` — envtest specs. (Create)
- `cmd/hub/main.go` — register the new reconciler + `cluster/v1` scheme. (Modify)
- `config/hub/role.yaml`, `test/acm-crds/` — add fleetbootsequences perms; ManagedCluster CRD already vendored. (Modify)

---

## Task 1: FleetBootSequence CRD types

**Files:** Create `api/v1alpha1/fleetbootsequence_types.go`

- [ ] **Step 1: Write the types**

```go
package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type FleetBootSequencePhase string

const (
	FleetBootSequencePhasePending    FleetBootSequencePhase = "Pending"
	FleetBootSequencePhaseInProgress FleetBootSequencePhase = "InProgress"
	FleetBootSequencePhaseComplete   FleetBootSequencePhase = "Complete"
	FleetBootSequencePhaseTimedOut   FleetBootSequencePhase = "TimedOut"
)

// FleetBootTier is one ordered stage of the fleet black-start.
type FleetBootTier struct {
	// Name is a unique identifier for the tier.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// PlacementRef names an ACM Placement (same namespace) selecting this tier's clusters.
	// +kubebuilder:validation:Required
	PlacementRef string `json:"placementRef"`

	// DependsOn lists tier names that must be Ready before this tier is evaluated.
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// TimeoutSeconds bounds how long to wait for this tier to become Ready before
	// the sequence is marked TimedOut.
	// +kubebuilder:default=1800
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
}

type FleetBootSequenceSpec struct {
	// Tiers are the ordered stages. Ordering is by DependsOn (a DAG), not list order.
	// +kubebuilder:validation:MinItems=1
	Tiers []FleetBootTier `json:"tiers"`
}

type FleetBootTierStatus struct {
	Name             string `json:"name"`
	Ready            bool   `json:"ready"`
	ClustersTotal    int32  `json:"clustersTotal"`
	ClustersReady    int32  `json:"clustersReady"`
	Message          string `json:"message,omitempty"`
}

type FleetBootSequenceStatus struct {
	// +kubebuilder:validation:Enum=Pending;InProgress;Complete;TimedOut
	Phase              FleetBootSequencePhase `json:"phase,omitempty"`
	Tiers              []FleetBootTierStatus  `json:"tiers,omitempty"`
	StartTime          *metav1.Time           `json:"startTime,omitempty"`
	ObservedGeneration int64                  `json:"observedGeneration,omitempty"`
	Message            string                 `json:"message,omitempty"`
	Conditions         []metav1.Condition     `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

type FleetBootSequence struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetBootSequenceSpec   `json:"spec,omitempty"`
	Status            FleetBootSequenceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type FleetBootSequenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetBootSequence `json:"items"`
}

func init() { SchemeBuilder.Register(&FleetBootSequence{}, &FleetBootSequenceList{}) }
```

- [ ] **Step 2: Regenerate** — `make manifests generate`. Expected: `config/crd/bases/pop.openshift.io_fleetbootsequences.yaml` created; deepcopy compiled; build clean.

- [ ] **Step 3: Commit** — `git commit -am "feat(hub): add FleetBootSequence CRD types"`

## Task 2: ManagedCluster availability helper

**Files:** Create `internal/hub/availability.go`, `internal/hub/availability_test.go`

- [ ] **Step 1: Write the failing unit test** (fake client, no envtest)

```go
package hub

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestClusterAvailable(t *testing.T) {
	s := runtime.NewScheme()
	_ = clusterv1.Install(s)
	mc := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "site-a"}}
	mc.Status.Conditions = []metav1.Condition{{
		Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue,
		Reason: "Ok", LastTransitionTime: metav1.Now(),
	}}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(mc).Build()

	ok, err := clusterAvailable(context.Background(), c, "site-a")
	if err != nil || !ok {
		t.Fatalf("clusterAvailable(site-a) = %v,%v want true,nil", ok, err)
	}
	ok, _ = clusterAvailable(context.Background(), c, "missing")
	if ok {
		t.Fatalf("clusterAvailable(missing) = true, want false")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/hub/ -run TestClusterAvailable` → `clusterAvailable` undefined.

- [ ] **Step 3: Implement**

```go
package hub

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// clusterAvailable reports whether the named ManagedCluster has
// ManagedClusterConditionAvailable == True. A missing cluster is not available.
func clusterAvailable(ctx context.Context, c client.Client, name string) (bool, error) {
	mc := &clusterv1.ManagedCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: name}, mc); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return meta.IsStatusConditionTrue(mc.Status.Conditions, clusterv1.ManagedClusterConditionAvailable), nil
}
```

- [ ] **Step 4: Run to verify pass. Step 5: Commit** — `git commit -am "feat(hub): ManagedCluster availability helper"`

## Task 3: FleetBootSequence controller

**Files:** Create `internal/hub/fleetbootsequence_controller.go`, `internal/hub/fleetbootsequence_controller_test.go`

- [ ] **Step 1: Write the failing envtest spec**

Create a `FleetBootSequence` with two tiers: `shared` (placement selecting `hub-a`) and `edge` (dependsOn `shared`, placement selecting `edge-a`). Create the PlacementDecisions and ManagedClusters. Initially only `hub-a` Available → reconcile → assert `shared` tier Ready, `edge` not (dependency met but its cluster not available yet), phase InProgress. Then mark `edge-a` Available → reconcile → assert both tiers Ready, phase Complete.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement the controller**

Reconcile algorithm (complete code in the file):
1. Get `FleetBootSequence`; set `ObservedGeneration`, `StartTime` if unset; terminal (Complete/TimedOut) → return.
2. Build a map of tier name → readiness (computed this pass).
3. Process tiers whose `DependsOn` are all already Ready (topological walk; detect cycles → fail with a clear message).
4. For each such tier: `resolvePlacement` (reuse from the FleetShutdown controller) → cluster names; count how many are `clusterAvailable`. Tier Ready when all selected clusters are available (and ≥1 selected). Record `FleetBootTierStatus`.
5. Overall: `Complete` when all tiers Ready; `TimedOut` if any unready tier exceeded its `TimeoutSeconds` since `StartTime`; else `InProgress`, requeue after 30s.
6. Update status.

Reuse `resolvePlacement` (already defined on `FleetShutdownReconciler`) by extracting it to a package function `resolvePlacementDecisions(ctx, c, ns, name)` in `manifestwork.go` or a shared file, and have both controllers call it. (Refactor: move the method body to a free function; keep a thin method wrapper for the FleetShutdown controller.)

- [ ] **Step 4: Run to verify pass. Step 5: Commit** — `git commit -am "feat(hub): FleetBootSequence health-gated tier ordering controller"`

## Task 4: Wire into cmd/hub + RBAC

**Files:** Modify `cmd/hub/main.go`, `config/hub/role.yaml`

- [ ] **Step 1: Register in cmd/hub**

Add `clusterv1.Install(scheme)` to the hub `init()`, and set up `FleetBootSequenceReconciler` alongside `FleetShutdownReconciler`.

- [ ] **Step 2: Add RBAC**

In `config/hub/role.yaml` add a rule for `fleetbootsequences` + `fleetbootsequences/status` (the `managedclusters`/`placements`/`placementdecisions` read rules already exist).

```yaml
  - apiGroups: ["pop.openshift.io"]
    resources: ["fleetbootsequences"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["pop.openshift.io"]
    resources: ["fleetbootsequences/status"]
    verbs: ["get", "update", "patch"]
```

- [ ] **Step 3: Build + full suite** — `go build ./... && make test` (hub suite includes the new specs). `make lint`.

- [ ] **Step 4: Commit** — `git commit -am "feat(hub): run FleetBootSequence controller in cmd/hub + RBAC"`

---

## Self-review

- **Spec coverage:** implements FR9's ordering half and the Phase-3a "universal, zero-BMC health-gated tier ordering" from the 2026-08-10 revision note. Power-on (3b) is explicitly out of scope.
- **Placeholders:** CRD types, availability helper, and its test are complete code; the controller reconcile is specified as a precise algorithm + the resolvePlacement refactor (fill in following the FleetShutdown controller patterns during execution).
- **Type consistency:** `FleetBootSequence*` types, `clusterAvailable`, and the reused `resolvePlacement` are consistent; `cluster/v1` (ManagedCluster) added to the hub scheme + envtest (CRD already vendored).

## Risks / notes

- Availability latency: `ManagedClusterConditionAvailable` flips ~lease interval (~60s) after a spoke's klusterlet reconnects; tier timeouts must account for boot + reconnect time.
- Cycle detection in `DependsOn` is required (fail fast with a clear message).
- This is observational/gating only; a tier becoming Ready releases *dependent tiers'* evaluation (and, in Phase 3b, their power-on). It does not itself power or apply anything.
