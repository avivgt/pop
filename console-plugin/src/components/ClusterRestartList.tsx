import React from 'react';
import {
  PageSection,
  Title,
  EmptyState,
  EmptyStateBody,
  Spinner,
  Bullseye,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import { K8sResourceCommon } from '@openshift-console/dynamic-plugin-sdk';
import { ClusterRestartModel } from '../models';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';

interface ClusterRestartResource extends K8sResourceCommon {
  status?: {
    phase?: string;
    readyNodes?: number;
    totalNodes?: number;
    approvedCSRs?: number;
  };
}

const ClusterRestartList: React.FC = () => {
  const [clusterRestarts, loaded, error] = useK8sWatchResource<ClusterRestartResource[]>({
    groupVersionKind: {
      group: ClusterRestartModel.apiGroup,
      version: ClusterRestartModel.apiVersion,
      kind: ClusterRestartModel.kind,
    },
    isList: true,
    namespaced: true,
  });

  if (!loaded) {
    return (
      <PageSection>
        <Bullseye>
          <Spinner size="xl" />
        </Bullseye>
      </PageSection>
    );
  }

  if (error) {
    return (
      <PageSection>
        <EmptyState>
          <EmptyStateBody>
            Error loading ClusterRestarts: {(error as Error)?.message || 'Unknown error'}
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  const sortedRestarts = [...(clusterRestarts || [])].sort((a, b) => {
    const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
    const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
    return bTime - aTime;
  });

  return (
    <>
      <PageSection variant="light">
        <Title headingLevel="h1">ClusterRestarts</Title>
      </PageSection>
      <PageSection>
        {sortedRestarts.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No ClusterRestart resources found.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="ClusterRestarts table" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Phase</Th>
                <Th>Ready Nodes</Th>
                <Th>Total Nodes</Th>
                <Th>CSRs Approved</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {sortedRestarts.map((restart) => (
                <Tr key={restart.metadata?.uid}>
                  <Td dataLabel="Name">{restart.metadata?.name || '-'}</Td>
                  <Td dataLabel="Phase">
                    <PhaseLabel phase={restart.status?.phase} />
                  </Td>
                  <Td dataLabel="Ready Nodes">
                    {restart.status?.readyNodes !== undefined
                      ? String(restart.status.readyNodes)
                      : '-'}
                  </Td>
                  <Td dataLabel="Total Nodes">
                    {restart.status?.totalNodes !== undefined
                      ? String(restart.status.totalNodes)
                      : '-'}
                  </Td>
                  <Td dataLabel="CSRs Approved">
                    {restart.status?.approvedCSRs !== undefined
                      ? String(restart.status.approvedCSRs)
                      : '-'}
                  </Td>
                  <Td dataLabel="Age">
                    {formatTimestamp(restart.metadata?.creationTimestamp)}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </PageSection>
    </>
  );
};

export default ClusterRestartList;
