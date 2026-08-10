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

	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// placementLabel links a PlacementDecision to its Placement.
const placementLabel = "cluster.open-cluster-management.io/placement"

// resolvePlacementDecisions returns the managed-cluster names selected by the
// named Placement, read from its PlacementDecision(s) in the given namespace.
func resolvePlacementDecisions(ctx context.Context, c client.Client, ns, name string) ([]string, error) {
	pdList := &clusterv1beta1.PlacementDecisionList{}
	if err := c.List(ctx, pdList,
		client.InNamespace(ns),
		client.MatchingLabels{placementLabel: name},
	); err != nil {
		return nil, err
	}
	var names []string
	for i := range pdList.Items {
		for _, d := range pdList.Items[i].Status.Decisions {
			names = append(names, d.ClusterName)
		}
	}
	return names, nil
}
