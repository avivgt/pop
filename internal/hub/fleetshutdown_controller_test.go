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

package hub

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	workv1 "open-cluster-management.io/api/work/v1"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

var _ = Describe("FleetShutdown Controller", func() {
	const ns = "default"

	newReconciler := func() *FleetShutdownReconciler {
		return &FleetShutdownReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	reconcileFS := func(name string) (reconcile.Result, error) {
		return newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
	}

	ensureNamespace := func(name string) {
		n := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		err := k8sClient.Create(ctx, n)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	}

	// createPlacementDecision creates a PlacementDecision selecting the given clusters.
	createPlacementDecision := func(placement string, clusters ...string) {
		pd := &clusterv1beta1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      placement + "-1",
				Namespace: ns,
				Labels:    map[string]string{placementLabel: placement},
			},
		}
		Expect(k8sClient.Create(ctx, pd)).To(Succeed())
		for _, c := range clusters {
			pd.Status.Decisions = append(pd.Status.Decisions, clusterv1beta1.ClusterDecision{ClusterName: c})
		}
		Expect(k8sClient.Status().Update(ctx, pd)).To(Succeed())
	}

	setManifestWorkPhase := func(fsName, clusterName, phase string) {
		mw := &workv1.ManifestWork{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: manifestWorkName(fsName), Namespace: clusterName}, mw)).To(Succeed())
		p := phase
		mw.Status.ResourceStatus.Manifests = []workv1.ManifestCondition{{
			StatusFeedbacks: workv1.StatusFeedbackResult{
				Values: []workv1.FeedbackValue{
					{Name: "phase", Value: workv1.FieldValue{Type: workv1.String, String: &p}},
				},
			},
		}}
		Expect(k8sClient.Status().Update(ctx, mw)).To(Succeed())
	}

	It("fans out ManifestWorks and completes when all spokes report Complete", func() {
		ensureNamespace("site-a")
		ensureNamespace("site-b")
		createPlacementDecision("fs-pl", "site-a", "site-b")

		fs := &popv1alpha1.FleetShutdown{
			ObjectMeta: metav1.ObjectMeta{Name: "fs-1", Namespace: ns},
			Spec: popv1alpha1.FleetShutdownSpec{
				PlacementRef: "fs-pl",
				PVCName:      "etcd-backup",
				Reason:       "maintenance",
			},
		}
		Expect(k8sClient.Create(ctx, fs)).To(Succeed())

		// First reconcile: fan out.
		_, err := reconcileFS("fs-1")
		Expect(err).NotTo(HaveOccurred())

		for _, c := range []string{"site-a", "site-b"} {
			mw := &workv1.ManifestWork{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pop-shutdown-fs-1", Namespace: c}, mw)).To(Succeed(), "ManifestWork in %s", c)
			Expect(mw.Spec.Workload.Manifests).To(HaveLen(1))
		}

		got := &popv1alpha1.FleetShutdown{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "fs-1", Namespace: ns}, got)).To(Succeed())
		Expect(got.Status.SelectedClusters).To(Equal(int32(2)))
		Expect(got.Status.Phase).To(Equal(popv1alpha1.FleetShutdownPhaseInProgress))

		// Spokes report Complete via ManifestWork feedback.
		setManifestWorkPhase("fs-1", "site-a", "Complete")
		setManifestWorkPhase("fs-1", "site-b", "Complete")

		_, err = reconcileFS("fs-1")
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "fs-1", Namespace: ns}, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(popv1alpha1.FleetShutdownPhaseCompleted))
		Expect(got.Status.CompletedClusters).To(Equal(int32(2)))

		// ManifestWorks cleaned up (self-cleaning).
		for _, c := range []string{"site-a", "site-b"} {
			mw := &workv1.ManifestWork{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "pop-shutdown-fs-1", Namespace: c}, mw)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "ManifestWork in %s should be deleted", c)
		}
	})

	It("stays Pending when the placement selects no clusters", func() {
		// No PlacementDecision exists for this placement -> zero selected clusters.
		fs := &popv1alpha1.FleetShutdown{
			ObjectMeta: metav1.ObjectMeta{Name: "fs-empty", Namespace: ns},
			Spec:       popv1alpha1.FleetShutdownSpec{PlacementRef: "no-such-pl", PVCName: "etcd-backup"},
		}
		Expect(k8sClient.Create(ctx, fs)).To(Succeed())

		_, err := reconcileFS("fs-empty")
		Expect(err).NotTo(HaveOccurred())

		got := &popv1alpha1.FleetShutdown{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "fs-empty", Namespace: ns}, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(popv1alpha1.FleetShutdownPhasePending))
		Expect(got.Status.SelectedClusters).To(Equal(int32(0)))
	})
})
