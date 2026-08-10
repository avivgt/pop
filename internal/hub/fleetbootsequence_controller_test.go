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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

var _ = Describe("FleetBootSequence Controller", func() {
	const ns = "default"

	reconcileFBS := func(name string) (reconcile.Result, error) {
		r := &FleetBootSequenceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		return r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}})
	}

	makeManagedCluster := func(name string, available bool) {
		mc := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: name}}
		err := k8sClient.Create(ctx, mc)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, mc)).To(Succeed())
		status := metav1.ConditionFalse
		if available {
			status = metav1.ConditionTrue
		}
		mc.Status.Conditions = []metav1.Condition{{
			Type: clusterv1.ManagedClusterConditionAvailable, Status: status,
			Reason: "Test", LastTransitionTime: metav1.Now(),
		}}
		Expect(k8sClient.Status().Update(ctx, mc)).To(Succeed())
	}

	makePlacementDecision := func(placement string, clusters ...string) {
		pd := &clusterv1beta1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name: placement + "-1", Namespace: ns,
				Labels: map[string]string{placementLabel: placement},
			},
		}
		err := k8sClient.Create(ctx, pd)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: placement + "-1", Namespace: ns}, pd)).To(Succeed())
		pd.Status.Decisions = nil
		for _, c := range clusters {
			pd.Status.Decisions = append(pd.Status.Decisions, clusterv1beta1.ClusterDecision{ClusterName: c})
		}
		Expect(k8sClient.Status().Update(ctx, pd)).To(Succeed())
	}

	It("marks tiers ready in dependency order and completes", func() {
		makeManagedCluster("bs-hub", true)
		makeManagedCluster("bs-edge", false) // not available yet
		makePlacementDecision("bs-shared-pl", "bs-hub")
		makePlacementDecision("bs-edge-pl", "bs-edge")

		fbs := &popv1alpha1.FleetBootSequence{
			ObjectMeta: metav1.ObjectMeta{Name: "bs-1", Namespace: ns},
			Spec: popv1alpha1.FleetBootSequenceSpec{
				Tiers: []popv1alpha1.FleetBootTier{
					{Name: "shared", PlacementRef: "bs-shared-pl", TimeoutSeconds: 3600},
					{Name: "edge", PlacementRef: "bs-edge-pl", DependsOn: []string{"shared"}, TimeoutSeconds: 3600},
				},
			},
		}
		Expect(k8sClient.Create(ctx, fbs)).To(Succeed())

		// First pass: shared ready, edge waiting on its (unavailable) cluster.
		_, err := reconcileFBS("bs-1")
		Expect(err).NotTo(HaveOccurred())
		_, err = reconcileFBS("bs-1")
		Expect(err).NotTo(HaveOccurred())

		got := &popv1alpha1.FleetBootSequence{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "bs-1", Namespace: ns}, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(popv1alpha1.FleetBootSequencePhaseInProgress))
		tierReady := map[string]bool{}
		for _, t := range got.Status.Tiers {
			tierReady[t.Name] = t.Ready
		}
		Expect(tierReady["shared"]).To(BeTrue())
		Expect(tierReady["edge"]).To(BeFalse())

		// Edge cluster becomes available -> sequence completes.
		makeManagedCluster("bs-edge", true)
		_, err = reconcileFBS("bs-1")
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "bs-1", Namespace: ns}, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(popv1alpha1.FleetBootSequencePhaseComplete))
	})

	It("rejects a dependency cycle", func() {
		fbs := &popv1alpha1.FleetBootSequence{
			ObjectMeta: metav1.ObjectMeta{Name: "bs-cycle", Namespace: ns},
			Spec: popv1alpha1.FleetBootSequenceSpec{
				Tiers: []popv1alpha1.FleetBootTier{
					{Name: "a", PlacementRef: "pl-a", DependsOn: []string{"b"}, TimeoutSeconds: 3600},
					{Name: "b", PlacementRef: "pl-b", DependsOn: []string{"a"}, TimeoutSeconds: 3600},
				},
			},
		}
		Expect(k8sClient.Create(ctx, fbs)).To(Succeed())

		_, err := reconcileFBS("bs-cycle")
		Expect(err).NotTo(HaveOccurred())

		got := &popv1alpha1.FleetBootSequence{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "bs-cycle", Namespace: ns}, got)).To(Succeed())
		Expect(got.Status.Message).To(ContainSubstring("cycle"))
	})
})
