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

package v1alpha1

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestExportTargetDeepCopyIsIndependent guards against shallow copies of the
// nested *S3Config pointer: mutating the copy must not affect the original.
func TestExportTargetDeepCopyIsIndependent(t *testing.T) {
	orig := &ExportTarget{
		Type: ExportTargetS3,
		S3: &S3Config{
			Bucket:            "backups",
			Region:            "us-east-1",
			CredentialsSecret: "s3-creds",
			Prefix:            "etcd/",
		},
	}
	cp := orig.DeepCopy()

	if !reflect.DeepEqual(orig, cp) {
		t.Fatalf("DeepCopy not equal to original:\n orig=%+v\n copy=%+v", orig, cp)
	}
	if orig.S3 == cp.S3 {
		t.Fatal("DeepCopy shares the same *S3Config pointer as the original")
	}

	cp.S3.Bucket = "mutated"
	if orig.S3.Bucket == "mutated" {
		t.Fatal("mutating the copy's S3.Bucket leaked into the original")
	}
}

// TestPreflightStatusDeepCopyIsIndependent guards against shallow copies of the
// DegradedOperators slice.
func TestPreflightStatusDeepCopyIsIndependent(t *testing.T) {
	orig := &PreflightStatus{
		DegradedOperators: []string{"etcd", "kube-apiserver"},
		UpdateInProgress:  true,
		Passed:            false,
		Message:           "operators degraded",
	}
	cp := orig.DeepCopy()

	if !reflect.DeepEqual(orig, cp) {
		t.Fatalf("DeepCopy not equal to original:\n orig=%+v\n copy=%+v", orig, cp)
	}
	if len(cp.DegradedOperators) > 0 && &orig.DegradedOperators[0] == &cp.DegradedOperators[0] {
		t.Fatal("DeepCopy shares the same DegradedOperators backing array")
	}

	cp.DegradedOperators[0] = "mutated"
	if orig.DegradedOperators[0] == "mutated" {
		t.Fatal("mutating the copy's slice element leaked into the original")
	}
}

// TestPowerEventDeepCopyObject verifies the runtime.Object contract used by the
// controller-runtime client cache.
func TestPowerEventDeepCopyObject(t *testing.T) {
	orig := &PowerEvent{
		ObjectMeta: metav1.ObjectMeta{Name: "pe-1", Namespace: "pop-system"},
		Spec: PowerEventSpec{
			EventType:   PowerEventTypeFailure,
			Source:      "ups-1",
			TriggeredBy: "webhook",
			Message:     "line lost",
		},
	}

	obj := orig.DeepCopyObject()
	cp, ok := obj.(*PowerEvent)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *PowerEvent", obj)
	}
	if cp == orig {
		t.Fatal("DeepCopyObject returned the same pointer")
	}
	if !reflect.DeepEqual(orig, cp) {
		t.Fatalf("DeepCopyObject not equal:\n orig=%+v\n copy=%+v", orig, cp)
	}

	// A nil receiver must return a nil runtime.Object, not panic.
	var nilPE *PowerEvent
	if got := nilPE.DeepCopyObject(); !isNilObject(got) {
		t.Fatalf("nil receiver DeepCopyObject = %v, want nil", got)
	}
}

func isNilObject(o runtime.Object) bool {
	if o == nil {
		return true
	}
	v := reflect.ValueOf(o)
	return v.Kind() == reflect.Ptr && v.IsNil()
}
