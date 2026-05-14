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
import { ClusterLifecycleConfigModel } from '../models';
import { ClusterLifecycleConfigResource } from '../types';
import SettingsSection from './SettingsSection';

const NAMESPACE = 'pop-system';

const SettingsPage: React.FC = () => {
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

  if (!configLoaded) {
    return (
      <>
        <PageSection variant="light">
          <Title headingLevel="h1">Settings</Title>
        </PageSection>
        <PageSection>
          <EmptyState>
            <Spinner size="xl" />
            <EmptyStateBody>Loading configuration...</EmptyStateBody>
          </EmptyState>
        </PageSection>
      </>
    );
  }

  return (
    <>
      <PageSection variant="light">
        <Title headingLevel="h1">Settings</Title>
      </PageSection>

      {configError && (
        <PageSection>
          <Alert variant="warning" isInline title="Configuration could not be loaded">
            <p>{String(configError)}</p>
          </Alert>
        </PageSection>
      )}

      <PageSection>
        <SettingsSection config={config} namespace={NAMESPACE} />
      </PageSection>
    </>
  );
};

export default SettingsPage;
