import { POP_NAMESPACE } from '../constants';
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
  ClusterShutdownModel,
} from '../models';
import {
  ClusterLifecycleConfigResource,
  ClusterShutdownResource,
} from '../types';
import ShutdownSection from './ShutdownSection';

const NAMESPACE = POP_NAMESPACE;

const ShutdownPage: React.FC = () => {
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

  if (!configLoaded) {
    return (
      <>
        <PageSection variant="default">
          <Title headingLevel="h1">Cluster Shutdown</Title>
        </PageSection>
        <PageSection>
          <EmptyState>
            <Spinner size="xl" />
            <EmptyStateBody>Loading shutdown data...</EmptyStateBody>
          </EmptyState>
        </PageSection>
      </>
    );
  }

  const hasError = configError || shutdownsError;

  return (
    <>
      <PageSection variant="default">
        <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
          <FlexItem>
            <Title headingLevel="h1">Cluster Shutdown</Title>
          </FlexItem>
          <FlexItem>
            <Button variant="link" component="a" href="/backup-and-restore/shutdowns">
              View All Shutdowns
            </Button>
          </FlexItem>
        </Flex>
      </PageSection>

      {hasError && (
        <PageSection>
          <Alert variant="warning" isInline title="Some resources could not be loaded">
            {configError && <p>Config: {String(configError)}</p>}
            {shutdownsError && <p>Shutdowns: {String(shutdownsError)}</p>}
          </Alert>
        </PageSection>
      )}

      <PageSection>
        <ShutdownSection
          config={config}
          shutdowns={clusterShutdowns || []}
          shutdownsLoaded={shutdownsLoaded}
          namespace={NAMESPACE}
        />
      </PageSection>
    </>
  );
};

export default ShutdownPage;
