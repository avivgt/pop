import { POP_NAMESPACE } from '../constants';
import React from 'react';
import {
  PageSection,
  Title,
  EmptyState,
  EmptyStateBody,
  Spinner,
  Bullseye,
  Button,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import { FleetShutdownModel } from '../models';
import { FleetShutdownResource } from '../types';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';

const NAMESPACE = POP_NAMESPACE;

const FleetShutdownList: React.FC = () => {
  const [fleetShutdowns, loaded, error] = useK8sWatchResource<FleetShutdownResource[]>({
    groupVersionKind: {
      group: FleetShutdownModel.apiGroup,
      version: FleetShutdownModel.apiVersion,
      kind: FleetShutdownModel.kind,
    },
    namespace: NAMESPACE,
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
            Error loading FleetShutdowns: {(error as Error)?.message || 'Unknown error'}
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  const sorted = [...(fleetShutdowns || [])].sort((a, b) => {
    const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
    const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
    return bTime - aTime;
  });

  return (
    <>
      <PageSection variant="default">
        <Title headingLevel="h1">Fleet Shutdowns</Title>
      </PageSection>
      <PageSection>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Button variant="primary" component="a" href="/multicloud/fleet-operations/shutdowns/create">
                Create Fleet Shutdown
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
        {sorted.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No FleetShutdown resources found.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="FleetShutdowns table" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Phase</Th>
                <Th>Placement</Th>
                <Th>Selected</Th>
                <Th>Completed</Th>
                <Th>Reason</Th>
                <Th>DryRun</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {sorted.map((fs) => (
                <Tr key={fs.metadata?.uid}>
                  <Td dataLabel="Name">{fs.metadata?.name || '-'}</Td>
                  <Td dataLabel="Phase">
                    <PhaseLabel phase={fs.status?.phase} />
                  </Td>
                  <Td dataLabel="Placement">{fs.spec?.placementRef || '-'}</Td>
                  <Td dataLabel="Selected">
                    {fs.status?.selectedClusters !== undefined ? String(fs.status.selectedClusters) : '-'}
                  </Td>
                  <Td dataLabel="Completed">
                    {fs.status?.completedClusters !== undefined
                      ? `${fs.status.completedClusters}/${fs.status.selectedClusters ?? '-'}`
                      : '-'}
                  </Td>
                  <Td dataLabel="Reason">{fs.spec?.reason || '-'}</Td>
                  <Td dataLabel="DryRun">
                    {fs.spec?.dryRun !== undefined ? String(fs.spec.dryRun) : '-'}
                  </Td>
                  <Td dataLabel="Age">
                    {formatTimestamp(fs.metadata?.creationTimestamp)}
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

export default FleetShutdownList;
