import React from 'react';
import {
  PageSection,
  Title,
  EmptyState,
  EmptyStateBody,
  Spinner,
  Alert,
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
import BackupSection from './BackupSection';
import ShutdownSection from './ShutdownSection';
import PowerSection from './PowerSection';
import SettingsSection from './SettingsSection';

const NAMESPACE = 'pop-system';

const DashboardPage: React.FC = () => {
  // Watch the singleton ClusterLifecycleConfig
  const [configList, configLoaded, configError] = useK8sWatchResource<
    ClusterLifecycleConfigResource[]
  >({
    groupVersionKind: {
      group: ClusterLifecycleConfigModel.apiGroup,
      version: ClusterLifecycleConfigModel.apiVersion,
      kind: ClusterLifecycleConfigModel.kind,
    },
    namespace: NAMESPACE,
    isList: true,
    namespaced: true,
  });

  // Watch EtcdBackups
  const [etcdBackups, etcdBackupsLoaded, etcdBackupsError] = useK8sWatchResource<
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

  // Watch ClusterShutdowns
  const [clusterShutdowns, shutdownsLoaded, shutdownsError] = useK8sWatchResource<
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

  // Watch PowerEvents
  const [powerEvents, powerEventsLoaded, powerEventsError] = useK8sWatchResource<
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

  // Find the "default" config or take the first one
  const config: ClusterLifecycleConfigResource | undefined =
    (configList || []).find((c) => c.metadata?.name === 'default') ||
    (configList || [])[0];

  const isLoading = !configLoaded;
  const hasError = configError || etcdBackupsError || shutdownsError || powerEventsError;

  if (isLoading) {
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
        <BackupSection
          config={config}
          backups={etcdBackups || []}
          backupsLoaded={etcdBackupsLoaded}
          namespace={NAMESPACE}
        />
      </PageSection>

      <PageSection>
        <ShutdownSection
          config={config}
          shutdowns={clusterShutdowns || []}
          shutdownsLoaded={shutdownsLoaded}
          namespace={NAMESPACE}
        />
      </PageSection>

      <PageSection>
        <PowerSection
          config={config}
          powerEvents={powerEvents || []}
          powerEventsLoaded={powerEventsLoaded}
          namespace={NAMESPACE}
        />
      </PageSection>

      <PageSection>
        <SettingsSection config={config} namespace={NAMESPACE} />
      </PageSection>
    </>
  );
};

export default DashboardPage;
