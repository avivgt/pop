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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

func createNamespace(name string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	err := k8sClient.Create(ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

func deleteNamespace(name string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_ = k8sClient.Delete(ctx, ns)
}

func createReadyPod(name, namespace string, ready bool) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "pause", Image: "pause"}},
		},
	}
	Expect(k8sClient.Create(ctx, pod)).To(Succeed())
	pod.Status.Phase = corev1.PodRunning
	cond := corev1.ConditionFalse
	if ready {
		cond = corev1.ConditionTrue
	}
	pod.Status.Conditions = []corev1.PodCondition{{
		Type: corev1.PodReady, Status: cond,
	}}
	Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
}

func deletePod(name, namespace string) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	_ = k8sClient.Delete(ctx, pod)
}

func stageByName(stages []popv1alpha1.BootStageStatus, name string) popv1alpha1.BootStageStatus {
	for _, s := range stages {
		if s.Name == name {
			return s
		}
	}
	Fail(fmt.Sprintf("stage %q not found", name))
	return popv1alpha1.BootStageStatus{}
}

var _ = Describe("topoSortStages", func() {
	It("orders a diamond DAG and rejects cycles, duplicates, and unknown refs", func() {
		order, err := topoSortStages([]popv1alpha1.BootStage{
			{Name: "apps", DependsOn: []string{"storage", "net"}},
			{Name: "storage"},
			{Name: "net", DependsOn: []string{"storage"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(order[0]).To(Equal("storage"))
		Expect(order).To(ConsistOf("storage", "net", "apps"))
		Expect(indexOf(order, "net")).To(BeNumerically("<", indexOf(order, "apps")))

		_, err = topoSortStages([]popv1alpha1.BootStage{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b", DependsOn: []string{"a"}},
		})
		Expect(err).To(MatchError(ContainSubstring("cycle")))

		_, err = topoSortStages([]popv1alpha1.BootStage{
			{Name: "a"},
			{Name: "a"},
		})
		Expect(err).To(MatchError(ContainSubstring("duplicate")))

		_, err = topoSortStages([]popv1alpha1.BootStage{
			{Name: "a", DependsOn: []string{"missing"}},
		})
		Expect(err).To(MatchError(ContainSubstring("unknown")))
	})
})

func indexOf(ss []string, v string) int {
	for i, s := range ss {
		if s == v {
			return i
		}
	}
	return -1
}

var _ = Describe("BootSequence Controller", func() {
	var reconciler *BootSequenceReconciler

	BeforeEach(func() {
		reconciler = &BootSequenceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	})

	const bsNS = "default"

	reconcileBS := func(name string) (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: bsNS},
		})
	}

	getBS := func(name string) *popv1alpha1.BootSequence {
		bs := &popv1alpha1.BootSequence{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: bsNS}, bs)).To(Succeed())
		return bs
	}

	It("does not complete when the selected namespace is empty", func() {
		ns := "bs-empty-ns"
		createNamespace(ns)
		defer deleteNamespace(ns)

		bs := &popv1alpha1.BootSequence{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-ns", Namespace: "default"},
			Spec: popv1alpha1.BootSequenceSpec{
				Stages: []popv1alpha1.BootStage{{
					Name:           "storage",
					Selector:       popv1alpha1.BootStageSelector{Namespaces: []string{ns}},
					TimeoutSeconds: 3600,
				}},
			},
		}
		Expect(k8sClient.Create(ctx, bs)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, bs) }()

		_, err := reconcileBS("empty-ns")
		Expect(err).NotTo(HaveOccurred())

		got := getBS("empty-ns")
		Expect(got.Status.Phase).To(Equal(popv1alpha1.BootSequencePhaseInProgress))
		Expect(stageByName(got.Status.Stages, "storage").Ready).To(BeFalse())
		Expect(stageByName(got.Status.Stages, "storage").PodsTotal).To(Equal(int32(0)))
	})

	It("does not complete when the selected namespace does not exist", func() {
		bs := &popv1alpha1.BootSequence{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-ns", Namespace: "default"},
			Spec: popv1alpha1.BootSequenceSpec{
				Stages: []popv1alpha1.BootStage{{
					Name:           "storage",
					Selector:       popv1alpha1.BootStageSelector{Namespaces: []string{"does-not-exist-bs"}},
					TimeoutSeconds: 3600,
				}},
			},
		}
		Expect(k8sClient.Create(ctx, bs)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, bs) }()

		_, err := reconcileBS("missing-ns")
		Expect(err).NotTo(HaveOccurred())

		got := getBS("missing-ns")
		Expect(got.Status.Phase).To(Equal(popv1alpha1.BootSequencePhaseInProgress))
		Expect(stageByName(got.Status.Stages, "storage").Ready).To(BeFalse())
		Expect(stageByName(got.Status.Stages, "storage").Message).To(ContainSubstring("does-not-exist-bs"))
	})

	It("blocks dependents until pods are Ready, then completes", func() {
		storageNS, appsNS := "bs-storage", "bs-apps"
		createNamespace(storageNS)
		createNamespace(appsNS)
		defer deleteNamespace(storageNS)
		defer deleteNamespace(appsNS)

		createReadyPod("ceph", storageNS, false)
		createReadyPod("app", appsNS, true)
		defer deletePod("ceph", storageNS)
		defer deletePod("app", appsNS)

		bs := &popv1alpha1.BootSequence{
			ObjectMeta: metav1.ObjectMeta{Name: "ordered", Namespace: "default"},
			Spec: popv1alpha1.BootSequenceSpec{
				Stages: []popv1alpha1.BootStage{
					{Name: "storage", Selector: popv1alpha1.BootStageSelector{Namespaces: []string{storageNS}}, TimeoutSeconds: 3600},
					{Name: "apps", Selector: popv1alpha1.BootStageSelector{Namespaces: []string{appsNS}}, DependsOn: []string{"storage"}, TimeoutSeconds: 3600},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bs)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, bs) }()

		_, err := reconcileBS("ordered")
		Expect(err).NotTo(HaveOccurred())
		got := getBS("ordered")
		Expect(got.Status.Phase).To(Equal(popv1alpha1.BootSequencePhaseInProgress))
		Expect(stageByName(got.Status.Stages, "storage").Ready).To(BeFalse())
		Expect(stageByName(got.Status.Stages, "apps").Ready).To(BeFalse())
		Expect(stageByName(got.Status.Stages, "apps").Message).To(ContainSubstring("waiting on dependencies"))

		ceph := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ceph", Namespace: storageNS}, ceph)).To(Succeed())
		ceph.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
		Expect(k8sClient.Status().Update(ctx, ceph)).To(Succeed())

		_, err = reconcileBS("ordered")
		Expect(err).NotTo(HaveOccurred())
		got = getBS("ordered")
		Expect(got.Status.Phase).To(Equal(popv1alpha1.BootSequencePhaseComplete))
		Expect(stageByName(got.Status.Stages, "storage").Ready).To(BeTrue())
		Expect(stageByName(got.Status.Stages, "apps").Ready).To(BeTrue())
	})

	It("keeps an invalid DAG retryable and completes after the spec is fixed", func() {
		ns := "bs-cycle-fix"
		createNamespace(ns)
		defer deleteNamespace(ns)
		createReadyPod("p", ns, true)
		defer deletePod("p", ns)

		bs := &popv1alpha1.BootSequence{
			ObjectMeta: metav1.ObjectMeta{Name: "cycle-fix", Namespace: "default"},
			Spec: popv1alpha1.BootSequenceSpec{
				Stages: []popv1alpha1.BootStage{
					{Name: "a", Selector: popv1alpha1.BootStageSelector{Namespaces: []string{ns}}, DependsOn: []string{"b"}, TimeoutSeconds: 3600},
					{Name: "b", Selector: popv1alpha1.BootStageSelector{Namespaces: []string{ns}}, DependsOn: []string{"a"}, TimeoutSeconds: 3600},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bs)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, bs) }()

		_, err := reconcileBS("cycle-fix")
		Expect(err).NotTo(HaveOccurred())
		got := getBS("cycle-fix")
		Expect(got.Status.Phase).NotTo(Equal(popv1alpha1.BootSequencePhaseFailed))
		Expect(got.Status.Message).To(ContainSubstring("cycle"))

		got.Spec.Stages = []popv1alpha1.BootStage{{
			Name: "a", Selector: popv1alpha1.BootStageSelector{Namespaces: []string{ns}}, TimeoutSeconds: 3600,
		}}
		Expect(k8sClient.Update(ctx, got)).To(Succeed())

		_, err = reconcileBS("cycle-fix")
		Expect(err).NotTo(HaveOccurred())
		got = getBS("cycle-fix")
		Expect(got.Status.Phase).To(Equal(popv1alpha1.BootSequencePhaseComplete))
	})

	It("re-runs a completed sequence when a new ClusterRestart appears", func() {
		ns := "bs-rerun"
		createNamespace(ns)
		defer deleteNamespace(ns)
		createReadyPod("p", ns, true)
		defer deletePod("p", ns)

		bs := &popv1alpha1.BootSequence{
			ObjectMeta: metav1.ObjectMeta{Name: "rerun", Namespace: "default"},
			Spec: popv1alpha1.BootSequenceSpec{
				Stages: []popv1alpha1.BootStage{{
					Name:           "storage",
					Selector:       popv1alpha1.BootStageSelector{Namespaces: []string{ns}},
					TimeoutSeconds: 3600,
				}},
			},
		}
		Expect(k8sClient.Create(ctx, bs)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, bs) }()

		_, err := reconcileBS("rerun")
		Expect(err).NotTo(HaveOccurred())
		got := getBS("rerun")
		Expect(got.Status.Phase).To(Equal(popv1alpha1.BootSequencePhaseComplete))
		latchedUID := got.Status.LastClusterRestartUID

		_, err = reconcileBS("rerun")
		Expect(err).NotTo(HaveOccurred())
		got = getBS("rerun")
		Expect(got.Status.Phase).To(Equal(popv1alpha1.BootSequencePhaseComplete))
		Expect(got.Status.LastClusterRestartUID).To(Equal(latchedUID))

		restart := &popv1alpha1.ClusterRestart{
			ObjectMeta: metav1.ObjectMeta{Name: "boot-2", Namespace: "default"},
		}
		Expect(k8sClient.Create(ctx, restart)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, restart) }()
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "boot-2", Namespace: "default"}, restart)).To(Succeed())

		_, err = reconcileBS("rerun")
		Expect(err).NotTo(HaveOccurred())
		got = getBS("rerun")
		Expect(got.Status.LastClusterRestartUID).To(Equal(string(restart.UID)))
		Expect(got.Status.LastClusterRestartUID).NotTo(Equal(latchedUID))
		Expect(got.Status.Phase).To(Or(
			Equal(popv1alpha1.BootSequencePhaseInProgress),
			Equal(popv1alpha1.BootSequencePhaseComplete),
		))
	})

	It("starts a dependent stage's timeout only after it becomes eligible", func() {
		storageNS, appsNS := "bs-force-storage", "bs-force-apps"
		createNamespace(storageNS)
		createNamespace(appsNS)
		defer deleteNamespace(storageNS)
		defer deleteNamespace(appsNS)
		createReadyPod("ceph", storageNS, false)
		createReadyPod("app", appsNS, false)
		defer deletePod("ceph", storageNS)
		defer deletePod("app", appsNS)

		bs := &popv1alpha1.BootSequence{
			ObjectMeta: metav1.ObjectMeta{Name: "force-timeout", Namespace: "default"},
			Spec: popv1alpha1.BootSequenceSpec{
				Stages: []popv1alpha1.BootStage{
					{Name: "storage", Selector: popv1alpha1.BootStageSelector{Namespaces: []string{storageNS}}, TimeoutSeconds: 30, OnStuck: "force"},
					{Name: "apps", Selector: popv1alpha1.BootStageSelector{Namespaces: []string{appsNS}}, DependsOn: []string{"storage"}, TimeoutSeconds: 30, OnStuck: "haltAndAlert"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bs)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, bs) }()

		_, err := reconcileBS("force-timeout")
		Expect(err).NotTo(HaveOccurred())

		got := getBS("force-timeout")
		past := metav1.NewTime(time.Now().Add(-31 * time.Second))
		got.Status.StartTime = &past
		if st := stageByName(got.Status.Stages, "storage"); st.EligibleSince != nil {
			for i := range got.Status.Stages {
				if got.Status.Stages[i].Name == "storage" {
					got.Status.Stages[i].EligibleSince = &past
				}
			}
		}
		Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())

		_, err = reconcileBS("force-timeout")
		Expect(err).NotTo(HaveOccurred())
		got = getBS("force-timeout")
		Expect(got.Status.Phase).To(Equal(popv1alpha1.BootSequencePhaseInProgress))
		Expect(stageByName(got.Status.Stages, "storage").Forced).To(BeTrue())
		Expect(stageByName(got.Status.Stages, "apps").Ready).To(BeFalse())
		Expect(got.Status.Phase).NotTo(Equal(popv1alpha1.BootSequencePhaseTimedOut))
	})
})
