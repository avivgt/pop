#!/usr/bin/env bash
set -euo pipefail
# Vendors the ACM (open-cluster-management.io) CRDs needed for the hub controller's
# envtest suite, from the pinned open-cluster-management.io/api module.
DEST="test/acm-crds"
mkdir -p "${DEST}"
MOD_DIR="$(go list -m -f '{{.Dir}}' open-cluster-management.io/api)"

cp "${MOD_DIR}/work/v1/0000_00_work.open-cluster-management.io_manifestworks.crd.yaml" "${DEST}/"
cp "${MOD_DIR}/cluster/v1/0000_00_clusters.open-cluster-management.io_managedclusters.crd.yaml" "${DEST}/"
cp "${MOD_DIR}/cluster/v1beta1/0000_02_clusters.open-cluster-management.io_placements.crd.yaml" "${DEST}/"
cp "${MOD_DIR}/cluster/v1beta1/0000_03_clusters.open-cluster-management.io_placementdecisions.crd.yaml" "${DEST}/"

echo "Vendored ACM CRDs into ${DEST}:"
ls -1 "${DEST}"
