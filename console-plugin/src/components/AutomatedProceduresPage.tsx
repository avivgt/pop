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
  AutomatedProceduresConfigModel,
  PowerEventModel,
} from '../models';
import {
  AutomatedProceduresConfigResource,
  PowerEventResource,
} from '../types';
import AutomatedProceduresSection from './AutomatedProceduresSection';

const NAMESPACE = 'pop-system';

const AutomatedProceduresPage: React.FC = () => {
  const [apConfig, apConfigLoaded, apConfigError] = useK8sWatchResource<AutomatedProceduresConfigResource>({
    groupVersionKind: {
      group: AutomatedProceduresConfigModel.apiGroup,
      version: AutomatedProceduresConfigModel.apiVersion,
      kind: AutomatedProceduresConfigModel.kind,
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

  if (!apConfigLoaded) {
    return (
      <>
        <PageSection variant="light">
          <Title headingLevel="h1">Automated Procedures</Title>
        </PageSection>
        <PageSection>
          <EmptyState>
            <Spinner size="xl" />
            <EmptyStateBody>Loading automated procedures data...</EmptyStateBody>
          </EmptyState>
        </PageSection>
      </>
    );
  }

  const hasError = apConfigError || powerEventsError;

  return (
    <>
      <PageSection variant="light">
        <Title headingLevel="h1">Automated Procedures</Title>
      </PageSection>

      {hasError && (
        <PageSection>
          <Alert variant="warning" isInline title="Some resources could not be loaded">
            {apConfigError && <p>Config: {String(apConfigError)}</p>}
            {powerEventsError && <p>Power Events: {String(powerEventsError)}</p>}
          </Alert>
        </PageSection>
      )}

      <PageSection>
        <AutomatedProceduresSection
          apConfig={apConfig}
          powerEvents={powerEvents || []}
          powerEventsLoaded={powerEventsLoaded}
          namespace={NAMESPACE}
        />
      </PageSection>
    </>
  );
};

export default AutomatedProceduresPage;
