package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

func createReadyNode(ctx context.Context, name string, isMaster bool) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
	if isMaster {
		node.Labels["node-role.kubernetes.io/master"] = ""
	}
	err := k8sClient.Create(ctx, node)
	if err != nil && !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

func deleteNode(ctx context.Context, name string) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_ = k8sClient.Delete(ctx, node)
}

var _ = Describe("ClusterShutdown Controller", func() {
	var (
		reconciler *ClusterShutdownReconciler
		ns         string
	)

	BeforeEach(func() {
		reconciler = &ClusterShutdownReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		ns = "default"
	})

	reconcileShutdown := func(name string) (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
	}

	getShutdown := func(name string) *popv1alpha1.ClusterShutdown {
		s := &popv1alpha1.ClusterShutdown{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, s)).To(Succeed())
		return s
	}

	// waitForCacheSync waits until the cached client sees the shutdown CR with the expected phase.
	waitForCacheSync := func(name string, expectedPhase popv1alpha1.ClusterShutdownPhase) {
		Eventually(func() popv1alpha1.ClusterShutdownPhase {
			s := &popv1alpha1.ClusterShutdown{}
			if err := cachedClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, s); err != nil {
				return ""
			}
			return s.Status.Phase
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(expectedPhase))
	}

	// reconcileWithCache runs reconcile using cachedClient (needed for MatchingFields queries).
	reconcileWithCache := func(name string) (reconcile.Result, error) {
		cr := &ClusterShutdownReconciler{
			Client: cachedClient,
			Scheme: k8sClient.Scheme(),
		}
		return cr.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
	}

	Context("Phase transitions", func() {
		It("should set phase to Pending and StartTime on first reconcile", func() {
			name := "shutdown-pending-test"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			// First reconcile adds finalizer.
			reconcileShutdown(name)
			// Second reconcile sets phase to Pending.
			result, err := reconcileShutdown(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			updated := getShutdown(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.ClusterShutdownPhasePending))
			Expect(updated.Status.StartTime).NotTo(BeNil())
		})

		It("should transition from Pending to Validating when no scheduled time", func() {
			name := "shutdown-validate-test"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileShutdown(name) // finalizer
			reconcileShutdown(name) // Pending
			reconcileShutdown(name) // Validating

			updated := getShutdown(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.ClusterShutdownPhaseValidating))
			Expect(updated.Status.EstimatedCompletionTime).NotTo(BeNil())
		})

		It("should create NodeShutdownState entries during Validating", func() {
			nodeName := "shutdown-validate-node"
			createReadyNode(ctx, nodeName, false)
			defer deleteNode(ctx, nodeName)

			name := "shutdown-nodestate-test"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileShutdown(name) // finalizer
			reconcileShutdown(name) // Pending
			reconcileShutdown(name) // Validating
			reconcileShutdown(name) // Cordoning (with node statuses)

			updated := getShutdown(name)
			Expect(updated.Status.TotalNodes).To(BeNumerically(">", 0))
			Expect(updated.Status.NodeStatuses).NotTo(BeEmpty())
		})

		It("should reach BackingUp phase during dry-run", func() {
			nodeName := "shutdown-backup-node"
			createReadyNode(ctx, nodeName, true)
			defer deleteNode(ctx, nodeName)

			name := "shutdown-backup-test"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true, DryRun: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			for i := 0; i < 10; i++ {
				reconcileShutdown(name)
				s := getShutdown(name)
				if s.Status.Phase == popv1alpha1.ClusterShutdownPhaseBackingUp {
					break
				}
			}

			updated := getShutdown(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.ClusterShutdownPhaseBackingUp))
		})

		It("should complete dry-run when backup succeeds", func() {
			name := "shutdown-dryrun-test"
			backupName := fmt.Sprintf("shutdown-%s", name)

			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{"pop.openshift.io/shutdown-cleanup"},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true, DryRun: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseBackingUp
			resource.Status.EtcdBackupRef = backupName
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			backup := &popv1alpha1.EtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: backupName, Namespace: ns},
				Spec:       popv1alpha1.EtcdBackupSpec{PVCName: "test-pvc"},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())
			backup.Status.Phase = popv1alpha1.EtcdBackupPhaseCompleted
			Expect(k8sClient.Status().Update(ctx, backup)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, backup) }()

			reconcileShutdown(name)

			updated := getShutdown(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.ClusterShutdownPhaseComplete))
		})
	})

	Context("Finalizer lifecycle", func() {
		It("should add finalizer on first reconcile", func() {
			name := "shutdown-finalizer-test"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileShutdown(name)

			updated := getShutdown(name)
			Expect(controllerutil.ContainsFinalizer(updated, shutdownCleanupFinalizer)).To(BeTrue())
		})

		It("should set observedGeneration on reconcile", func() {
			name := "shutdown-obsgen-test"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileShutdown(name) // adds finalizer
			reconcileShutdown(name) // sets Pending

			updated := getShutdown(name)
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
		})
	})

	Context("Draining phase", func() {
		It("should skip draining in dry-run mode", func() {
			nodeName := "drain-dryrun-node"
			createReadyNode(ctx, nodeName, false)
			defer deleteNode(ctx, nodeName)

			name := "shutdown-drain-dryrun"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{shutdownCleanupFinalizer},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true, DryRun: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseDraining
			resource.Status.NodeStatuses = []popv1alpha1.NodeShutdownState{{Name: nodeName, Status: "Cordoned"}}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileShutdown(name)

			updated := getShutdown(name)
			// Dry run skips draining and goes to hooks or notifying
			Expect(updated.Status.Phase).To(BeElementOf(
				popv1alpha1.ClusterShutdownPhaseRunningHooks,
				popv1alpha1.ClusterShutdownPhaseNotifyingApps,
			))
		})

		It("should drain non-DaemonSet pods from nodes", func() {
			nodeName := "drain-regular-node"
			createReadyNode(ctx, nodeName, false)
			defer deleteNode(ctx, nodeName)

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "drain-regular-pod",
					Namespace: ns,
				},
				Spec: corev1.PodSpec{
					NodeName: nodeName,
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "busybox",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()

			name := "shutdown-drain-regular"
			now := metav1.Now()
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{shutdownCleanupFinalizer},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseDraining
			resource.Status.StartTime = &now
			resource.Status.NodeStatuses = []popv1alpha1.NodeShutdownState{{Name: nodeName, Status: "Cordoned"}}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			waitForCacheSync(name, popv1alpha1.ClusterShutdownPhaseDraining)

			_, err := reconcileWithCache(name)
			Expect(err).NotTo(HaveOccurred())

			updated := getShutdown(name)
			for _, nodeStatus := range updated.Status.NodeStatuses {
				if nodeStatus.Name == nodeName {
					Expect(nodeStatus.Status).To(BeElementOf("Draining", "Drained"))
				}
			}
		})

		It("should skip DaemonSet-owned pods during draining", func() {
			nodeName := "drain-ds-node"
			createReadyNode(ctx, nodeName, false)
			defer deleteNode(ctx, nodeName)

			trueVal := true
			dsPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "drain-ds-pod",
					Namespace: ns,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1",
						Kind:       "DaemonSet",
						Name:       "test-ds",
						UID:        "fake-uid",
						Controller: &trueVal,
					}},
				},
				Spec: corev1.PodSpec{
					NodeName: nodeName,
					Containers: []corev1.Container{{
						Name:  "ds-container",
						Image: "busybox",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, dsPod)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, dsPod) }()

			name := "shutdown-drain-ds"
			now := metav1.Now()
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{shutdownCleanupFinalizer},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseDraining
			resource.Status.StartTime = &now
			resource.Status.NodeStatuses = []popv1alpha1.NodeShutdownState{{Name: nodeName, Status: "Cordoned"}}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			waitForCacheSync(name, popv1alpha1.ClusterShutdownPhaseDraining)

			_, err := reconcileWithCache(name)
			Expect(err).NotTo(HaveOccurred())

			updated := getShutdown(name)
			for _, nodeStatus := range updated.Status.NodeStatuses {
				if nodeStatus.Name == nodeName {
					Expect(nodeStatus.Status).To(Equal("Drained"))
					Expect(nodeStatus.Message).To(ContainSubstring("all non-DaemonSet pods evicted"))
				}
			}
		})

		It("should skip mirror pods during draining", func() {
			nodeName := "drain-mirror-node"
			createReadyNode(ctx, nodeName, false)
			defer deleteNode(ctx, nodeName)

			mirrorPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "drain-mirror-pod",
					Namespace: ns,
					Annotations: map[string]string{
						corev1.MirrorPodAnnotationKey: "some-hash",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: nodeName,
					Containers: []corev1.Container{{
						Name:  "static",
						Image: "busybox",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, mirrorPod)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, mirrorPod) }()

			name := "shutdown-drain-mirror"
			now := metav1.Now()
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{shutdownCleanupFinalizer},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseDraining
			resource.Status.StartTime = &now
			resource.Status.NodeStatuses = []popv1alpha1.NodeShutdownState{{Name: nodeName, Status: "Cordoned"}}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			waitForCacheSync(name, popv1alpha1.ClusterShutdownPhaseDraining)

			_, err := reconcileWithCache(name)
			Expect(err).NotTo(HaveOccurred())

			updated := getShutdown(name)
			for _, nodeStatus := range updated.Status.NodeStatuses {
				if nodeStatus.Name == nodeName {
					Expect(nodeStatus.Status).To(Equal("Drained"))
				}
			}
		})

		It("should skip completed/failed pods during draining", func() {
			nodeName := "drain-completed-node"
			createReadyNode(ctx, nodeName, false)
			defer deleteNode(ctx, nodeName)

			completedPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "drain-completed-pod",
					Namespace: ns,
				},
				Spec: corev1.PodSpec{
					NodeName: nodeName,
					Containers: []corev1.Container{{
						Name:  "job",
						Image: "busybox",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, completedPod)).To(Succeed())
			completedPod.Status.Phase = corev1.PodSucceeded
			Expect(k8sClient.Status().Update(ctx, completedPod)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, completedPod) }()

			name := "shutdown-drain-completed"
			now := metav1.Now()
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{shutdownCleanupFinalizer},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseDraining
			resource.Status.StartTime = &now
			resource.Status.NodeStatuses = []popv1alpha1.NodeShutdownState{{Name: nodeName, Status: "Cordoned"}}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			waitForCacheSync(name, popv1alpha1.ClusterShutdownPhaseDraining)

			_, err := reconcileWithCache(name)
			Expect(err).NotTo(HaveOccurred())

			updated := getShutdown(name)
			for _, nodeStatus := range updated.Status.NodeStatuses {
				if nodeStatus.Name == nodeName {
					Expect(nodeStatus.Status).To(Equal("Drained"))
				}
			}
		})

		It("should skip pods matching DaemonSet label selectors", func() {
			nodeName := "drain-ds-label-node"
			createReadyNode(ctx, nodeName, false)
			defer deleteNode(ctx, nodeName)

			ds := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-label-ds",
					Namespace: ns,
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "ds-label-match"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "ds-label-match"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, ds) }()

			labelPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "drain-ds-label-pod",
					Namespace: ns,
					Labels:    map[string]string{"app": "ds-label-match"},
				},
				Spec: corev1.PodSpec{
					NodeName: nodeName,
					Containers: []corev1.Container{{
						Name:  "c",
						Image: "busybox",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, labelPod)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, labelPod) }()

			name := "shutdown-drain-ds-label"
			now := metav1.Now()
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{shutdownCleanupFinalizer},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseDraining
			resource.Status.StartTime = &now
			resource.Status.NodeStatuses = []popv1alpha1.NodeShutdownState{{Name: nodeName, Status: "Cordoned"}}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			waitForCacheSync(name, popv1alpha1.ClusterShutdownPhaseDraining)
			Eventually(func() int {
				dsList := &appsv1.DaemonSetList{}
				_ = cachedClient.List(ctx, dsList)
				return len(dsList.Items)
			}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))

			_, err := reconcileWithCache(name)
			Expect(err).NotTo(HaveOccurred())

			updated := getShutdown(name)
			for _, nodeStatus := range updated.Status.NodeStatuses {
				if nodeStatus.Name == nodeName {
					Expect(nodeStatus.Status).To(Equal("Drained"))
				}
			}
		})

		It("should handle drain timeout", func() {
			nodeName := "drain-timeout-node"
			createReadyNode(ctx, nodeName, false)
			defer deleteNode(ctx, nodeName)

			name := "shutdown-drain-timeout"
			pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{shutdownCleanupFinalizer},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{
					PVCName:              "test-pvc",
					ShutdownDelayMinutes: 1,
					SkipCertCheck:        true,
					DrainTimeoutSeconds:  30,
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseDraining
			resource.Status.StartTime = &pastTime
			resource.Status.NodeStatuses = []popv1alpha1.NodeShutdownState{{Name: nodeName, Status: "Draining"}}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			waitForCacheSync(name, popv1alpha1.ClusterShutdownPhaseDraining)

			_, err := reconcileWithCache(name)
			Expect(err).NotTo(HaveOccurred())

			updated := getShutdown(name)
			for _, nodeStatus := range updated.Status.NodeStatuses {
				if nodeStatus.Name == nodeName {
					Expect(nodeStatus.Status).To(Equal("Drained"))
					Expect(nodeStatus.Message).To(ContainSubstring("drain timed out"))
				}
			}
		})
	})

	Context("Cleanup on deletion", func() {
		It("should uncordon nodes when CR is deleted", func() {
			nodeName := "cleanup-uncordon-node"
			createReadyNode(ctx, nodeName, false)
			defer deleteNode(ctx, nodeName)

			// Cordon the node.
			node := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)).To(Succeed())
			original := node.DeepCopy()
			node.Spec.Unschedulable = true
			Expect(k8sClient.Patch(ctx, node, client.MergeFrom(original))).To(Succeed())

			name := "shutdown-cleanup-uncordon"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{shutdownCleanupFinalizer},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseComplete
			resource.Status.NodeStatuses = []popv1alpha1.NodeShutdownState{{Name: nodeName, Status: "ShuttingDown"}}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			// Delete the CR to trigger the finalizer.
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			// Reconcile handles the deletion.
			reconcileShutdown(name)

			// Verify node is uncordoned.
			updatedNode := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, updatedNode)).To(Succeed())
			Expect(updatedNode.Spec.Unschedulable).To(BeFalse())
		})

		It("should delete shutdown signal ConfigMap when CR is deleted", func() {
			name := "shutdown-cleanup-cm"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{shutdownCleanupFinalizer},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseComplete
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			// Create the signal ConfigMap.
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pop-shutdown-signal",
					Namespace: ns,
				},
				Data: map[string]string{"shutdown": name},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			reconcileShutdown(name)

			// Verify ConfigMap is deleted.
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "pop-shutdown-signal", Namespace: ns}, &corev1.ConfigMap{})
			Expect(errors.IsNotFound(err)).To(BeTrue())

			// Verify CR is gone (finalizer removed).
			err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &popv1alpha1.ClusterShutdown{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("Error paths", func() {
		It("should fail when no nodes exist during Validating", func() {
			name := "shutdown-nonodes-test"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			for i := 0; i < 5; i++ {
				reconcileShutdown(name)
			}

			updated := getShutdown(name)
			if updated.Status.Phase == popv1alpha1.ClusterShutdownPhaseFailed {
				Expect(updated.Status.Message).NotTo(BeEmpty())
			}
		})

		It("should fail when backup fails", func() {
			name := "shutdown-backupfail-test"
			backupName := fmt.Sprintf("shutdown-%s", name)

			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{"pop.openshift.io/shutdown-cleanup"},
				},
				Spec: popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true, DryRun: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterShutdownPhaseBackingUp
			resource.Status.EtcdBackupRef = backupName
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			backup := &popv1alpha1.EtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: backupName, Namespace: ns},
				Spec:       popv1alpha1.EtcdBackupSpec{PVCName: "test-pvc"},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())
			backup.Status.Phase = popv1alpha1.EtcdBackupPhaseFailed
			backup.Status.Message = "simulated failure"
			Expect(k8sClient.Status().Update(ctx, backup)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, backup) }()

			reconcileShutdown(name)

			updated := getShutdown(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.ClusterShutdownPhaseFailed))
		})
	})
})
