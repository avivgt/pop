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
	"testing"

	workv1 "open-cluster-management.io/api/work/v1"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

func TestBuildManifestWork(t *testing.T) {
	cs := &popv1alpha1.ClusterShutdown{}
	cs.Name = "fs-1"
	cs.Namespace = "pop-system"
	cs.Spec.PVCName = "etcd-backup"

	mw, err := buildManifestWork("site-042", "fs-1", cs)
	if err != nil {
		t.Fatalf("buildManifestWork: %v", err)
	}
	if mw.Namespace != "site-042" {
		t.Errorf("ManifestWork namespace = %q, want site-042 (per-cluster hub ns)", mw.Namespace)
	}
	if mw.Name != "pop-shutdown-fs-1" {
		t.Errorf("ManifestWork name = %q, want pop-shutdown-fs-1", mw.Name)
	}
	if mw.Labels[manifestWorkLabel] != "fs-1" {
		t.Errorf("missing fleetshutdown label")
	}
	if len(mw.Spec.Workload.Manifests) != 1 {
		t.Fatalf("want 1 wrapped manifest, got %d", len(mw.Spec.Workload.Manifests))
	}

	// The wrapped manifest must be a ClusterShutdown carrying the spec through.
	var wrapped popv1alpha1.ClusterShutdown
	if err := json.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, &wrapped); err != nil {
		t.Fatalf("unmarshal wrapped manifest: %v", err)
	}
	if wrapped.Kind != "ClusterShutdown" || wrapped.Spec.PVCName != "etcd-backup" {
		t.Fatalf("wrapped manifest wrong: kind=%q pvc=%q", wrapped.Kind, wrapped.Spec.PVCName)
	}

	if len(mw.Spec.ManifestConfigs) != 1 {
		t.Fatalf("want 1 manifestConfig, got %d", len(mw.Spec.ManifestConfigs))
	}
	mc := mw.Spec.ManifestConfigs[0]
	if mc.ResourceIdentifier.Resource != "clustershutdowns" || mc.ResourceIdentifier.Name != "fs-1" {
		t.Errorf("resourceIdentifier wrong: %+v", mc.ResourceIdentifier)
	}
	if len(mc.FeedbackRules) != 1 || mc.FeedbackRules[0].Type != workv1.JSONPathsType {
		t.Fatalf("expected one JSONPaths feedbackRule")
	}
}

func TestPhaseFromFeedback(t *testing.T) {
	complete := "Complete"
	mw := &workv1.ManifestWork{}
	mw.Status.ResourceStatus.Manifests = []workv1.ManifestCondition{
		{
			StatusFeedbacks: workv1.StatusFeedbackResult{
				Values: []workv1.FeedbackValue{
					{Name: "phase", Value: workv1.FieldValue{Type: workv1.String, String: &complete}},
				},
			},
		},
	}
	if got := phaseFromFeedback(mw); got != "Complete" {
		t.Fatalf("phaseFromFeedback = %q, want Complete", got)
	}

	// No feedback yet -> empty.
	if got := phaseFromFeedback(&workv1.ManifestWork{}); got != "" {
		t.Fatalf("phaseFromFeedback (empty) = %q, want \"\"", got)
	}
}
