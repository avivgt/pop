package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

func createPVC(ctx context.Context, name, namespace string) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}
	err := k8sClient.Create(ctx, pvc)
	if err != nil && !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

var _ = Describe("EtcdBackup Controller", func() {
	var (
		reconciler *EtcdBackupReconciler
		ns         string
	)

	BeforeEach(func() {
		reconciler = &EtcdBackupReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		ns = "default"
	})

	reconcileBackup := func(name string) (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
	}

	getBackup := func(name string) *popv1alpha1.EtcdBackup {
		b := &popv1alpha1.EtcdBackup{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, b)).To(Succeed())
		return b
	}

	Context("Phase transitions", func() {
		It("should set phase to Pending on first reconcile", func() {
			name := "backup-pending-test"
			resource := &popv1alpha1.EtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.EtcdBackupSpec{PVCName: "backup-test-pvc"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			result, err := reconcileBackup(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			updated := getBackup(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.EtcdBackupPhasePending))
			Expect(updated.Status.StartTime).NotTo(BeNil())
		})

		It("should select a control plane node during Validating", func() {
			pvcName := "backup-validate-pvc"
			createPVC(ctx, pvcName, ns)
			defer func() {
				pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: ns}}
				_ = k8sClient.Delete(ctx, pvc)
			}()

			nodeName := "backup-validate-cp"
			createReadyNode(ctx, nodeName, true)
			defer deleteNode(ctx, nodeName)

			name := "backup-validate-test"
			resource := &popv1alpha1.EtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.EtcdBackupSpec{PVCName: pvcName},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			for i := 0; i < 5; i++ {
				reconcileBackup(name)
			}

			updated := getBackup(name)
			Expect(updated.Status.BackupNode).NotTo(BeEmpty())
		})

		It("should enter Running phase and attempt Job creation", func() {
			pvcName := "backup-running-pvc"
			createPVC(ctx, pvcName, ns)
			defer func() {
				pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: ns}}
				_ = k8sClient.Delete(ctx, pvc)
			}()

			nodeName := "backup-running-cp"
			createReadyNode(ctx, nodeName, true)
			defer deleteNode(ctx, nodeName)

			name := "backup-running-test"
			resource := &popv1alpha1.EtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.EtcdBackupSpec{PVCName: pvcName},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			for i := 0; i < 10; i++ {
				reconcileBackup(name)
				b := getBackup(name)
				if b.Status.Phase == popv1alpha1.EtcdBackupPhaseRunning {
					break
				}
			}

			updated := getBackup(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.EtcdBackupPhaseRunning))
		})

		It("should complete when Job succeeds", func() {
			name := "backup-complete-test"
			jobName := fmt.Sprintf("etcd-backup-%s", name)

			resource := &popv1alpha1.EtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.EtcdBackupSpec{PVCName: "test-pvc"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.EtcdBackupPhaseRunning
			resource.Status.BackupNode = "test-node"
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers:    []corev1.Container{{Name: "backup", Image: "test", Command: []string{"true"}}},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())
			job.Status.Succeeded = 1
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, job) }()

			reconcileBackup(name)

			updated := getBackup(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.EtcdBackupPhaseCompleted))
			Expect(updated.Status.CompletionTime).NotTo(BeNil())
		})
	})

	Context("Error paths", func() {
		It("should fail when PVC does not exist", func() {
			nodeName := "backup-nopvc-cp"
			createReadyNode(ctx, nodeName, true)
			defer deleteNode(ctx, nodeName)

			name := "backup-nopvc-test"
			resource := &popv1alpha1.EtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.EtcdBackupSpec{PVCName: "nonexistent-pvc"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			for i := 0; i < 5; i++ {
				reconcileBackup(name)
			}

			updated := getBackup(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.EtcdBackupPhaseFailed))
			Expect(updated.Status.Message).To(ContainSubstring("not found"))
		})

		It("should fail when no control plane nodes are ready", func() {
			pvcName := "backup-nocp-pvc"
			createPVC(ctx, pvcName, ns)
			defer func() {
				pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: ns}}
				_ = k8sClient.Delete(ctx, pvc)
			}()

			name := "backup-nocp-test"
			resource := &popv1alpha1.EtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.EtcdBackupSpec{PVCName: pvcName},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			for i := 0; i < 5; i++ {
				reconcileBackup(name)
			}

			updated := getBackup(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.EtcdBackupPhaseFailed))
		})

		It("should fail when Job fails", func() {
			name := "backup-jobfail-test"
			jobName := fmt.Sprintf("etcd-backup-%s", name)

			resource := &popv1alpha1.EtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       popv1alpha1.EtcdBackupSpec{PVCName: "test-pvc"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.EtcdBackupPhaseRunning
			resource.Status.BackupNode = "test-node"
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers:    []corev1.Container{{Name: "backup", Image: "test", Command: []string{"true"}}},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())
			job.Status.Failed = 1
			job.Status.Conditions = []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "backup script failed"},
			}
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, job) }()

			reconcileBackup(name)

			updated := getBackup(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.EtcdBackupPhaseFailed))
		})
	})
})
