import React from 'react';
import {
  Card,
  CardTitle,
  CardBody,
  Grid,
  GridItem,
  Button,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  EmptyState,
  EmptyStateBody,
  Title,
  Tooltip,
} from '@patternfly/react-core';
import { RedhatIcon } from '@patternfly/react-icons';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  ClusterLifecycleConfigResource,
  EtcdBackupResource,
} from '../types';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';
import BackupNowModal from './actions/BackupNowModal';

interface BackupSectionProps {
  config: ClusterLifecycleConfigResource | undefined;
  backups: EtcdBackupResource[];
  backupsLoaded: boolean;
  namespace: string;
}

const BackupSection: React.FC<BackupSectionProps> = ({
  config,
  backups,
  backupsLoaded,
  namespace,
}) => {
  const [isModalOpen, setIsModalOpen] = React.useState(false);

  const pvcName = config?.spec?.backup?.storage?.pvcName || 'etcd-backup-pvc';
  const scheduleConfig = config?.spec?.backup?.schedule;
  const backupStatus = config?.status?.backup;

  const latestCompleted = [...(backups || [])]
    .filter((b) => b.status?.phase === 'Completed' && b.status?.pvcUsagePercent != null)
    .sort((a, b) => {
      const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
      const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
      return bTime - aTime;
    })[0];

  const sortedBackups = [...(backups || [])]
    .sort((a, b) => {
      const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
      const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
      return bTime - aTime;
    })
    .slice(0, 10);

  return (
    <Card>
      <CardTitle>
        <Title headingLevel="h2">etcd Backup</Title>
      </CardTitle>
      <CardBody>
        <Grid hasGutter>
          <GridItem span={4} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
            <Tooltip content="Trigger an etcd backup now">
              <Button
                variant="plain"
                onClick={() => setIsModalOpen(true)}
                style={{
                  width: 80,
                  height: 80,
                  borderRadius: '50%',
                  backgroundColor: '#EE0000',
                  color: '#FFFFFF',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 40,
                  boxShadow: '0 2px 8px rgba(0,0,0,0.2)',
                }}
              >
                <RedhatIcon />
              </Button>
            </Tooltip>
            <div style={{ marginTop: 8, fontWeight: 600 }}>Backup Now</div>
          </GridItem>
          <GridItem span={8}>
            <DescriptionList isCompact isHorizontal>
              <DescriptionListGroup>
                <DescriptionListTerm>Schedule</DescriptionListTerm>
                <DescriptionListDescription>
                  {scheduleConfig?.enabled !== false
                    ? `${scheduleConfig?.frequency || 'Daily'} at ${scheduleConfig?.time || '02:00'} UTC`
                    : 'Disabled'}
                  {backupStatus?.resolvedSchedule
                    ? ` (${backupStatus.resolvedSchedule})`
                    : ''}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Last backup</DescriptionListTerm>
                <DescriptionListDescription>
                  {formatTimestamp(backupStatus?.lastBackupTime)}
                  {backupStatus?.lastSuccessfulBackup
                    ? ` (${backupStatus.lastSuccessfulBackup})`
                    : ''}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Next backup</DescriptionListTerm>
                <DescriptionListDescription>
                  {formatTimestamp(backupStatus?.nextBackupTime)}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Total / Failed</DescriptionListTerm>
                <DescriptionListDescription>
                  {backupStatus?.totalBackups ?? 0} / {backupStatus?.failedBackups ?? 0}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>PVC usage</DescriptionListTerm>
                <DescriptionListDescription>
                  {latestCompleted?.status?.pvcUsagePercent != null
                    ? `${latestCompleted.status.pvcUsagePercent}%`
                    : 'N/A'}
                  {latestCompleted?.status?.backupSize
                    ? ` (last backup: ${latestCompleted.status.backupSize})`
                    : ''}
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </GridItem>
        </Grid>

        <Title headingLevel="h3" style={{ marginTop: 24, marginBottom: 8 }}>
          Recent backups
        </Title>

        {!backupsLoaded ? (
          <EmptyState>
            <EmptyStateBody>Loading backups...</EmptyStateBody>
          </EmptyState>
        ) : sortedBackups.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No backups found.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="Recent backups" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Phase</Th>
                <Th>Node</Th>
                <Th>Size</Th>
                <Th>PVC %</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {sortedBackups.map((backup) => (
                <Tr key={backup.metadata?.uid || backup.metadata?.name}>
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
                  <Td dataLabel="Age">
                    {formatTimestamp(backup.metadata?.creationTimestamp)}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </CardBody>

      <BackupNowModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        pvcName={pvcName}
        namespace={namespace}
      />
    </Card>
  );
};

export default BackupSection;
