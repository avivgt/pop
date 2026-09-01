package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
)

const (
	ClaimPowerStatus     = "power-status.pop.openshift.io"
	ClaimLastBackupTime  = "last-backup-time.pop.openshift.io"
	ClaimOperatorVersion = "operator-version.pop.openshift.io"
)

// ensureClusterClaim creates or updates a ClusterClaim with the given value.
// On non-ACM clusters where the CRD is not registered, it returns nil silently.
func ensureClusterClaim(ctx context.Context, c client.Client, name, value string) error {
	logger := log.FromContext(ctx).V(1)

	existing := &clusterv1alpha1.ClusterClaim{}
	err := c.Get(ctx, client.ObjectKey{Name: name}, existing)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			if isNoKindMatch(err) {
				logger.Info("ClusterClaim CRD not registered, skipping", "claim", name)
				return nil
			}
			return err
		}
		claim := &clusterv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{"pop.openshift.io/managed-by": "pop-operator"},
			},
			Spec: clusterv1alpha1.ClusterClaimSpec{Value: value},
		}
		if cerr := c.Create(ctx, claim); cerr != nil {
			if isNoKindMatch(cerr) {
				return nil
			}
			return cerr
		}
		logger.Info("created ClusterClaim", "claim", name, "value", value)
		return nil
	}

	if existing.Spec.Value == value {
		return nil
	}
	existing.Spec.Value = value
	if uerr := c.Update(ctx, existing); uerr != nil {
		return uerr
	}
	logger.Info("updated ClusterClaim", "claim", name, "value", value)
	return nil
}

func isNoKindMatch(err error) bool {
	_, ok := err.(*apierrors.StatusError)
	if ok {
		return false
	}
	// meta.NoKindMatchError is returned when the CRD is not registered in the scheme
	// or the discovery cache has no mapping for the GVK.
	return true
}
