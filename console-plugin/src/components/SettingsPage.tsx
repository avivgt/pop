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
import { ClusterLifecycleConfigModel, AutomatedProceduresConfigModel } from '../models';
import { ClusterLifecycleConfigResource, AutomatedProceduresConfigResource } from '../types';
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

  const hasError = configError || apConfigError;

  return (
    <>
      <PageSection variant="light">
        <Title headingLevel="h1">Settings</Title>
      </PageSection>

      {hasError && (
        <PageSection>
          <Alert variant="warning" isInline title="Some configuration could not be loaded">
            {configError && <p>Lifecycle Config: {String(configError)}</p>}
            {apConfigError && <p>Automated Procedures: {String(apConfigError)}</p>}
          </Alert>
        </PageSection>
      )}

      <PageSection>
        <SettingsSection config={config} apConfig={apConfig} namespace={NAMESPACE} />
      </PageSection>
    </>
  );
};

export default SettingsPage;
