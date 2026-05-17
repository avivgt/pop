#!/bin/bash
set -euo pipefail

# Migration script: POP v0.1 -> v0.2
# Converts PowerProtectionConfig resources to AutomatedProceduresConfig
# and removes the embedded powerProtection field from ClusterLifecycleConfig.
#
# Prerequisites:
#   - oc/kubectl logged in with cluster-admin privileges
#   - New CRD (AutomatedProceduresConfig) already installed
#
# Usage:
#   ./scripts/migrate-v0.1-to-v0.2.sh [--namespace pop-system] [--dry-run]

NAMESPACE="pop-system"
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --namespace) NAMESPACE="$2"; shift 2 ;;
    --dry-run) DRY_RUN=true; shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

OC=$(command -v oc 2>/dev/null || command -v kubectl 2>/dev/null)
if [[ -z "$OC" ]]; then
  echo "ERROR: oc or kubectl not found in PATH"
  exit 1
fi

echo "=== POP Migration v0.1 -> v0.2 ==="
echo "Namespace: $NAMESPACE"
echo "Dry run: $DRY_RUN"
echo ""

# Step 1: Check if old CRD exists
if ! $OC get crd powerprotectionconfigs.pop.openshift.io &>/dev/null; then
  echo "Old CRD (PowerProtectionConfig) not found. Nothing to migrate."
  exit 0
fi

# Step 2: Check if new CRD exists
if ! $OC get crd automatedproceduresconfigs.pop.openshift.io &>/dev/null; then
  echo "ERROR: New CRD (AutomatedProceduresConfig) not found."
  echo "Install the new CRDs first: oc apply -f config/crd/bases/"
  exit 1
fi

# Step 3: List old resources
OLD_CONFIGS=$($OC get powerprotectionconfigs -n "$NAMESPACE" -o json 2>/dev/null || echo '{"items":[]}')
COUNT=$(echo "$OLD_CONFIGS" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['items']))")

if [[ "$COUNT" -eq 0 ]]; then
  echo "No PowerProtectionConfig resources found in $NAMESPACE."
else
  echo "Found $COUNT PowerProtectionConfig resource(s) to migrate."
  echo ""

  echo "$OLD_CONFIGS" | python3 -c "
import sys, json

data = json.load(sys.stdin)
for item in data['items']:
    name = item['metadata']['name']
    ns = item['metadata']['namespace']
    spec = item.get('spec', {})

    new_resource = {
        'apiVersion': 'pop.openshift.io/v1alpha1',
        'kind': 'AutomatedProceduresConfig',
        'metadata': {
            'name': name,
            'namespace': ns,
        },
        'spec': {
            'enabled': spec.get('enabled', False),
            'clusterPriority': spec.get('clusterPriority', 'Standard'),
            'batteryCapacityMinutes': spec.get('batteryCapacityMinutes', 15),
            'powerConfirmationSeconds': spec.get('powerConfirmationSeconds', 120),
            'autonomousShutdown': spec.get('autonomousShutdown', False),
            'backupConfig': spec.get('backupConfig', {'pvcName': 'etcd-backup-storage'}),
        },
    }

    if spec.get('alertEndpoint'):
        new_resource['spec']['alertEndpoint'] = spec['alertEndpoint']
        new_resource['spec']['eventSources'] = {
            'webhook': spec['alertEndpoint'],
        }

    if spec.get('managementURL'):
        new_resource['spec']['managementURL'] = spec['managementURL']

    if spec.get('notification'):
        new_resource['spec']['notification'] = spec['notification']

    if spec.get('shutdownDelayOverrideSeconds') is not None:
        new_resource['spec']['shutdownDelayOverrideSeconds'] = spec['shutdownDelayOverrideSeconds']

    print(json.dumps(new_resource))
" | while read -r line; do
    RESOURCE_NAME=$(echo "$line" | python3 -c "import sys,json; print(json.load(sys.stdin)['metadata']['name'])")

    if $OC get automatedproceduresconfigs "$RESOURCE_NAME" -n "$NAMESPACE" &>/dev/null; then
      echo "  SKIP: AutomatedProceduresConfig/$RESOURCE_NAME already exists"
      continue
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
      echo "  DRY-RUN: Would create AutomatedProceduresConfig/$RESOURCE_NAME"
      echo "$line" | python3 -m json.tool
    else
      echo "$line" | $OC apply -n "$NAMESPACE" -f -
      echo "  CREATED: AutomatedProceduresConfig/$RESOURCE_NAME"
    fi
  done
fi

# Step 4: Migrate embedded powerProtection from ClusterLifecycleConfig
echo ""
echo "Checking ClusterLifecycleConfig for embedded powerProtection..."

CLC=$($OC get clusterlifecycleconfigs -n "$NAMESPACE" -o json 2>/dev/null || echo '{"items":[]}')
echo "$CLC" | python3 -c "
import sys, json

data = json.load(sys.stdin)
for item in data['items']:
    name = item['metadata']['name']
    spec = item.get('spec', {})
    pp = spec.get('powerProtection')
    if pp:
        print(f'FOUND: {name} has embedded powerProtection')
    else:
        print(f'OK: {name} has no embedded powerProtection')
"

# Step 5: Summary
echo ""
if [[ "$DRY_RUN" == "true" ]]; then
  echo "=== DRY RUN COMPLETE ==="
  echo "Re-run without --dry-run to apply changes."
else
  echo "=== MIGRATION COMPLETE ==="
  echo ""
  echo "Next steps:"
  echo "  1. Verify: oc get automatedproceduresconfigs -n $NAMESPACE"
  echo "  2. Upgrade the operator to v0.2"
  echo "  3. After confirming the new config works, clean up old resources:"
  echo "     oc delete powerprotectionconfigs --all -n $NAMESPACE"
  echo "     oc delete crd powerprotectionconfigs.pop.openshift.io"
fi
