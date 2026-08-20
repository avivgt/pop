import { POP_NAMESPACE } from '../constants';
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
import { FleetBootSequenceModel } from '../models';
import { FleetBootSequenceResource } from '../types';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';

const NAMESPACE = POP_NAMESPACE;

const FleetBootSequenceList: React.FC = () => {
  const [bootSequences, loaded, error] = useK8sWatchResource<FleetBootSequenceResource[]>({
    groupVersionKind: {
      group: FleetBootSequenceModel.apiGroup,
      version: FleetBootSequenceModel.apiVersion,
      kind: FleetBootSequenceModel.kind,
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
            Error loading FleetBootSequences: {(error as Error)?.message || 'Unknown error'}
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  const sorted = [...(bootSequences || [])].sort((a, b) => {
    const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
    const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
    return bTime - aTime;
  });

  return (
    <>
      <PageSection variant="default">
        <Title headingLevel="h1">Fleet Boot Sequences</Title>
      </PageSection>
      <PageSection>
        {sorted.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No FleetBootSequence resources found.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="FleetBootSequences table" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Phase</Th>
                <Th>Tiers</Th>
                <Th>Start Time</Th>
                <Th>Message</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {sorted.map((bs) => (
                <Tr key={bs.metadata?.uid}>
                  <Td dataLabel="Name">{bs.metadata?.name || '-'}</Td>
                  <Td dataLabel="Phase">
                    <PhaseLabel phase={bs.status?.phase} />
                  </Td>
                  <Td dataLabel="Tiers">
                    {bs.spec?.tiers?.length !== undefined ? String(bs.spec.tiers.length) : '-'}
                  </Td>
                  <Td dataLabel="Start Time">
                    {bs.status?.startTime ? formatTimestamp(bs.status.startTime) : '-'}
                  </Td>
                  <Td dataLabel="Message">{bs.status?.message || '-'}</Td>
                  <Td dataLabel="Age">
                    {formatTimestamp(bs.metadata?.creationTimestamp)}
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

export default FleetBootSequenceList;
