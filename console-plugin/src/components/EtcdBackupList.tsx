import { POP_NAMESPACE } from '../constants';
import React from 'react';
import {
  PageSection,
  Title,
  EmptyState,
  EmptyStateBody,
  Spinner,
  Bullseye,
  Breadcrumb,
  BreadcrumbItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import { EtcdBackupModel } from '../models';
import { EtcdBackupResource } from '../types';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';

const NAMESPACE = POP_NAMESPACE;

const EtcdBackupList: React.FC = () => {
  const [etcdBackups, loaded, error] = useK8sWatchResource<EtcdBackupResource[]>({
    groupVersionKind: {
      group: EtcdBackupModel.apiGroup,
      version: EtcdBackupModel.apiVersion,
      kind: EtcdBackupModel.kind,
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
            Error loading EtcdBackups: {(error as Error)?.message || 'Unknown error'}
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  const sortedBackups = [...(etcdBackups || [])].sort((a, b) => {
    const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
    const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
    return bTime - aTime;
  });

  return (
    <>
      <PageSection variant="default">
        <Breadcrumb>
          <BreadcrumbItem to="/backup-and-restore">Dashboard</BreadcrumbItem>
          <BreadcrumbItem to="/backup-and-restore/backup">etcd Backup</BreadcrumbItem>
          <BreadcrumbItem isActive>All Backups</BreadcrumbItem>
        </Breadcrumb>
        <Title headingLevel="h1" style={{ marginTop: 8 }}>All Backups</Title>
      </PageSection>
      <PageSection>
        {sortedBackups.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No EtcdBackup resources found.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="EtcdBackups table" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Phase</Th>
                <Th>Node</Th>
                <Th>Size</Th>
                <Th>PVC %</Th>
                <Th>Fits</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {sortedBackups.map((backup) => (
                <Tr key={backup.metadata?.uid}>
                  <Td dataLabel="Name">{backup.metadata?.name || '-'}</Td>
                  <Td dataLabel="Phase">
                    <PhaseLabel phase={backup.status?.phase} />
                  </Td>
                  <Td dataLabel="Node">{backup.status?.backupNode || '-'}</Td>
                  <Td dataLabel="Size">{backup.status?.backupSize || '-'}</Td>
                  <Td dataLabel="PVC %">
                    {backup.status?.pvcUsagePercent != null
                      ? `${backup.status.pvcUsagePercent}%`
                      : '-'}
                  </Td>
                  <Td dataLabel="Fits">
                    {backup.status?.estimatedCapacity != null
                      ? backup.status.estimatedCapacity
                      : '-'}
                  </Td>
                  <Td dataLabel="Age">
                    {formatTimestamp(backup.metadata?.creationTimestamp)}
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

export default EtcdBackupList;
