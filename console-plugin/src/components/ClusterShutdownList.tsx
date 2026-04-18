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
import { ClusterShutdownModel } from '../models';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';

interface ClusterShutdownResource extends K8sResourceCommon {
  spec?: {
    dryRun?: boolean;
  };
  status?: {
    phase?: string;
    shutdownNodes?: number;
    totalNodes?: number;
  };
}

const ClusterShutdownList: React.FC = () => {
  const [clusterShutdowns, loaded, error] = useK8sWatchResource<ClusterShutdownResource[]>({
    groupVersionKind: {
      group: ClusterShutdownModel.apiGroup,
      version: ClusterShutdownModel.apiVersion,
      kind: ClusterShutdownModel.kind,
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
            Error loading ClusterShutdowns: {(error as Error)?.message || 'Unknown error'}
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  const sortedShutdowns = [...(clusterShutdowns || [])].sort((a, b) => {
    const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
    const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
    return bTime - aTime;
  });

  return (
    <>
      <PageSection variant="light">
        <Title headingLevel="h1">ClusterShutdowns</Title>
      </PageSection>
      <PageSection>
        {sortedShutdowns.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No ClusterShutdown resources found.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="ClusterShutdowns table" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Phase</Th>
                <Th>DryRun</Th>
                <Th>Nodes</Th>
                <Th>Shutdown Count</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {sortedShutdowns.map((shutdown) => (
                <Tr key={shutdown.metadata?.uid}>
                  <Td dataLabel="Name">{shutdown.metadata?.name || '-'}</Td>
                  <Td dataLabel="Phase">
                    <PhaseLabel phase={shutdown.status?.phase} />
                  </Td>
                  <Td dataLabel="DryRun">
                    {shutdown.spec?.dryRun !== undefined
                      ? String(shutdown.spec.dryRun)
                      : '-'}
                  </Td>
                  <Td dataLabel="Nodes">
                    {shutdown.status?.totalNodes !== undefined
                      ? String(shutdown.status.totalNodes)
                      : '-'}
                  </Td>
                  <Td dataLabel="Shutdown Count">
                    {shutdown.status?.shutdownNodes !== undefined
                      ? `${shutdown.status.shutdownNodes}/${shutdown.status.totalNodes ?? '-'}`
                      : '-'}
                  </Td>
                  <Td dataLabel="Age">
                    {formatTimestamp(shutdown.metadata?.creationTimestamp)}
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

export default ClusterShutdownList;
