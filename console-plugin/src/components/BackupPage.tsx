import React from 'react';
import {
  PageSection,
  Title,
  EmptyState,
  EmptyStateBody,
  Spinner,
  Alert,
  Button,
  Flex,
  FlexItem,
} from '@patternfly/react-core';
import { useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import {
  ClusterLifecycleConfigModel,
  EtcdBackupModel,
} from '../models';
import {
  ClusterLifecycleConfigResource,
  EtcdBackupResource,
} from '../types';
import BackupSection from './BackupSection';

const NAMESPACE = 'pop-system';

const BackupPage: React.FC = () => {
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

  if (!configLoaded) {
    return (
      <>
        <PageSection variant="light">
          <Title headingLevel="h1">etcd Backup</Title>
        </PageSection>
        <PageSection>
          <EmptyState>
            <Spinner size="xl" />
            <EmptyStateBody>Loading backup data...</EmptyStateBody>
          </EmptyState>
        </PageSection>
      </>
    );
  }

  const hasError = configError || etcdBackupsError;

  return (
    <>
      <PageSection variant="light">
        <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
          <FlexItem>
            <Title headingLevel="h1">etcd Backup</Title>
          </FlexItem>
          <FlexItem>
            <Button variant="link" component="a" href="/backup-and-restore/backups">
              View All Backups
            </Button>
          </FlexItem>
        </Flex>
      </PageSection>

      {hasError && (
        <PageSection>
          <Alert variant="warning" isInline title="Some resources could not be loaded">
            {configError && <p>Config: {String(configError)}</p>}
            {etcdBackupsError && <p>Backups: {String(etcdBackupsError)}</p>}
          </Alert>
        </PageSection>
      )}

      <PageSection>
        <BackupSection
          config={config}
          backups={etcdBackups || []}
          backupsLoaded={etcdBackupsLoaded}
          namespace={NAMESPACE}
        />
      </PageSection>
    </>
  );
};

export default BackupPage;
