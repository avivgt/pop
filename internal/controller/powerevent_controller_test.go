package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
	"github.com/avivgt/pop/internal/notification"
	ctrl "sigs.k8s.io/controller-runtime"
)

func createAPConfig(ctx context.Context, namespace string) {
	config := &popv1alpha1.AutomatedProceduresConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: namespace},
		Spec: popv1alpha1.AutomatedProceduresConfigSpec{
			Enabled:                  true,
			BatteryCapacityMinutes:   15,
			PowerConfirmationSeconds: 10,
			AutonomousShutdown:       false,
			BackupConfig:             popv1alpha1.BackupConfig{PVCName: "test-backup-pvc"},
		},
	}
	err := k8sClient.Create(ctx, config)
	if err != nil && !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

func deleteAPConfig(ctx context.Context, namespace string) {
	config := &popv1alpha1.AutomatedProceduresConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: namespace},
	}
	_ = k8sClient.Delete(ctx, config)
}

var _ = Describe("PowerEvent Controller", func() {
	var (
		reconciler *PowerEventReconciler
		ns         string
	)

	BeforeEach(func() {
		reconciler = &PowerEventReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		ns = "default"
	})

	reconcileEvent := func(name string) (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
	}

	getEvent := func(name string) *popv1alpha1.PowerEvent {
		e := &popv1alpha1.PowerEvent{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, e)).To(Succeed())
		return e
	}

	Context("Test events", func() {
		It("should complete a Test event in two reconciles", func() {
			name := "pe-test-complete"
			resource := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeTest,
					TriggeredBy: "manual",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			result, err := reconcileEvent(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			updated := getEvent(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseDetected))

			_, err = reconcileEvent(name)
			Expect(err).NotTo(HaveOccurred())

			updated = getEvent(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseComplete))
		})
	})

	Context("PowerRestored events", func() {
		It("should complete a PowerRestored event", func() {
			name := "pe-restored-complete"
			resource := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeRestored,
					TriggeredBy: "webhook",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileEvent(name)
			reconcileEvent(name)

			updated := getEvent(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseComplete))
		})

		It("should cancel an active PowerFailure event when power is restored", func() {
			createAPConfig(ctx, ns)
			defer deleteAPConfig(ctx, ns)

			failureName := "pe-failure-to-cancel"
			failure := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: failureName, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeFailure,
					TriggeredBy: "webhook",
				},
			}
			Expect(k8sClient.Create(ctx, failure)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, failure) }()

			reconcileEvent(failureName)
			reconcileEvent(failureName)

			updated := getEvent(failureName)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseBackingUp))

			restoredName := "pe-restored-cancels"
			restored := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: restoredName, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeRestored,
					TriggeredBy: "webhook",
				},
			}
			Expect(k8sClient.Create(ctx, restored)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, restored) }()

			reconcileEvent(restoredName)
			reconcileEvent(restoredName)

			cancelledFailure := getEvent(failureName)
			Expect(cancelledFailure.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseCancelled))
		})
	})

	Context("PowerFailure events", func() {
		It("should fail when AutomatedProceduresConfig does not exist", func() {
			name := "pe-failure-noconfig"
			resource := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeFailure,
					TriggeredBy: "webhook",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileEvent(name)
			reconcileEvent(name)

			updated := getEvent(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseFailed))
		})

		It("should fail when config is disabled", func() {
			config := &popv1alpha1.AutomatedProceduresConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: ns},
				Spec: popv1alpha1.AutomatedProceduresConfigSpec{
					Enabled:      false,
					BackupConfig: popv1alpha1.BackupConfig{PVCName: "test-pvc"},
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, config) }()

			name := "pe-failure-disabled"
			resource := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeFailure,
					TriggeredBy: "webhook",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileEvent(name)
			reconcileEvent(name)

			updated := getEvent(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseFailed))
		})

		It("should transition to BackingUp and create EtcdBackup", func() {
			createAPConfig(ctx, ns)
			defer deleteAPConfig(ctx, ns)

			name := "pe-failure-backup"
			resource := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeFailure,
					TriggeredBy: "webhook",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileEvent(name)
			reconcileEvent(name)

			updated := getEvent(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseBackingUp))

			backupName := "power-backup-" + name
			backup := &popv1alpha1.EtcdBackup{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: backupName, Namespace: ns}, backup)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = k8sClient.Delete(ctx, backup) }()
		})

		It("should complete with manual intervention message when AutonomousShutdown is false", func() {
			createAPConfig(ctx, ns)
			defer deleteAPConfig(ctx, ns)

			name := "pe-failure-manual"
			resource := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeFailure,
					TriggeredBy: "webhook",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconcileEvent(name)
			reconcileEvent(name)

			backupName := "power-backup-" + name
			backup := &popv1alpha1.EtcdBackup{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: backupName, Namespace: ns}, backup); err != nil {
				backup = &popv1alpha1.EtcdBackup{
					ObjectMeta: metav1.ObjectMeta{Name: backupName, Namespace: ns},
					Spec:       popv1alpha1.EtcdBackupSpec{PVCName: "test-backup-pvc"},
				}
				Expect(k8sClient.Create(ctx, backup)).To(Succeed())
			}
			backup.Status.Phase = popv1alpha1.EtcdBackupPhaseCompleted
			Expect(k8sClient.Status().Update(ctx, backup)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, backup) }()

			updated := getEvent(name)
			past := metav1.NewTime(metav1.Now().Add(-60 * 1e9))
			updated.Status.ConfirmationDeadline = &past
			Expect(k8sClient.Status().Update(ctx, updated)).To(Succeed())

			for i := 0; i < 10; i++ {
				reconcileEvent(name)
				e := getEvent(name)
				if e.Status.Phase == popv1alpha1.PowerEventPhaseComplete {
					break
				}
			}

			final := getEvent(name)
			Expect(final.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseComplete))
			Expect(final.Status.Message).To(ContainSubstring("Manual"))
		})
	})

	Context("Notification integration", func() {
		It("should populate NotificationsSent when notification service is configured", func() {
			config := &popv1alpha1.AutomatedProceduresConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: ns},
				Spec: popv1alpha1.AutomatedProceduresConfigSpec{
					Enabled:                  true,
					BatteryCapacityMinutes:   15,
					PowerConfirmationSeconds: 10,
					BackupConfig:             popv1alpha1.BackupConfig{PVCName: "test-pvc"},
					Notification: &popv1alpha1.NotificationConfig{
						KubernetesEvents: &popv1alpha1.KubernetesEventsConfig{
							Enabled: true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, config) }()

			notifReconciler := &PowerEventReconciler{
				Client:              k8sClient,
				Scheme:              k8sClient.Scheme(),
				NotificationService: notification.NewService(k8sClient, ctrl.Log.WithName("test-notification")),
			}

			name := "pe-notif-test"
			resource := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeTest,
					TriggeredBy: "manual",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			_, err := notifReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := getEvent(name)
			Expect(updated.Status.Phase).To(Equal(popv1alpha1.PowerEventPhaseDetected))
			Expect(updated.Status.NotificationsSent).NotTo(BeEmpty())

			found := false
			for _, rec := range updated.Status.NotificationsSent {
				if rec.Channel == "kubernetes-event" {
					found = true
					Expect(rec.Success).To(BeTrue())
				}
			}
			Expect(found).To(BeTrue(), "expected kubernetes-event notification record")

			// Verify the actual K8s Event was created.
			eventList := &corev1.EventList{}
			Expect(k8sClient.List(ctx, eventList)).To(Succeed())
			foundEvent := false
			for _, e := range eventList.Items {
				if e.InvolvedObject.Name == name && e.InvolvedObject.Kind == "PowerEvent" {
					foundEvent = true
					Expect(e.Reason).To(Equal("PowerTest"))
				}
			}
			Expect(foundEvent).To(BeTrue(), "expected K8s Event referencing the PowerEvent")
		})

		It("should not fail when notification service is nil", func() {
			name := "pe-no-notif"
			resource := &popv1alpha1.PowerEvent{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: popv1alpha1.PowerEventSpec{
					EventType:   popv1alpha1.PowerEventTypeTest,
					TriggeredBy: "manual",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			// reconciler has nil NotificationService (default from BeforeEach)
			_, err := reconcileEvent(name)
			Expect(err).NotTo(HaveOccurred())

			updated := getEvent(name)
			Expect(updated.Status.NotificationsSent).To(BeEmpty())
		})
	})
})
