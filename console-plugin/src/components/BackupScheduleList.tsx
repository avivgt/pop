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
import { BackupScheduleModel } from '../models';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';

interface BackupScheduleResource extends K8sResourceCommon {
  spec?: {
    schedule?: string;
    suspend?: boolean;
  };
  status?: {
    phase?: string;
    lastBackupTime?: string;
  };
}

const BackupScheduleList: React.FC = () => {
  const [backupSchedules, loaded, error] = useK8sWatchResource<BackupScheduleResource[]>({
    groupVersionKind: {
      group: BackupScheduleModel.apiGroup,
      version: BackupScheduleModel.apiVersion,
      kind: BackupScheduleModel.kind,
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
            Error loading BackupSchedules: {(error as Error)?.message || 'Unknown error'}
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  const sortedSchedules = [...(backupSchedules || [])].sort((a, b) => {
    const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
    const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
    return bTime - aTime;
  });

  return (
    <>
      <PageSection variant="light">
        <Title headingLevel="h1">BackupSchedules</Title>
      </PageSection>
      <PageSection>
        {sortedSchedules.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No BackupSchedule resources found.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="BackupSchedules table" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Schedule</Th>
                <Th>Suspend</Th>
                <Th>Last Backup</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {sortedSchedules.map((schedule) => (
                <Tr key={schedule.metadata?.uid}>
                  <Td dataLabel="Name">{schedule.metadata?.name || '-'}</Td>
                  <Td dataLabel="Schedule">{schedule.spec?.schedule || '-'}</Td>
                  <Td dataLabel="Suspend">
                    {schedule.spec?.suspend !== undefined ? (
                      <PhaseLabel phase={schedule.spec.suspend ? 'Suspended' : 'Active'} />
                    ) : (
                      '-'
                    )}
                  </Td>
                  <Td dataLabel="Last Backup">
                    {formatTimestamp(schedule.status?.lastBackupTime)}
                  </Td>
                  <Td dataLabel="Age">
                    {formatTimestamp(schedule.metadata?.creationTimestamp)}
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

export default BackupScheduleList;
