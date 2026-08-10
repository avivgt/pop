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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// FleetBootSequenceReconciler walks a FleetBootSequence's tiers in dependency
// order, marking each Ready only when all its clusters are Available. It does
// not power hardware on.
type FleetBootSequenceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *FleetBootSequenceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	fbs := &popv1alpha1.FleetBootSequence{}
	if err := r.Get(ctx, req.NamespacedName, fbs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	fbs.Status.ObservedGeneration = fbs.Generation

	if fbs.Status.Phase == popv1alpha1.FleetBootSequencePhaseComplete ||
		fbs.Status.Phase == popv1alpha1.FleetBootSequencePhaseTimedOut {
		return ctrl.Result{}, nil
	}

	if fbs.Status.StartTime == nil {
		now := metav1.Now()
		fbs.Status.StartTime = &now
		fbs.Status.Phase = popv1alpha1.FleetBootSequencePhaseInProgress
	}

	order, err := topoSortTiers(fbs.Spec.Tiers)
	if err != nil {
		fbs.Status.Message = fmt.Sprintf("invalid tiers: %v", err)
		meta.SetStatusCondition(&fbs.Status.Conditions, metav1.Condition{
			Type: "Valid", Status: metav1.ConditionFalse, Reason: "InvalidTiers", Message: err.Error(),
		})
		if uerr := r.Status().Update(ctx, fbs); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, nil // spec error: wait for the user to fix it
	}

	byName := make(map[string]popv1alpha1.FleetBootTier, len(fbs.Spec.Tiers))
	for _, t := range fbs.Spec.Tiers {
		byName[t.Name] = t
	}

	readyByName := make(map[string]bool, len(order))
	statuses := make([]popv1alpha1.FleetBootTierStatus, 0, len(order))
	elapsed := time.Since(fbs.Status.StartTime.Time)
	allReady, timedOut := true, false

	for _, name := range order {
		tier := byName[name]

		depsReady := true
		for _, dep := range tier.DependsOn {
			if !readyByName[dep] {
				depsReady = false
				break
			}
		}

		total, ready := 0, 0
		msg := ""
		if !depsReady {
			msg = "waiting on dependencies"
		} else {
			clusters, cerr := resolvePlacementDecisions(ctx, r.Client, fbs.Namespace, tier.PlacementRef)
			if cerr != nil {
				return ctrl.Result{}, cerr
			}
			total = len(clusters)
			for _, cn := range clusters {
				ok, aerr := clusterAvailable(ctx, r.Client, cn)
				if aerr != nil {
					return ctrl.Result{}, aerr
				}
				if ok {
					ready++
				}
			}
			if total == 0 {
				msg = "no clusters selected by placement " + tier.PlacementRef
			} else {
				msg = fmt.Sprintf("%d/%d clusters available", ready, total)
			}
		}

		tierReady := depsReady && total > 0 && ready == total
		readyByName[name] = tierReady
		if !tierReady {
			allReady = false
			if int32(elapsed.Seconds()) > tier.TimeoutSeconds {
				timedOut = true
			}
		}
		statuses = append(statuses, popv1alpha1.FleetBootTierStatus{
			Name: name, Ready: tierReady, ClustersTotal: int32(total), ClustersReady: int32(ready), Message: msg,
		})
	}

	fbs.Status.Tiers = statuses

	switch {
	case allReady:
		fbs.Status.Phase = popv1alpha1.FleetBootSequencePhaseComplete
		fbs.Status.Message = "All tiers ready"
		meta.SetStatusCondition(&fbs.Status.Conditions, metav1.Condition{
			Type: "Complete", Status: metav1.ConditionTrue, Reason: "AllTiersReady", Message: "All tiers ready",
		})
		if err := r.Status().Update(ctx, fbs); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	case timedOut:
		fbs.Status.Phase = popv1alpha1.FleetBootSequencePhaseTimedOut
		fbs.Status.Message = "One or more tiers did not become ready within their timeout"
		meta.SetStatusCondition(&fbs.Status.Conditions, metav1.Condition{
			Type: "Complete", Status: metav1.ConditionFalse, Reason: "TierTimeout", Message: fbs.Status.Message,
		})
		if err := r.Status().Update(ctx, fbs); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	default:
		fbs.Status.Phase = popv1alpha1.FleetBootSequencePhaseInProgress
		fbs.Status.Message = "Fleet black-start in progress"
		if err := r.Status().Update(ctx, fbs); err != nil {
			return ctrl.Result{}, err
		}
		logger.V(1).Info("fleet boot sequence in progress", "tiers", len(statuses))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

// topoSortTiers returns the tier names in dependency order, or an error if a
// DependsOn references an unknown tier or the graph has a cycle.
func topoSortTiers(tiers []popv1alpha1.FleetBootTier) ([]string, error) {
	names := make(map[string]bool, len(tiers))
	for _, t := range tiers {
		names[t.Name] = true
	}
	indeg := make(map[string]int, len(tiers))
	deps := make(map[string][]string, len(tiers))
	for _, t := range tiers {
		if _, ok := indeg[t.Name]; !ok {
			indeg[t.Name] = 0
		}
		for _, d := range t.DependsOn {
			if !names[d] {
				return nil, fmt.Errorf("tier %q depends on unknown tier %q", t.Name, d)
			}
			deps[t.Name] = append(deps[t.Name], d)
			indeg[t.Name]++
		}
	}

	var queue, order []string
	for _, t := range tiers {
		if indeg[t.Name] == 0 {
			queue = append(queue, t.Name)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		// Decrement in-degree of tiers that depend on n.
		for _, t := range tiers {
			for _, d := range deps[t.Name] {
				if d == n {
					indeg[t.Name]--
					if indeg[t.Name] == 0 {
						queue = append(queue, t.Name)
					}
				}
			}
		}
	}
	if len(order) != len(tiers) {
		return nil, fmt.Errorf("dependency cycle detected among tiers")
	}
	return order, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FleetBootSequenceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.FleetBootSequence{}).
		Complete(r)
}
