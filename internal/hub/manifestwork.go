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
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	workv1 "open-cluster-management.io/api/work/v1"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// manifestWorkLabel links a ManifestWork back to the FleetShutdown that created it.
const manifestWorkLabel = "pop.openshift.io/fleetshutdown"

// manifestWorkName is the deterministic ManifestWork name for a FleetShutdown.
func manifestWorkName(fleetShutdownName string) string {
	return "pop-shutdown-" + fleetShutdownName
}

// buildManifestWork wraps a ClusterShutdown in a ManifestWork targeted at the
// given managed cluster. The ManifestWork lives in the per-cluster hub namespace
// (namespace == managed cluster name); a JSONPaths feedbackRule surfaces the
// wrapped ClusterShutdown's status.phase back to the hub.
func buildManifestWork(clusterName, fleetShutdownName string, cs *popv1alpha1.ClusterShutdown) (*workv1.ManifestWork, error) {
	cs.APIVersion = popv1alpha1.GroupVersion.String()
	cs.Kind = "ClusterShutdown"
	raw, err := json.Marshal(cs)
	if err != nil {
		return nil, err
	}

	return &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      manifestWorkName(fleetShutdownName),
			Namespace: clusterName,
			Labels:    map[string]string{manifestWorkLabel: fleetShutdownName},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{RawExtension: runtime.RawExtension{Raw: raw}},
				},
			},
			ManifestConfigs: []workv1.ManifestConfigOption{
				{
					ResourceIdentifier: workv1.ResourceIdentifier{
						Group:     popv1alpha1.GroupVersion.Group,
						Resource:  "clustershutdowns",
						Namespace: cs.Namespace,
						Name:      cs.Name,
					},
					FeedbackRules: []workv1.FeedbackRule{
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{Name: "phase", Path: ".status.phase"},
								{Name: "message", Path: ".status.message"},
								{Name: "backupRef", Path: ".status.etcdBackupRef"},
							},
						},
					},
				},
			},
		},
	}, nil
}

// feedbackString returns the string-typed status feedback value with the given
// alias name from a ManifestWork, or "" if not reported yet.
func feedbackString(mw *workv1.ManifestWork, name string) string {
	for _, m := range mw.Status.ResourceStatus.Manifests {
		for _, v := range m.StatusFeedbacks.Values {
			if v.Name == name && v.Value.String != nil {
				return *v.Value.String
			}
		}
	}
	return ""
}

// phaseFromFeedback returns the wrapped ClusterShutdown's status.phase.
func phaseFromFeedback(mw *workv1.ManifestWork) string {
	return feedbackString(mw, "phase")
}

// backupRefFromFeedback returns the wrapped ClusterShutdown's
// status.etcdBackupRef (the EtcdBackup CR name), or "" if not reported yet.
func backupRefFromFeedback(mw *workv1.ManifestWork) string {
	return feedbackString(mw, "backupRef")
}
