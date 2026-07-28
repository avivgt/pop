import { POP_NAMESPACE } from '../constants';
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
  ClusterShutdownModel,
  AutomatedProceduresConfigModel,
} from '../models';
import {
  ClusterLifecycleConfigResource,
  ClusterShutdownResource,
  AutomatedProceduresConfigResource,
} from '../types';
import StatusBar from './StatusBar';
import PhaseLabel from './shared/PhaseLabel';
import { formatTimestamp } from '../utils';

const NAMESPACE = POP_NAMESPACE;

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

  const [apConfig, , apConfigError] = useK8sWatchResource<AutomatedProceduresConfigResource>({
    groupVersionKind: {
      group: AutomatedProceduresConfigModel.apiGroup,
      version: AutomatedProceduresConfigModel.apiVersion,
      kind: AutomatedProceduresConfigModel.kind,
    },
    namespace: NAMESPACE,
    name: 'default',
    namespaced: true,
  });

  if (!configLoaded) {
    return (
      <>
        <PageSection variant="default">
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

  const hasError = configError || shutdownsError || apConfigError;

  const backupStatus = config?.status?.backup;
  const scheduleEnabled = config?.spec?.backup?.schedule?.enabled;

  const sortedShutdowns = [...(clusterShutdowns || [])].sort(
    (a, b) =>
      new Date(b.metadata?.creationTimestamp || 0).getTime() -
      new Date(a.metadata?.creationTimestamp || 0).getTime(),
  );
  const activeShutdown = sortedShutdowns.find(
    (s) => s.status?.phase && !['Complete', 'Failed'].includes(s.status.phase),
  );
  const latestShutdown = activeShutdown || sortedShutdowns[0];

  const powerStatus = apConfig?.status?.powerStatus || 'Unknown';

  return (
    <>
      <PageSection variant="default">
        <Title headingLevel="h1">POP - Power Outage Provisioner</Title>
      </PageSection>

      {hasError && (
        <PageSection>
          <Alert variant="warning" isInline title="Some resources could not be loaded">
            {configError && <p>Config: {String(configError)}</p>}
            {shutdownsError && <p>Shutdowns: {String(shutdownsError)}</p>}
            {apConfigError && <p>Automated Procedures: {String(apConfigError)}</p>}
          </Alert>
        </PageSection>
      )}

      <PageSection>
        <StatusBar config={config} apConfig={apConfig} loaded={configLoaded} />
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
                      {backupStatus?.totalBackups ?? 0}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Latest</DescriptionListTerm>
                    <DescriptionListDescription>
                      {backupStatus?.lastBackupTime
                        ? formatTimestamp(backupStatus.lastBackupTime)
                        : 'No backups yet'}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Schedule</DescriptionListTerm>
                    <DescriptionListDescription>
                      {scheduleEnabled ? backupStatus?.resolvedSchedule || 'Enabled' : 'Disabled'}
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
              <CardTitle>Automated Procedures</CardTitle>
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
                    <DescriptionListTerm>Enabled</DescriptionListTerm>
                    <DescriptionListDescription>
                      {apConfig?.spec?.enabled ? 'Yes' : 'No'}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </CardBody>
              <CardFooter>
                <Button variant="link" component="a" href="/backup-and-restore/automated-procedures" isInline>
                  Automated Procedures
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
