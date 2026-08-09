# Cluster-Type-Aware Shutdown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect whether a cluster is single-node (SNO) or multi-node and skip the pod-eviction drain phase on single-node clusters, where there are no peer nodes to drain to.

**Architecture:** Extend the existing `ClusterShutdown` state machine. `reconcileValidating` already enumerates nodes; record a detected `ClusterType` in status there. `reconcileDraining` then branches: multi-node keeps the existing PDB-aware eviction; single-node skips eviction (evicting pods on the only node just deletes them with nowhere to go — pointless and disruptive before the etcd backup) and advances straight to the next phase.

**Tech Stack:** Go, controller-runtime, kubebuilder, Ginkgo/Gomega + envtest.

---

## Decision embedded in this plan (confirm before executing)

**How to detect single-node:** this plan uses **node count** (exactly one enumerated Ready node ⇒ SingleNode). Rationale: zero new dependencies, uses data `reconcileValidating` already gathers, and the fleet scope is "full OCP + SNO" (SNO is definitionally one node).

**Alternative (not chosen here):** read OpenShift `Infrastructure.status.controlPlaneTopology` (`SingleReplica` ⇒ SNO). More authoritative and cleaner to isolate in tests, but adds a `github.com/openshift/api` dependency and requires the `config.openshift.io` CRD in the envtest setup. If you prefer this, it should be its own small task before Task 1; the branch logic in Task 2 is unchanged.

If node-count is acceptable, proceed. Otherwise stop and switch detection to `controlPlaneTopology` first.

---

## File structure

- `api/v1alpha1/clustershutdown_types.go` — add `ClusterType` type, constants, and a `ClusterType` status field. (Modify)
- `internal/controller/clustershutdown_controller.go` — set `ClusterType` in `reconcileValidating`; branch in `reconcileDraining`. (Modify)
- `internal/controller/clustershutdown_controller_test.go` — new specs for detection and single-node drain skip. (Modify)
- `config/crd/bases/pop.openshift.io_clustershutdowns.yaml` — regenerated (do not hand-edit). (Generated)
- `api/v1alpha1/zz_generated.deepcopy.go` — regenerated. (Generated)

---

## Task 1: Detect cluster type in the Validating phase

**Files:**
- Modify: `api/v1alpha1/clustershutdown_types.go`
- Modify: `internal/controller/clustershutdown_controller.go` (`reconcileValidating`, ends ~line 296)
- Test: `internal/controller/clustershutdown_controller_test.go`

- [ ] **Step 1: Add the `ClusterType` type, constants, and status field**

In `api/v1alpha1/clustershutdown_types.go`, after the `ClusterShutdownPhase` const block (after line 37), add:

```go
// ClusterType is the detected cluster topology.
type ClusterType string

const (
	// ClusterTypeSingleNode is a single-node OpenShift cluster (SNO): one node
	// acting as both control-plane and worker. No peer nodes to drain to.
	ClusterTypeSingleNode ClusterType = "SingleNode"
	// ClusterTypeMultiNode is any cluster with more than one node.
	ClusterTypeMultiNode ClusterType = "MultiNode"
)
```

In `ClusterShutdownStatus` (struct starting line 164), add this field after the `TotalNodes` field (line 179):

```go
	// ClusterType is the detected topology: SingleNode (SNO) or MultiNode.
	// Determines whether the drain phase performs pod eviction.
	// +kubebuilder:validation:Enum=SingleNode;MultiNode
	// +optional
	ClusterType ClusterType `json:"clusterType,omitempty"`
```

- [ ] **Step 2: Regenerate CRD manifests and deepcopy**

The new status field must be in the CRD schema or envtest's API server will prune it.

Run: `make manifests generate`
Expected: `config/crd/bases/pop.openshift.io_clustershutdowns.yaml` gains a `clusterType` property under `status`; `zz_generated.deepcopy.go` recompiled with no diff needed (scalar field). No errors.

- [ ] **Step 3: Write the failing tests**

In `internal/controller/clustershutdown_controller_test.go`, add these specs inside the existing `Describe("ClusterShutdown Controller", ...)` block (after the existing specs). They wipe nodes first so the count is deterministic in the shared envtest.

```go
Describe("cluster type detection", func() {
	// wipeNodes removes all nodes so node-count detection is deterministic.
	wipeNodes := func() {
		list := &corev1.NodeList{}
		Expect(k8sClient.List(ctx, list)).To(Succeed())
		for i := range list.Items {
			_ = k8sClient.Delete(ctx, &list.Items[i])
		}
		Eventually(func() int {
			l := &corev1.NodeList{}
			_ = k8sClient.List(ctx, l)
			return len(l.Items)
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(0))
	}

	// driveToValidating creates a shutdown and reconciles until ClusterType is set.
	driveToValidating := func(name string) *popv1alpha1.ClusterShutdown {
		s := &popv1alpha1.ClusterShutdown{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: popv1alpha1.ClusterShutdownSpec{
				PVCName:       "etcd-backup",
				SkipCertCheck: true,
				CordonNodes:   false,
			},
		}
		Expect(k8sClient.Create(ctx, s)).To(Succeed())
		// Reconcile enough times to pass: finalizer add, phase init, Pending, Validating.
		for i := 0; i < 5; i++ {
			_, err := reconcileShutdown(name)
			Expect(err).NotTo(HaveOccurred())
			cur := getShutdown(name)
			if cur.Status.ClusterType != "" {
				return cur
			}
		}
		return getShutdown(name)
	}

	AfterEach(func() {
		wipeNodes()
	})

	It("detects a single-node cluster", func() {
		wipeNodes()
		createReadyNode(ctx, "sno-0", true)
		s := driveToValidating("ct-sno")
		Expect(s.Status.ClusterType).To(Equal(popv1alpha1.ClusterTypeSingleNode))
		Expect(s.Status.TotalNodes).To(Equal(int32(1)))
	})

	It("detects a multi-node cluster", func() {
		wipeNodes()
		createReadyNode(ctx, "cp-0", true)
		createReadyNode(ctx, "cp-1", true)
		createReadyNode(ctx, "cp-2", true)
		s := driveToValidating("ct-multi")
		Expect(s.Status.ClusterType).To(Equal(popv1alpha1.ClusterTypeMultiNode))
		Expect(s.Status.TotalNodes).To(Equal(int32(3)))
	})
})
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `make test`
Expected: FAIL in "cluster type detection" specs — `ClusterType` is `""`, not `SingleNode`/`MultiNode`, because detection is not implemented yet.

- [ ] **Step 5: Implement detection in `reconcileValidating`**

In `internal/controller/clustershutdown_controller.go`, in `reconcileValidating`, immediately after these existing lines (~252-253):

```go
	shutdown.Status.NodeStatuses = nodeStatuses
	shutdown.Status.TotalNodes = int32(len(nodeStatuses))
```

add:

```go
	if len(nodeStatuses) == 1 {
		shutdown.Status.ClusterType = popv1alpha1.ClusterTypeSingleNode
	} else {
		shutdown.Status.ClusterType = popv1alpha1.ClusterTypeMultiNode
	}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `make test`
Expected: PASS, including both "cluster type detection" specs and all pre-existing controller specs.

- [ ] **Step 7: Commit**

```bash
git add api/v1alpha1/clustershutdown_types.go \
  internal/controller/clustershutdown_controller.go \
  internal/controller/clustershutdown_controller_test.go \
  config/crd/bases/pop.openshift.io_clustershutdowns.yaml \
  api/v1alpha1/zz_generated.deepcopy.go
git commit -m "feat(shutdown): detect single-node vs multi-node cluster type"
```

---

## Task 2: Skip pod eviction on single-node clusters

**Files:**
- Modify: `internal/controller/clustershutdown_controller.go` (`reconcileDraining`, starts ~line 350)
- Test: `internal/controller/clustershutdown_controller_test.go`

- [ ] **Step 1: Write the failing test**

Add this spec inside the existing `Describe("ClusterShutdown Controller", ...)` block:

```go
Describe("single-node drain skip", func() {
	wipeNodes := func() {
		list := &corev1.NodeList{}
		Expect(k8sClient.List(ctx, list)).To(Succeed())
		for i := range list.Items {
			_ = k8sClient.Delete(ctx, &list.Items[i])
		}
		Eventually(func() int {
			l := &corev1.NodeList{}
			_ = k8sClient.List(ctx, l)
			return len(l.Items)
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(0))
	}

	AfterEach(func() { wipeNodes() })

	It("transitions Draining to NotifyingApps without eviction on SNO", func() {
		wipeNodes()
		createReadyNode(ctx, "sno-drain-0", true)

		s := &popv1alpha1.ClusterShutdown{
			ObjectMeta: metav1.ObjectMeta{Name: "sno-drain", Namespace: ns},
			Spec: popv1alpha1.ClusterShutdownSpec{
				PVCName:            "etcd-backup",
				SkipCertCheck:      true,
				CordonNodes:        false,
				NotifyApplications: false,
			},
		}
		Expect(k8sClient.Create(ctx, s)).To(Succeed())

		// Drive the state machine up to and through Draining.
		var cur *popv1alpha1.ClusterShutdown
		Eventually(func() popv1alpha1.ClusterShutdownPhase {
			_, err := reconcileShutdown("sno-drain")
			Expect(err).NotTo(HaveOccurred())
			cur = getShutdown("sno-drain")
			return cur.Status.Phase
		}, 5*time.Second, 50*time.Millisecond).Should(Or(
			Equal(popv1alpha1.ClusterShutdownPhaseRunningHooks),
			Equal(popv1alpha1.ClusterShutdownPhaseNotifyingApps),
			Equal(popv1alpha1.ClusterShutdownPhaseBackingUp),
		))

		cur = getShutdown("sno-drain")
		Expect(cur.Status.ClusterType).To(Equal(popv1alpha1.ClusterTypeSingleNode))
		for _, nodeStatus := range cur.Status.NodeStatuses {
			Expect(nodeStatus.Status).To(Equal("Drained"))
			Expect(nodeStatus.Message).To(ContainSubstring("single-node"))
		}
	})
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `make test`
Expected: FAIL — on a single-node cluster the current `reconcileDraining` runs the eviction loop and does not set the "single-node" message; the node status message assertion fails.

- [ ] **Step 3: Implement the single-node branch in `reconcileDraining`**

In `internal/controller/clustershutdown_controller.go`, in `reconcileDraining`, immediately after the existing `DryRun` early-return block (the block that starts `if shutdown.Spec.DryRun {`) and before the line `drainTimeout := time.Duration(...)`, insert:

```go
	if shutdown.Status.ClusterType == popv1alpha1.ClusterTypeSingleNode {
		logger.Info("single-node cluster: skipping drain (no peer nodes to evict to)")
		for i := range shutdown.Status.NodeStatuses {
			shutdown.Status.NodeStatuses[i].Status = "Drained"
			shutdown.Status.NodeStatuses[i].Message = "single-node: drain skipped"
		}
		nextPhase := popv1alpha1.ClusterShutdownPhaseRunningHooks
		nextMessage := "Single-node cluster, skipping drain, running pre-shutdown hooks"
		if len(shutdown.Spec.PreShutdownHooks) == 0 {
			nextPhase = popv1alpha1.ClusterShutdownPhaseNotifyingApps
			nextMessage = "Single-node cluster, skipping drain, notifying applications"
		}
		shutdown.Status.Phase = nextPhase
		shutdown.Status.Message = nextMessage
		meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
			Type:    "NodesDrained",
			Status:  metav1.ConditionTrue,
			Reason:  "SingleNodeSkipDrain",
			Message: "Single-node cluster: drain skipped, no peer nodes",
		})
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
```

(`meta` and `metav1` are already imported in this file.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `make test`
Expected: PASS, including the new "single-node drain skip" spec and all pre-existing specs (multi-node draining behavior unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/controller/clustershutdown_controller.go \
  internal/controller/clustershutdown_controller_test.go
git commit -m "feat(shutdown): skip pod eviction on single-node clusters"
```

---

## Self-review

- **Spec coverage:** implements design §6 "cluster-type auto-detection driving shutdown behavior (full → drain/evict; SNO → skip drain-to-peer)" and FR2. Does not cover the SNO "quiesce + etcd backup + clean poweroff" wording beyond skipping drain — backup/poweroff already happen in the existing `BackingUp`/`ShuttingDown` phases, which SNO now reaches directly. No additional SNO backup logic needed here.
- **Placeholders:** none — all steps contain exact code and commands.
- **Type consistency:** `ClusterType`, `ClusterTypeSingleNode`, `ClusterTypeMultiNode`, and `Status.ClusterType` are defined in Task 1 and used identically in Task 2. `NodeShutdownState.Status`/`.Message`, `meta.SetStatusCondition`, and phase constants match the existing controller.

## Notes / follow-ups

- Detection uses node count (see "Decision embedded" above). A follow-up can switch to `Infrastructure.status.controlPlaneTopology` for authoritative detection if unusual topologies matter.
- This plan is Phase-1 subsystem 1 of the multicluster design. Subsystems 2 (recovery-manifest writer), 3 (`BootSequence`), and 4 (boot-agent, pending the cert-recovery spike) are separate plans.
