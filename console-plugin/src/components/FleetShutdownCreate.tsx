import { POP_NAMESPACE } from '../constants';
import React, { useState } from 'react';
import {
  PageSection,
  Title,
  Card,
  CardBody,
  Form,
  FormGroup,
  TextInput,
  Checkbox,
  Button,
  Alert,
  ActionGroup,
  FormSelect,
  FormSelectOption,
  Spinner,
} from '@patternfly/react-core';
import {
  useK8sWatchResource,
  k8sCreate,
} from '@openshift-console/dynamic-plugin-sdk';
import { FleetShutdownModel, PlacementModel } from '../models';
import { PlacementResource } from '../types';

const NAMESPACE = POP_NAMESPACE;

const FleetShutdownCreate: React.FC = () => {
  const [placementRef, setPlacementRef] = useState('');
  const [pvcName, setPvcName] = useState('etcd-backup');
  const [reason, setReason] = useState('');
  const [dryRun, setDryRun] = useState(true);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [placements, placementsLoaded] = useK8sWatchResource<PlacementResource[]>({
    groupVersionKind: {
      group: PlacementModel.apiGroup,
      version: PlacementModel.apiVersion,
      kind: PlacementModel.kind,
    },
    namespace: NAMESPACE,
    isList: true,
    namespaced: true,
  });

  const handleSubmit = async () => {
    if (!placementRef || !pvcName) {
      setError('Placement and PVC Name are required.');
      return;
    }
    setCreating(true);
    setError(null);
    try {
      const name = `fleet-shutdown-${Date.now()}`;
      await k8sCreate({
        model: FleetShutdownModel,
        data: {
          apiVersion: `${FleetShutdownModel.apiGroup}/${FleetShutdownModel.apiVersion}`,
          kind: FleetShutdownModel.kind,
          metadata: { name, namespace: NAMESPACE },
          spec: {
            placementRef,
            pvcName,
            reason: reason || undefined,
            dryRun,
          },
        },
      });
      window.location.href = '/multicloud/fleet-operations/shutdowns';
    } catch (err) {
      setError((err as Error)?.message || 'Failed to create FleetShutdown');
      setCreating(false);
    }
  };

  return (
    <>
      <PageSection variant="default">
        <Title headingLevel="h1">Create Fleet Shutdown</Title>
      </PageSection>
      <PageSection>
        {!dryRun && (
          <Alert
            variant="danger"
            isInline
            title="This will shut down real clusters"
            style={{ marginBottom: 16 }}
          >
            Dry-run is disabled. Submitting will initiate a real shutdown of all selected managed clusters.
          </Alert>
        )}
        {error && (
          <Alert variant="warning" isInline title="Error" style={{ marginBottom: 16 }}>
            {error}
          </Alert>
        )}
        <Card>
          <CardBody>
            <Form>
              <FormGroup label="Placement" isRequired fieldId="placement-ref">
                {!placementsLoaded ? (
                  <Spinner size="md" />
                ) : (
                  <FormSelect
                    id="placement-ref"
                    value={placementRef}
                    onChange={(_event, value) => setPlacementRef(value)}
                  >
                    <FormSelectOption value="" label="Select a Placement..." isDisabled />
                    {(placements || []).map((p) => (
                      <FormSelectOption
                        key={p.metadata?.name}
                        value={p.metadata?.name || ''}
                        label={`${p.metadata?.name} (${p.status?.numberOfSelectedClusters ?? 0} clusters)`}
                      />
                    ))}
                  </FormSelect>
                )}
              </FormGroup>
              <FormGroup label="PVC Name" isRequired fieldId="pvc-name">
                <TextInput
                  id="pvc-name"
                  value={pvcName}
                  onChange={(_event, value) => setPvcName(value)}
                />
              </FormGroup>
              <FormGroup label="Reason" fieldId="reason">
                <TextInput
                  id="reason"
                  value={reason}
                  onChange={(_event, value) => setReason(value)}
                  placeholder="Maintenance, power outage, etc."
                />
              </FormGroup>
              <FormGroup fieldId="dry-run">
                <Checkbox
                  id="dry-run"
                  label="Dry Run (simulate without actual shutdown)"
                  isChecked={dryRun}
                  onChange={(_event, checked) => setDryRun(checked)}
                />
              </FormGroup>
              <ActionGroup>
                <Button
                  variant={dryRun ? 'primary' : 'danger'}
                  onClick={handleSubmit}
                  isDisabled={creating || !placementRef || !pvcName}
                  isLoading={creating}
                >
                  {dryRun ? 'Create (Dry Run)' : 'Create Fleet Shutdown'}
                </Button>
                <Button variant="link" component="a" href="/multicloud/fleet-operations/shutdowns">
                  Cancel
                </Button>
              </ActionGroup>
            </Form>
          </CardBody>
        </Card>
      </PageSection>
    </>
  );
};

export default FleetShutdownCreate;
