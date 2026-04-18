import React from 'react';
import {
  Card,
  CardBody,
  CardTitle,
  Grid,
  GridItem,
  PageSection,
  Title,
  EmptyState,
  EmptyStateBody,
  Spinner,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  List,
  ListItem,
} from '@patternfly/react-core';
import { useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import { K8sResourceCommon } from '@openshift-console/dynamic-plugin-sdk';
import { EtcdBackupModel, BackupScheduleModel, ClusterShutdownModel, ClusterRestartModel } from '../models';
import { formatTimestamp, formatAbsoluteTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';

interface EtcdBackupResource extends K8sResourceCommon {
  status?: {
    phase?: string;
    node?: string;
    completionTimestamp?: string;
  };
}

interface BackupScheduleResource extends K8sResourceCommon {
  spec?: {
    schedule?: string;
    suspend?: boolean;
  };
  status?: {
    phase?: string;
    lastBackupTime?: string;
    nextBackupTime?: string;
  };
}

interface ClusterShutdownResource extends K8sResourceCommon {
  status?: {
    phase?: string;
    certificateExpiry?: string;
  };
}

interface ClusterRestartResource extends K8sResourceCommon {
  status?: {
    phase?: string;
  };
}

const DashboardPage: React.FC = () => {
  const [etcdBackups, etcdBackupsLoaded, etcdBackupsError] = useK8sWatchResource<EtcdBackupResource[]>({
    groupVersionKind: {
      group: EtcdBackupModel.apiGroup,
      version: EtcdBackupModel.apiVersion,
      kind: EtcdBackupModel.kind,
    },
    isList: true,
    namespaced: true,
  });

  const [backupSchedules, schedulesLoaded, schedulesError] = useK8sWatchResource<BackupScheduleResource[]>({
    groupVersionKind: {
      group: BackupScheduleModel.apiGroup,
      version: BackupScheduleModel.apiVersion,
      kind: BackupScheduleModel.kind,
    },
    isList: true,
    namespaced: true,
  });

  const [clusterShutdowns, shutdownsLoaded, shutdownsError] = useK8sWatchResource<ClusterShutdownResource[]>({
    groupVersionKind: {
      group: ClusterShutdownModel.apiGroup,
      version: ClusterShutdownModel.apiVersion,
      kind: ClusterShutdownModel.kind,
    },
    isList: true,
    namespaced: true,
  });

  const [clusterRestarts, restartsLoaded, restartsError] = useK8sWatchResource<ClusterRestartResource[]>({
    groupVersionKind: {
      group: ClusterRestartModel.apiGroup,
      version: ClusterRestartModel.apiVersion,
      kind: ClusterRestartModel.kind,
    },
    isList: true,
    namespaced: true,
  });

  // Sort backups by creation timestamp (newest first) and pick the latest
  const sortedBackups = [...(etcdBackups || [])].sort((a, b) => {
    const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
    const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
    return bTime - aTime;
  });
  const lastBackup = sortedBackups.length > 0 ? sortedBackups[0] : undefined;

  // Find the active schedule
  const activeSchedule = (backupSchedules || []).find(
    (s) => s.status?.phase === 'Active',
  ) || (backupSchedules || [])[0];

  // Find latest cluster shutdown with cert expiry info
  const sortedShutdowns = [...(clusterShutdowns || [])].sort((a, b) => {
    const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
    const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
    return bTime - aTime;
  });
  const latestShutdown = sortedShutdowns.length > 0 ? sortedShutdowns[0] : undefined;

  // Recent operations: combine shutdowns and restarts, sort by creation time
  const recentOperations: Array<{
    name: string;
    kind: string;
    phase: string | undefined;
    createdAt: string | undefined;
  }> = [
    ...(clusterShutdowns || []).map((s) => ({
      name: s.metadata?.name || '-',
      kind: 'ClusterShutdown',
      phase: s.status?.phase,
      createdAt: s.metadata?.creationTimestamp,
    })),
    ...(clusterRestarts || []).map((r) => ({
      name: r.metadata?.name || '-',
      kind: 'ClusterRestart',
      phase: r.status?.phase,
      createdAt: r.metadata?.creationTimestamp,
    })),
  ].sort((a, b) => {
    const aTime = new Date(a.createdAt || 0).getTime();
    const bTime = new Date(b.createdAt || 0).getTime();
    return bTime - aTime;
  }).slice(0, 10);

  const isLoading = !etcdBackupsLoaded || !schedulesLoaded || !shutdownsLoaded || !restartsLoaded;
  const hasError = etcdBackupsError || schedulesError || shutdownsError || restartsError;

  return (
    <>
      <PageSection variant="light">
        <Title headingLevel="h1">Cluster Lifecycle Overview</Title>
      </PageSection>
      <PageSection>
        {isLoading && (
          <EmptyState>
            <Spinner size="xl" />
            <EmptyStateBody>Loading cluster lifecycle data...</EmptyStateBody>
          </EmptyState>
        )}
        {hasError && (
          <EmptyState>
            <EmptyStateBody>
              Error loading resources. Ensure the POP operator CRDs are installed.
            </EmptyStateBody>
          </EmptyState>
        )}
        {!isLoading && !hasError && (
          <Grid hasGutter>
            <GridItem span={6}>
              <Card>
                <CardTitle>Last Backup</CardTitle>
                <CardBody>
                  {lastBackup ? (
                    <DescriptionList>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Name</DescriptionListTerm>
                        <DescriptionListDescription>
                          {lastBackup.metadata?.name || '-'}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Status</DescriptionListTerm>
                        <DescriptionListDescription>
                          <PhaseLabel phase={lastBackup.status?.phase} />
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Node</DescriptionListTerm>
                        <DescriptionListDescription>
                          {lastBackup.status?.node || '-'}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Completed</DescriptionListTerm>
                        <DescriptionListDescription>
                          {formatTimestamp(
                            lastBackup.status?.completionTimestamp ||
                              lastBackup.metadata?.creationTimestamp,
                          )}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    </DescriptionList>
                  ) : (
                    <EmptyStateBody>No backups found.</EmptyStateBody>
                  )}
                </CardBody>
              </Card>
            </GridItem>

            <GridItem span={6}>
              <Card>
                <CardTitle>Schedule</CardTitle>
                <CardBody>
                  {activeSchedule ? (
                    <DescriptionList>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Name</DescriptionListTerm>
                        <DescriptionListDescription>
                          {activeSchedule.metadata?.name || '-'}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Status</DescriptionListTerm>
                        <DescriptionListDescription>
                          <PhaseLabel phase={activeSchedule.status?.phase} />
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Cron Schedule</DescriptionListTerm>
                        <DescriptionListDescription>
                          {activeSchedule.spec?.schedule || '-'}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Next Run</DescriptionListTerm>
                        <DescriptionListDescription>
                          {activeSchedule.status?.nextBackupTime
                            ? formatAbsoluteTimestamp(activeSchedule.status.nextBackupTime)
                            : '-'}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    </DescriptionList>
                  ) : (
                    <EmptyStateBody>No backup schedule configured.</EmptyStateBody>
                  )}
                </CardBody>
              </Card>
            </GridItem>

            <GridItem span={6}>
              <Card>
                <CardTitle>Certificate Expiry</CardTitle>
                <CardBody>
                  {latestShutdown?.status?.certificateExpiry ? (
                    <DescriptionList>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Source</DescriptionListTerm>
                        <DescriptionListDescription>
                          {latestShutdown.metadata?.name || '-'}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Expiry</DescriptionListTerm>
                        <DescriptionListDescription>
                          {formatAbsoluteTimestamp(latestShutdown.status.certificateExpiry)}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Time Remaining</DescriptionListTerm>
                        <DescriptionListDescription>
                          {formatTimestamp(latestShutdown.status.certificateExpiry)}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    </DescriptionList>
                  ) : (
                    <EmptyStateBody>
                      No certificate expiry information available.
                    </EmptyStateBody>
                  )}
                </CardBody>
              </Card>
            </GridItem>

            <GridItem span={6}>
              <Card>
                <CardTitle>Recent Operations</CardTitle>
                <CardBody>
                  {recentOperations.length > 0 ? (
                    <List isPlain>
                      {recentOperations.map((op) => (
                        <ListItem key={`${op.kind}-${op.name}`}>
                          <strong>{op.kind}:</strong> {op.name}{' '}
                          <PhaseLabel phase={op.phase} />{' '}
                          <span>{formatTimestamp(op.createdAt)}</span>
                        </ListItem>
                      ))}
                    </List>
                  ) : (
                    <EmptyStateBody>No recent operations.</EmptyStateBody>
                  )}
                </CardBody>
              </Card>
            </GridItem>
          </Grid>
        )}
      </PageSection>
    </>
  );
};

export default DashboardPage;
