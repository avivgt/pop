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
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

type popTier = popv1alpha1.FleetBootTier

func TestClusterAvailable(t *testing.T) {
	s := runtime.NewScheme()
	if err := clusterv1.Install(s); err != nil {
		t.Fatalf("install cluster/v1: %v", err)
	}

	avail := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "site-a"}}
	avail.Status.Conditions = []metav1.Condition{{
		Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue,
		Reason: "Ok", LastTransitionTime: metav1.Now(),
	}}
	notAvail := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "site-b"}}
	notAvail.Status.Conditions = []metav1.Condition{{
		Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionFalse,
		Reason: "Down", LastTransitionTime: metav1.Now(),
	}}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(avail, notAvail).Build()

	cases := []struct {
		name string
		want bool
	}{
		{"site-a", true},
		{"site-b", false},
		{"missing", false},
	}
	for _, tc := range cases {
		got, err := clusterAvailable(context.Background(), c, tc.name)
		if err != nil {
			t.Fatalf("clusterAvailable(%s): %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("clusterAvailable(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTopoSortTiers(t *testing.T) {
	// valid DAG
	order, err := topoSortTiers([]popTier{
		{Name: "edge", DependsOn: []string{"shared"}},
		{Name: "shared"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 2 || order[0] != "shared" || order[1] != "edge" {
		t.Fatalf("order = %v, want [shared edge]", order)
	}

	// unknown dependency
	if _, err := topoSortTiers([]popTier{{Name: "a", DependsOn: []string{"nope"}}}); err == nil {
		t.Fatal("expected error for unknown dependency")
	}

	// cycle
	if _, err := topoSortTiers([]popTier{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
	}); err == nil {
		t.Fatal("expected error for dependency cycle")
	}
}
