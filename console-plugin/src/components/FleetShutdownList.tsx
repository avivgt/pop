import { POP_NAMESPACE } from '../constants';
import React, { useState } from 'react';
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
import {
  Table,
  Thead,
  Tr,
  Th,
  Tbody,
  Td,
  ExpandableRowContent,
} from '@patternfly/react-table';
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

  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

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
                <Th screenReaderText="Row expansion" />
                <Th>Name</Th>
                <Th>Phase</Th>
                <Th>Placement</Th>
                <Th>Selected</Th>
                <Th>Completed</Th>
                <Th>Backups</Th>
                <Th>Reason</Th>
                <Th>DryRun</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            {sorted.map((fs, rowIndex) => {
              const key = fs.metadata?.uid || fs.metadata?.name || String(rowIndex);
              const clusters = fs.status?.clusters || [];
              const backedUp = clusters.filter((c) => !!c.backupRef).length;
              const isExpanded = !!expanded[key];
              return (
                <Tbody key={key} isExpanded={isExpanded}>
                  <Tr>
                    <Td
                      expand={
                        clusters.length > 0
                          ? {
                              rowIndex,
                              isExpanded,
                              onToggle: () =>
                                setExpanded((prev) => ({ ...prev, [key]: !prev[key] })),
                              expandId: `fs-expand-${key}`,
                            }
                          : undefined
                      }
                    />
                    <Td dataLabel="Name">{fs.metadata?.name || '-'}</Td>
                    <Td dataLabel="Phase">
                      <PhaseLabel phase={fs.status?.phase} />
                    </Td>
                    <Td dataLabel="Placement">{fs.spec?.placementRef || '-'}</Td>
                    <Td dataLabel="Selected">
                      {fs.status?.selectedClusters !== undefined
                        ? String(fs.status.selectedClusters)
                        : '-'}
                    </Td>
                    <Td dataLabel="Completed">
                      {fs.status?.completedClusters !== undefined
                        ? `${fs.status.completedClusters}/${fs.status.selectedClusters ?? '-'}`
                        : '-'}
                    </Td>
                    <Td dataLabel="Backups">
                      {clusters.length > 0
                        ? `${backedUp}/${clusters.length}`
                        : '-'}
                    </Td>
                    <Td dataLabel="Reason">{fs.spec?.reason || '-'}</Td>
                    <Td dataLabel="DryRun">
                      {fs.spec?.dryRun !== undefined ? String(fs.spec.dryRun) : '-'}
                    </Td>
                    <Td dataLabel="Age">{formatTimestamp(fs.metadata?.creationTimestamp)}</Td>
                  </Tr>
                  {clusters.length > 0 && (
                    <Tr isExpanded={isExpanded}>
                      <Td />
                      <Td dataLabel="Per-cluster detail" colSpan={9}>
                        <ExpandableRowContent>
                          <Table
                            aria-label={`Per-cluster detail for ${fs.metadata?.name}`}
                            variant="compact"
                            borders={false}
                          >
                            <Thead>
                              <Tr>
                                <Th>Cluster</Th>
                                <Th>Phase</Th>
                                <Th>Etcd Backup</Th>
                                <Th>Message</Th>
                              </Tr>
                            </Thead>
                            <Tbody>
                              {clusters.map((c) => (
                                <Tr key={c.name}>
                                  <Td dataLabel="Cluster">{c.name}</Td>
                                  <Td dataLabel="Phase">
                                    <PhaseLabel phase={c.phase} />
                                  </Td>
                                  <Td dataLabel="Etcd Backup">{c.backupRef || '-'}</Td>
                                  <Td dataLabel="Message">{c.message || '-'}</Td>
                                </Tr>
                              ))}
                            </Tbody>
                          </Table>
                        </ExpandableRowContent>
                      </Td>
                    </Tr>
                  )}
                </Tbody>
              );
            })}
          </Table>
        )}
      </PageSection>
    </>
  );
};

export default FleetShutdownList;
