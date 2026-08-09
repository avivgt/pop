# FleetShutdown Orchestration (Phase 2b) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A hub-side operator that reconciles a GitOps-authored `FleetShutdown` CR by selecting managed clusters via ACM `Placement` and fanning out a per-spoke `ClusterShutdown` through self-cleaning `ManifestWork`, aggregating per-cluster progress via ManifestWork status feedback.

**Architecture:** New `cmd/hub` binary (second deployable in the same repo; shares `api/`). It runs on the ACM hub only. GitOps applies the `FleetShutdown` CR (intent); the controller resolves `Placement` → `PlacementDecision` for the target clusters, creates one `ManifestWork` per cluster (in that cluster's hub namespace) wrapping a `ClusterShutdown`, and reads the wrapped resource's `status.phase` back through `feedbackRules` to populate `FleetShutdown.status.clusters[]`. Completion deletes the ManifestWork (self-cleaning; no re-trigger loop). GitOps owns intent; ManifestWork owns the transient action.

**Tech Stack:** Go, controller-runtime, `open-cluster-management.io/api` (`work/v1`, `cluster/v1beta1`, `cluster/v1`), envtest with ACM CRDs.

**Verified API facts (2026-08-09 spike):**
- ManifestWork: `open-cluster-management.io/api/work/v1`, GVK `work.open-cluster-management.io/v1, ManifestWork`. Created in the **per-cluster hub namespace** (namespace == managed cluster name). `spec.workload.manifests[]` holds raw objects; `spec.manifestConfigs[].feedbackRules[]` (type `JSONPaths`) surface applied-resource status into `status.resourceStatus.manifests[].statusFeedbacks`.
- Placement/PlacementDecision: `open-cluster-management.io/api/cluster/v1beta1`. `PlacementDecision`s are labeled `cluster.open-cluster-management.io/placement=<placementName>`; `status.decisions[].clusterName` lists selected clusters.
- Envtest: install ACM CRDs from the `open-cluster-management.io/api` module (vendored CRD YAML) alongside POP CRDs.

---

## File structure

- `api/v1alpha1/fleetshutdown_types.go` — `FleetShutdown` CRD types. (Create)
- `cmd/hub/main.go` — hub manager binary. (Create)
- `internal/hub/fleetshutdown_controller.go` — the controller + helpers. (Create)
- `internal/hub/manifestwork.go` — ManifestWork build/parse helpers. (Create)
- `internal/hub/fleetshutdown_controller_test.go` — envtest specs. (Create)
- `internal/hub/manifestwork_test.go` — pure unit tests for the helpers. (Create)
- `hack/get-acm-crds.sh` — fetch ACM CRD YAML for envtest. (Create)
- `Containerfile.hub`, `Makefile` targets, `config/hub/**` (RBAC + deployment). (Create/Modify)

---

## Task 1: Add ACM dependencies and vendor CRDs for envtest

**Files:** `go.mod`, `hack/get-acm-crds.sh`, `test/acm-crds/` (downloaded)

- [ ] **Step 1: Add the module dependency**

Run: `go get open-cluster-management.io/api@latest`
Expected: `go.mod` gains `open-cluster-management.io/api vX.Y.Z`.

- [ ] **Step 2: Write the CRD fetch script**

Create `hack/get-acm-crds.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
# Vendors the ACM CRDs needed for envtest from the open-cluster-management.io/api module.
DEST="test/acm-crds"
mkdir -p "${DEST}"
MOD_DIR="$(go list -m -f '{{.Dir}}' open-cluster-management.io/api)"
cp "${MOD_DIR}/work/v1/0000_00_work.open-cluster-management.io_manifestworks.crd.yaml" "${DEST}/"
cp "${MOD_DIR}/cluster/v1/0000_00_clusters.open-cluster-management.io_managedclusters.crd.yaml" "${DEST}/"
cp "${MOD_DIR}/cluster/v1beta1/0000_02_clusters.open-cluster-management.io_placementdecisions.crd.yaml" "${DEST}/"
cp "${MOD_DIR}/cluster/v1beta1/0000_02_clusters.open-cluster-management.io_placements.crd.yaml" "${DEST}/"
echo "Vendored ACM CRDs into ${DEST}"
```

- [ ] **Step 3: Run it and verify**

Run: `chmod +x hack/get-acm-crds.sh && ./hack/get-acm-crds.sh && ls test/acm-crds`
Expected: four CRD YAML files present. (If a path differs by module version, `find "${MOD_DIR}" -name '*manifestworks*crd.yaml'` to locate; update the script.)

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum hack/get-acm-crds.sh test/acm-crds
git commit -m "build(hub): add open-cluster-management.io/api dep and vendor ACM CRDs for envtest"
```

## Task 2: FleetShutdown CRD types

**Files:** Create `api/v1alpha1/fleetshutdown_types.go`

- [ ] **Step 1: Write the types**

```go
package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type FleetShutdownPhase string

const (
	FleetShutdownPhasePending         FleetShutdownPhase = "Pending"
	FleetShutdownPhaseInProgress      FleetShutdownPhase = "InProgress"
	FleetShutdownPhaseCompleted       FleetShutdownPhase = "Completed"
	FleetShutdownPhasePartiallyFailed FleetShutdownPhase = "PartiallyFailed"
)

// FleetShutdownPolicy is inherited by each per-spoke ClusterShutdown.
type FleetShutdownPolicy struct {
	// +kubebuilder:validation:Enum=force;skip;haltAndAlert;escalateToHub
	// +kubebuilder:default=force
	// +optional
	OnStuck string `json:"onStuck,omitempty"`
	// +optional
	HardDeadlineSeconds int32 `json:"hardDeadlineSeconds,omitempty"`
}

type FleetShutdownSpec struct {
	// PlacementRef names an ACM Placement (same namespace) selecting target clusters.
	// +kubebuilder:validation:Required
	PlacementRef string `json:"placementRef"`

	// PVCName is passed through to each spoke ClusterShutdown.
	// +kubebuilder:validation:Required
	PVCName string `json:"pvcName"`

	// +optional
	Reason string `json:"reason,omitempty"`
	// +kubebuilder:default=false
	// +optional
	DryRun bool `json:"dryRun,omitempty"`
	// +optional
	ShutdownPolicy *FleetShutdownPolicy `json:"shutdownPolicy,omitempty"`
}

type FleetClusterStatus struct {
	Name    string `json:"name"`
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

type FleetShutdownStatus struct {
	// +kubebuilder:validation:Enum=Pending;InProgress;Completed;PartiallyFailed
	Phase              FleetShutdownPhase   `json:"phase,omitempty"`
	Clusters           []FleetClusterStatus `json:"clusters,omitempty"`
	SelectedClusters   int32                `json:"selectedClusters,omitempty"`
	CompletedClusters  int32                `json:"completedClusters,omitempty"`
	ObservedGeneration int64                `json:"observedGeneration,omitempty"`
	Message            string               `json:"message,omitempty"`
	Conditions         []metav1.Condition   `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Selected",type=integer,JSONPath=`.status.selectedClusters`
// +kubebuilder:printcolumn:name="Completed",type=integer,JSONPath=`.status.completedClusters`

type FleetShutdown struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetShutdownSpec   `json:"spec,omitempty"`
	Status            FleetShutdownStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type FleetShutdownList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetShutdown `json:"items"`
}

func init() { SchemeBuilder.Register(&FleetShutdown{}, &FleetShutdownList{}) }
```

- [ ] **Step 2: Regenerate**

Run: `make manifests generate`
Expected: `config/crd/bases/pop.openshift.io_fleetshutdowns.yaml` created; `zz_generated.deepcopy.go` gains FleetShutdown deepcopy funcs; build clean.

- [ ] **Step 3: Commit**

```bash
git add api/v1alpha1/fleetshutdown_types.go api/v1alpha1/zz_generated.deepcopy.go config/crd/bases/pop.openshift.io_fleetshutdowns.yaml
git commit -m "feat(hub): add FleetShutdown CRD types"
```

## Task 3: ManifestWork build/parse helpers (pure unit-tested)

**Files:** Create `internal/hub/manifestwork.go`, `internal/hub/manifestwork_test.go`

- [ ] **Step 1: Write failing unit tests**

`internal/hub/manifestwork_test.go` builds a ManifestWork wrapping a ClusterShutdown and asserts the wrapped manifest + feedbackRule, then parses a synthetic status feedback back to a phase string.

```go
package hub

import (
	"testing"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"
)

func TestBuildManifestWork(t *testing.T) {
	cs := &popv1alpha1.ClusterShutdown{}
	cs.Name = "fleet-shutdown-1"
	cs.Namespace = "pop-system"
	cs.Spec.PVCName = "etcd-backup"

	mw, err := buildManifestWork("site-042", "fs-1", cs)
	if err != nil {
		t.Fatalf("buildManifestWork: %v", err)
	}
	if mw.Namespace != "site-042" {
		t.Errorf("ManifestWork namespace = %q, want site-042 (per-cluster hub ns)", mw.Namespace)
	}
	if len(mw.Spec.Workload.Manifests) != 1 {
		t.Fatalf("want 1 wrapped manifest, got %d", len(mw.Spec.Workload.Manifests))
	}
	if len(mw.Spec.ManifestConfigs) != 1 || len(mw.Spec.ManifestConfigs[0].FeedbackRules) == 0 {
		t.Fatalf("expected a feedbackRule for status.phase")
	}
}

func TestPhaseFromFeedback(t *testing.T) {
	mw := &workv1.ManifestWork{}
	mw.Status.ResourceStatus.Manifests = []workv1.ManifestCondition{{
		StatusFeedbacks: workv1.StatusFeedbackResult{
			Values: []workv1.FeedbackValue{{
				Name:  "phase",
				Value: workv1.FieldValue{Type: workv1.String, String: strptr("Complete")},
			}},
		},
	}}
	if got := phaseFromFeedback(mw); got != "Complete" {
		t.Fatalf("phaseFromFeedback = %q, want Complete", got)
	}
}

func strptr(s string) *string { return &s }
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/hub/ -run 'TestBuildManifestWork|TestPhaseFromFeedback' -v`
Expected: FAIL — `buildManifestWork` / `phaseFromFeedback` undefined.

- [ ] **Step 3: Implement the helpers**

`internal/hub/manifestwork.go`:

```go
package hub

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const manifestWorkLabel = "pop.openshift.io/fleetshutdown"

// buildManifestWork wraps a ClusterShutdown in a ManifestWork targeted at the
// given managed cluster (ManifestWork lives in the per-cluster hub namespace),
// with a feedbackRule that surfaces the wrapped ClusterShutdown's status.phase.
func buildManifestWork(clusterName, fleetShutdownName string, cs *popv1alpha1.ClusterShutdown) (*workv1.ManifestWork, error) {
	cs.APIVersion = popv1alpha1.GroupVersion.String()
	cs.Kind = "ClusterShutdown"
	raw, err := json.Marshal(cs)
	if err != nil {
		return nil, err
	}
	return &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pop-shutdown-" + fleetShutdownName,
			Namespace: clusterName,
			Labels:    map[string]string{manifestWorkLabel: fleetShutdownName},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{{RawExtension: runtime.RawExtension{Raw: raw}}},
			},
			ManifestConfigs: []workv1.ManifestConfigOption{{
				ResourceIdentifier: workv1.ResourceIdentifier{
					Group:     popv1alpha1.GroupVersion.Group,
					Resource:  "clustershutdowns",
					Namespace: cs.Namespace,
					Name:      cs.Name,
				},
				FeedbackRules: []workv1.FeedbackRule{{
					Type: workv1.JSONPathsType,
					JsonPaths: []workv1.JsonPath{{
						Name: "phase",
						Path: ".status.phase",
					}},
				}},
			}},
		},
	}, nil
}

// phaseFromFeedback extracts the wrapped ClusterShutdown's status.phase from the
// ManifestWork status feedback, or "" if not yet reported.
func phaseFromFeedback(mw *workv1.ManifestWork) string {
	for _, m := range mw.Status.ResourceStatus.Manifests {
		for _, v := range m.StatusFeedbacks.Values {
			if v.Name == "phase" && v.Value.String != nil {
				return *v.Value.String
			}
		}
	}
	return ""
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/hub/ -run 'TestBuildManifestWork|TestPhaseFromFeedback' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hub/manifestwork.go internal/hub/manifestwork_test.go
git commit -m "feat(hub): ManifestWork build + status-feedback parse helpers"
```

## Task 4: Placement resolution helper

**Files:** Modify `internal/hub/fleetshutdown_controller.go` (new file this task), test in `internal/hub/fleetshutdown_controller_test.go`

- [ ] **Step 1: Write the failing envtest spec**

Add to `internal/hub/fleetshutdown_controller_test.go` a spec that creates a `Placement` + a `PlacementDecision` (labeled `cluster.open-cluster-management.io/placement=<name>`, with `status.decisions[].clusterName`) and asserts `resolvePlacement` returns those cluster names. (Suite setup in Task 7 registers the ACM CRDs.)

```go
It("resolves selected clusters from PlacementDecision", func() {
	// create Placement "fs-pl" and a PlacementDecision with two decisions...
	names, err := r.resolvePlacement(ctx, "default", "fs-pl")
	Expect(err).NotTo(HaveOccurred())
	Expect(names).To(ConsistOf("site-a", "site-b"))
})
```

- [ ] **Step 2: Run to verify it fails** — `resolvePlacement` undefined.

- [ ] **Step 3: Implement `resolvePlacement`**

```go
func (r *FleetShutdownReconciler) resolvePlacement(ctx context.Context, ns, name string) ([]string, error) {
	pdList := &clusterv1beta1.PlacementDecisionList{}
	if err := r.List(ctx, pdList,
		client.InNamespace(ns),
		client.MatchingLabels{"cluster.open-cluster-management.io/placement": name},
	); err != nil {
		return nil, err
	}
	var names []string
	for i := range pdList.Items {
		for _, d := range pdList.Items[i].Status.Decisions {
			names = append(names, d.ClusterName)
		}
	}
	return names, nil
}
```

- [ ] **Step 4: Run to verify pass. Step 5: Commit.**

```bash
git commit -am "feat(hub): resolve target clusters from Placement decisions"
```

## Task 5: Reconcile — fan out, aggregate, complete

**Files:** Modify `internal/hub/fleetshutdown_controller.go`, test in `internal/hub/fleetshutdown_controller_test.go`

- [ ] **Step 1: Write the failing envtest spec**

Create a `FleetShutdown` referencing a Placement that selects `site-a`/`site-b`; reconcile; assert one `ManifestWork` exists in each cluster namespace wrapping a `ClusterShutdown`, and `FleetShutdown.status.selectedClusters == 2`, phase `InProgress`. Then patch each ManifestWork's status feedback to `Complete`, reconcile, assert `FleetShutdown.status.phase == Completed` and the ManifestWorks are deleted.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement Reconcile**

Reconcile logic (complete code in the controller file):
1. Get `FleetShutdown`; set `ObservedGeneration`; if terminal, return.
2. `names, _ := r.resolvePlacement(...)`; set `SelectedClusters`.
3. For each cluster: build a `ClusterShutdown` (name `<fleetshutdown>`, namespace = the spoke POP namespace, spec from `FleetShutdown.spec`: PVCName, DryRun, Reason, and map `ShutdownPolicy`); `buildManifestWork`; create-or-noop the ManifestWork in the cluster namespace.
4. Read each ManifestWork's `phaseFromFeedback`; write `status.clusters[]`; count `Complete`.
5. If all clusters report `Complete`: set phase `Completed`, delete the ManifestWorks (self-cleaning), set the `Ready` condition. Else phase `InProgress`, requeue after 15s.
6. `PartiallyFailed` if any wrapped phase is `Failed`.

- [ ] **Step 4: Run to verify pass. Step 5: Commit.**

```bash
git commit -am "feat(hub): FleetShutdown reconcile - ManifestWork fan-out, status aggregation, cleanup"
```

## Task 6: cmd/hub binary + build/deploy wiring

**Files:** Create `cmd/hub/main.go`, `Containerfile.hub`; modify `Makefile`; create `config/hub/**`

- [ ] **Step 1: Write `cmd/hub/main.go`**

A controller-runtime manager (mirroring `cmd/main.go`) whose scheme registers `popv1alpha1`, `workv1`, `clusterv1`, `clusterv1beta1`; sets up only `FleetShutdownReconciler`; leader election id `pop-hub.openshift.io`.

- [ ] **Step 2: Build**

Run: `go build ./cmd/hub/...`
Expected: builds clean.

- [ ] **Step 3: Add Makefile targets + Containerfile.hub**

`Containerfile.hub` mirrors `Containerfile` but `go build ./cmd/hub`. Makefile: `docker-build-hub`, `docker-push-hub` using `HUB_IMG ?= $(IMAGE_TAG_BASE)-hub:v$(VERSION)`.

- [ ] **Step 4: RBAC for the hub**

`config/hub/` with RBAC: get/list/watch `fleetshutdowns` (+status), `placements`/`placementdecisions`/`managedclusters` (get;list;watch), `manifestworks` (get;list;watch;create;update;delete). Generate via `+kubebuilder:rbac` markers on the reconciler + `make manifests`.

- [ ] **Step 5: Commit**

```bash
git add cmd/hub Containerfile.hub Makefile config/hub
git commit -m "build(hub): cmd/hub binary, Containerfile.hub, Makefile targets, hub RBAC"
```

## Task 7: envtest suite with ACM CRDs

**Files:** Create `internal/hub/suite_test.go`

- [ ] **Step 1: Write the suite**

Mirror `internal/controller/suite_test.go` but set `CRDDirectoryPaths` to both `config/crd/bases` (POP) and `test/acm-crds` (ACM), and register `workv1`, `clusterv1`, `clusterv1beta1` into the scheme. This makes Tasks 4–5 specs runnable.

- [ ] **Step 2: Run the hub suite**

Run: `KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.31.0 --bin-dir bin -p path)" go test ./internal/hub/... -v` (or `make test` once the `test` target includes `./internal/hub/...`).
Expected: all hub specs pass.

- [ ] **Step 3: Wire `./internal/hub/...` into `make test`** (it already globs `./...` minus e2e, so ensure the ACM CRD vendor step runs first; add `get-acm-crds.sh` invocation to the `test` target prerequisite or document running it once).

- [ ] **Step 4: Commit**

```bash
git add internal/hub/suite_test.go Makefile
git commit -m "test(hub): envtest suite with ACM CRDs for the FleetShutdown controller"
```

---

## Self-review

- **Spec coverage:** implements FR8 (hub-orchestrated FleetShutdown), the two-binary packaging decision (`cmd/hub`), and the GitOps-intent + ManifestWork fan-out model. Uses verified API paths/GVKs.
- **Placeholders:** the critical/non-obvious code (types, ManifestWork build + feedbackRules, feedback parse, Placement resolution) is complete; Tasks 5–7 give the reconcile algorithm, binary, and suite structure to fill following the shown patterns. Tighten any remaining prose into code during execution (subagent-driven).
- **Type consistency:** `buildManifestWork`/`phaseFromFeedback`/`resolvePlacement`/`FleetShutdownReconciler` and the `FleetShutdown*` types are used consistently across tasks; ACM types via `workv1`/`clusterv1beta1` aliases.

## Risks / gates (from the spike)

- Confirm the exact ACM CRD file paths in the pinned module version (Task 1 Step 3 fallback).
- `feedbackRules` only surface fields the work-agent is configured to read; verify `.status.phase` feedback actually populates against a real spoke before relying on completion detection (envtest simulates it by patching status).
- ManifestWork applies the wrapped `ClusterShutdown` on the spoke — the spoke POP + CRD must already be installed there (delivered via GitOps, Phase 2 config). Document this ordering.
- E2E (real hub + spokes) is gated on hub access; this plan is build + envtest-verified.
