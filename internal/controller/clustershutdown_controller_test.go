package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

	Context("Phase transitions", func() {
		It("should set phase to Pending and StartTime on first reconcile", func() {
			name := "shutdown-pending-test"
			resource := &popv1alpha1.ClusterShutdown{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

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

			reconcileShutdown(name)
			reconcileShutdown(name)

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

			reconcileShutdown(name)
			reconcileShutdown(name)
			reconcileShutdown(name)

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
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true, DryRun: true},
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
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.ClusterShutdownSpec{PVCName: "test-pvc", ShutdownDelayMinutes: 1, SkipCertCheck: true, DryRun: true},
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
