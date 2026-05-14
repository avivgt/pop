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
  PowerEventModel,
} from '../models';
import {
  ClusterLifecycleConfigResource,
  PowerEventResource,
} from '../types';
import PowerSection from './PowerSection';

const NAMESPACE = 'pop-system';

const PowerPage: React.FC = () => {
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

  if (!configLoaded) {
    return (
      <>
        <PageSection variant="light">
          <Title headingLevel="h1">Power Protection</Title>
        </PageSection>
        <PageSection>
          <EmptyState>
            <Spinner size="xl" />
            <EmptyStateBody>Loading power protection data...</EmptyStateBody>
          </EmptyState>
        </PageSection>
      </>
    );
  }

  const hasError = configError || powerEventsError;

  return (
    <>
      <PageSection variant="light">
        <Title headingLevel="h1">Power Protection</Title>
      </PageSection>

      {hasError && (
        <PageSection>
          <Alert variant="warning" isInline title="Some resources could not be loaded">
            {configError && <p>Config: {String(configError)}</p>}
            {powerEventsError && <p>Power Events: {String(powerEventsError)}</p>}
          </Alert>
        </PageSection>
      )}

      <PageSection>
        <PowerSection
          config={config}
          powerEvents={powerEvents || []}
          powerEventsLoaded={powerEventsLoaded}
          namespace={NAMESPACE}
        />
      </PageSection>
    </>
  );
};

export default PowerPage;
