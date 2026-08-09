/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

var _ = Describe("ClusterRestart ready signal", func() {
	ctx := context.Background()

	It("emits a cluster-ready signal ConfigMap owned by the ClusterRestart", func() {
		restart := &popv1alpha1.ClusterRestart{
			ObjectMeta: metav1.ObjectMeta{Name: "ready-signal", Namespace: "default"},
			Spec:       popv1alpha1.ClusterRestartSpec{},
		}
		Expect(k8sClient.Create(ctx, restart)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, restart) }()

		now := metav1.Now()
		restart.Status.CompletionTime = &now

		r := &ClusterRestartReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		Expect(r.ensureClusterReadySignal(ctx, restart)).To(Succeed())

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pop-cluster-ready", Namespace: "default"}, cm)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, cm) }()

		Expect(cm.Data["status"]).To(Equal("ready"))
		Expect(cm.Data["restartName"]).To(Equal("ready-signal"))
		Expect(cm.Data["completionTime"]).NotTo(BeEmpty())
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(cm.OwnerReferences[0].Name).To(Equal("ready-signal"))
		Expect(cm.OwnerReferences[0].Kind).To(Equal("ClusterRestart"))

		// Idempotent: a second call updates in place without error.
		Expect(r.ensureClusterReadySignal(ctx, restart)).To(Succeed())
		updated := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pop-cluster-ready", Namespace: "default"}, updated)).To(Succeed())
		Expect(updated.Data["status"]).To(Equal("ready"))
	})
})

var _ = Describe("ClusterRestart Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		clusterrestart := &popv1alpha1.ClusterRestart{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterRestart")
			err := k8sClient.Get(ctx, typeNamespacedName, clusterrestart)
			if err != nil && errors.IsNotFound(err) {
				resource := &popv1alpha1.ClusterRestart{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &popv1alpha1.ClusterRestart{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ClusterRestart")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ClusterRestartReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("Finalizer and observedGeneration", func() {
		It("should add finalizer on first reconcile", func() {
			name := "restart-finalizer-test"
			resource := &popv1alpha1.ClusterRestart{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconciler := &ClusterRestartReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &popv1alpha1.ClusterRestart{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, updated)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updated, "pop.openshift.io/restart-cleanup")).To(BeTrue())
		})

		It("should set observedGeneration on reconcile", func() {
			name := "restart-obsgen-test"
			resource := &popv1alpha1.ClusterRestart{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			reconciler := &ClusterRestartReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// First reconcile adds finalizer.
			reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: "default"},
			})
			// Second reconcile sets phase.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &popv1alpha1.ClusterRestart{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, updated)).To(Succeed())
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
		})

		It("should persist degradedCheckCount in status not reconciler struct", func() {
			name := "restart-degraded-count-test"
			resource := &popv1alpha1.ClusterRestart{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  "default",
					Finalizers: []string{"pop.openshift.io/restart-cleanup"},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			resource.Status.Phase = popv1alpha1.ClusterRestartPhaseVerifyingOperators
			resource.Status.DegradedCheckCount = 2
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, resource) }()

			// Verify the count persists in the CR status.
			updated := &popv1alpha1.ClusterRestart{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, updated)).To(Succeed())
			Expect(updated.Status.DegradedCheckCount).To(Equal(int32(2)))
		})
	})
})
