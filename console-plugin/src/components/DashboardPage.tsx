import React from 'react';
import {
  PageSection,
  Title,
  EmptyState,
  EmptyStateBody,
  Spinner,
  Alert,
  Card,
  CardTitle,
  CardBody,
  CardFooter,
  Grid,
  GridItem,
  Button,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
} from '@patternfly/react-core';
import { useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import {
  ClusterLifecycleConfigModel,
  EtcdBackupModel,
  ClusterShutdownModel,
  PowerEventModel,
} from '../models';
import {
  ClusterLifecycleConfigResource,
  EtcdBackupResource,
  ClusterShutdownResource,
  PowerEventResource,
} from '../types';
import StatusBar from './StatusBar';
import PhaseLabel from './shared/PhaseLabel';
import { formatTimestamp } from '../utils';

const NAMESPACE = 'pop-system';

const DashboardPage: React.FC = () => {
  const [config, configLoaded, configError] = useK8sWatchResource<ClusterLifecycleConfigResource>({
    groupVersionKind: {
      group: ClusterLifecycleConfigModel.apiGroup,
      version: ClusterLifecycleConfigModel.apiVersion,
      kind: ClusterLifecycleConfigModel.kind,
    },
    namespace: NAMESPACE,
    name: 'default',
    namespaced: true,
  });

  const [etcdBackups, , etcdBackupsError] = useK8sWatchResource<
    EtcdBackupResource[]
  >({
    groupVersionKind: {
      group: EtcdBackupModel.apiGroup,
      version: EtcdBackupModel.apiVersion,
      kind: EtcdBackupModel.kind,
    },
    namespace: NAMESPACE,
    isList: true,
    namespaced: true,
  });

  const [clusterShutdowns, , shutdownsError] = useK8sWatchResource<
    ClusterShutdownResource[]
  >({
    groupVersionKind: {
      group: ClusterShutdownModel.apiGroup,
      version: ClusterShutdownModel.apiVersion,
      kind: ClusterShutdownModel.kind,
    },
    namespace: NAMESPACE,
    isList: true,
    namespaced: true,
  });

  const [powerEvents, , powerEventsError] = useK8sWatchResource<
    PowerEventResource[]
  >({
    groupVersionKind: {
      group: PowerEventModel.apiGroup,
      version: PowerEventModel.apiVersion,
      kind: PowerEventModel.kind,
    },
    namespace: NAMESPACE,
    isList: true,
    namespaced: true,
  });

  if (!configLoaded) {
    return (
      <>
        <PageSection variant="light">
          <Title headingLevel="h1">POP - Power Outage Provisioner</Title>
        </PageSection>
        <PageSection>
          <EmptyState>
            <Spinner size="xl" />
            <EmptyStateBody>Loading cluster lifecycle data...</EmptyStateBody>
          </EmptyState>
        </PageSection>
      </>
    );
  }

  const hasError = configError || etcdBackupsError || shutdownsError || powerEventsError;

  const sortedBackups = [...(etcdBackups || [])].sort(
    (a, b) =>
      new Date(b.metadata?.creationTimestamp || 0).getTime() -
      new Date(a.metadata?.creationTimestamp || 0).getTime(),
  );
  const latestBackup = sortedBackups[0];

  const sortedShutdowns = [...(clusterShutdowns || [])].sort(
    (a, b) =>
      new Date(b.metadata?.creationTimestamp || 0).getTime() -
      new Date(a.metadata?.creationTimestamp || 0).getTime(),
  );
  const activeShutdown = sortedShutdowns.find(
    (s) => s.status?.phase && !['Complete', 'Failed'].includes(s.status.phase),
  );
  const latestShutdown = activeShutdown || sortedShutdowns[0];

  const powerStatus = config?.status?.power?.status || 'Normal';
  const recentEvents = (powerEvents || []).length;

  const scheduleEnabled = config?.spec?.backup?.schedule?.enabled;

  return (
    <>
      <PageSection variant="light">
        <Title headingLevel="h1">POP - Power Outage Provisioner</Title>
      </PageSection>

      {hasError && (
        <PageSection>
          <Alert variant="warning" isInline title="Some resources could not be loaded">
            {configError && <p>Config: {String(configError)}</p>}
            {etcdBackupsError && <p>Backups: {String(etcdBackupsError)}</p>}
            {shutdownsError && <p>Shutdowns: {String(shutdownsError)}</p>}
            {powerEventsError && <p>Power Events: {String(powerEventsError)}</p>}
          </Alert>
        </PageSection>
      )}

      <PageSection>
        <StatusBar config={config} loaded={configLoaded} />
      </PageSection>

      <PageSection>
        <Grid hasGutter>
          <GridItem md={6}>
            <Card isFullHeight>
              <CardTitle>etcd Backup</CardTitle>
              <CardBody>
                <DescriptionList isCompact isHorizontal>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Total Backups</DescriptionListTerm>
                    <DescriptionListDescription>
                      {(etcdBackups || []).length}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Latest</DescriptionListTerm>
                    <DescriptionListDescription>
                      {latestBackup ? (
                        <>
                          <PhaseLabel phase={latestBackup.status?.phase} />{' '}
                          {formatTimestamp(latestBackup.metadata?.creationTimestamp)}
                        </>
                      ) : (
                        'No backups yet'
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Schedule</DescriptionListTerm>
                    <DescriptionListDescription>
                      {scheduleEnabled ? 'Enabled' : 'Disabled'}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </CardBody>
              <CardFooter>
                <Button variant="link" component="a" href="/backup-and-restore/backup" isInline>
                  Manage Backups
                </Button>
              </CardFooter>
            </Card>
          </GridItem>

          <GridItem md={6}>
            <Card isFullHeight>
              <CardTitle>Cluster Shutdown</CardTitle>
              <CardBody>
                <DescriptionList isCompact isHorizontal>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Total Shutdowns</DescriptionListTerm>
                    <DescriptionListDescription>
                      {(clusterShutdowns || []).length}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Status</DescriptionListTerm>
                    <DescriptionListDescription>
                      {latestShutdown ? (
                        <>
                          <PhaseLabel phase={latestShutdown.status?.phase} />{' '}
                          {latestShutdown.spec?.dryRun ? '(dry-run)' : ''}{' '}
                          {formatTimestamp(latestShutdown.metadata?.creationTimestamp)}
                        </>
                      ) : (
                        'No shutdowns'
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </CardBody>
              <CardFooter>
                <Button variant="link" component="a" href="/backup-and-restore/shutdown" isInline>
                  Manage Shutdowns
                </Button>
              </CardFooter>
            </Card>
          </GridItem>

          <GridItem md={6}>
            <Card isFullHeight>
              <CardTitle>Power Protection</CardTitle>
              <CardBody>
                <DescriptionList isCompact isHorizontal>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Power Status</DescriptionListTerm>
                    <DescriptionListDescription>
                      <PhaseLabel phase={powerStatus === 'Normal' ? 'Complete' : 'Failed'} />
                      {' '}{powerStatus}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Power Events</DescriptionListTerm>
                    <DescriptionListDescription>
                      {recentEvents} recorded
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </CardBody>
              <CardFooter>
                <Button variant="link" component="a" href="/backup-and-restore/power" isInline>
                  Power Protection
                </Button>
              </CardFooter>
            </Card>
          </GridItem>

          <GridItem md={6}>
            <Card isFullHeight>
              <CardTitle>Settings</CardTitle>
              <CardBody>
                <DescriptionList isCompact isHorizontal>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Configuration</DescriptionListTerm>
                    <DescriptionListDescription>
                      {config ? config.metadata?.name : 'Not configured'}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Notifications</DescriptionListTerm>
                    <DescriptionListDescription>
                      {config?.spec?.notification?.webhook?.enabled ? 'Enabled' : 'Disabled'}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </CardBody>
              <CardFooter>
                <Button variant="link" component="a" href="/backup-and-restore/settings" isInline>
                  Configure
                </Button>
              </CardFooter>
            </Card>
          </GridItem>
        </Grid>
      </PageSection>
    </>
  );
};

export default DashboardPage;
