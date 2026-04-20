import React from 'react';
import {
  Card,
  CardTitle,
  CardBody,
  Button,
  Flex,
  FlexItem,
  Label,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  EmptyState,
  EmptyStateBody,
  Alert,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
} from '@patternfly/react-icons';
import { k8sCreate } from '@openshift-console/dynamic-plugin-sdk';
import {
  ClusterLifecycleConfigResource,
  PowerEventResource,
} from '../types';
import { PowerEventModel } from '../models';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';

interface PowerSectionProps {
  config: ClusterLifecycleConfigResource | undefined;
  powerEvents: PowerEventResource[];
  powerEventsLoaded: boolean;
  namespace: string;
}

const PowerSection: React.FC<PowerSectionProps> = ({
  config,
  powerEvents,
  powerEventsLoaded,
  namespace,
}) => {
  const [testError, setTestError] = React.useState<string | undefined>(undefined);
  const [isTesting, setIsTesting] = React.useState(false);

  const powerConfig = config?.spec?.powerProtection;
  const powerStatus = config?.status?.power;
  const isEnabled = powerConfig?.enabled ?? false;
  const currentStatus = powerStatus?.status || 'Unknown';
  const isNormal = currentStatus === 'Normal';

  const sortedEvents = [...(powerEvents || [])]
    .sort((a, b) => {
      const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
      const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
      return bTime - aTime;
    })
    .slice(0, 5);

  const handleTestAlert = () => {
    setIsTesting(true);
    setTestError(undefined);

    const eventName = `test-alert-${Date.now()}`;

    k8sCreate({
      model: PowerEventModel,
      data: {
        apiVersion: 'pop.openshift.io/v1alpha1',
        kind: 'PowerEvent',
        metadata: {
          name: eventName,
          namespace,
        },
        spec: {
          eventType: 'Test',
          triggeredBy: 'manual',
          message: 'Test alert from console dashboard',
        },
      },
    })
      .then(() => {
        setIsTesting(false);
      })
      .catch((err: Error) => {
        setIsTesting(false);
        setTestError(err.message || 'Failed to create test alert');
      });
  };

  return (
    <Card>
      <CardTitle>
        <Flex
          alignItems={{ default: 'alignItemsCenter' }}
          spaceItems={{ default: 'spaceItemsMd' }}
        >
          <FlexItem>
            <Title headingLevel="h2">Power Protection</Title>
          </FlexItem>
          <FlexItem>
            <Label
              color={isEnabled ? 'green' : 'grey'}
              icon={isEnabled ? <CheckCircleIcon /> : undefined}
            >
              {isEnabled ? 'Enabled' : 'Disabled'}
            </Label>
          </FlexItem>
        </Flex>
      </CardTitle>
      <CardBody>
        <Flex spaceItems={{ default: 'spaceItemsLg' }} style={{ marginBottom: 16 }}>
          <FlexItem>
            <DescriptionList isCompact isHorizontal>
              <DescriptionListGroup>
                <DescriptionListTerm>Status</DescriptionListTerm>
                <DescriptionListDescription>
                  <Label
                    color={isNormal ? 'green' : 'red'}
                    icon={isNormal ? <CheckCircleIcon /> : <ExclamationCircleIcon />}
                  >
                    {currentStatus}
                  </Label>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Priority</DescriptionListTerm>
                <DescriptionListDescription>
                  {powerConfig?.clusterPriority || 'Standard'}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Battery</DescriptionListTerm>
                <DescriptionListDescription>
                  {powerConfig?.batteryCapacityMinutes ?? 15} min
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Autonomous</DescriptionListTerm>
                <DescriptionListDescription>
                  {powerConfig?.autonomousShutdown ? 'Yes' : 'No'}
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </FlexItem>
          <FlexItem>
            <Button
              variant="secondary"
              onClick={handleTestAlert}
              isLoading={isTesting}
              isDisabled={isTesting || !isEnabled}
            >
              Test Alert
            </Button>
          </FlexItem>
        </Flex>

        {testError && (
          <Alert
            variant="danger"
            isInline
            title="Test alert failed"
            style={{ marginBottom: 16 }}
          >
            {testError}
          </Alert>
        )}

        <Title headingLevel="h3" style={{ marginBottom: 8 }}>
          Recent power events
        </Title>

        {!powerEventsLoaded ? (
          <EmptyState>
            <EmptyStateBody>Loading power events...</EmptyStateBody>
          </EmptyState>
        ) : sortedEvents.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No power events recorded.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="Recent power events" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Type</Th>
                <Th>Phase</Th>
                <Th>Source</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {sortedEvents.map((event) => (
                <Tr key={event.metadata?.uid || event.metadata?.name}>
                  <Td dataLabel="Name">{event.metadata?.name || '-'}</Td>
                  <Td dataLabel="Type">{event.spec?.eventType || '-'}</Td>
                  <Td dataLabel="Phase">
                    <PhaseLabel phase={event.status?.phase} />
                  </Td>
                  <Td dataLabel="Source">{event.spec?.source || '-'}</Td>
                  <Td dataLabel="Age">
                    {formatTimestamp(event.metadata?.creationTimestamp)}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </CardBody>
    </Card>
  );
};

export default PowerSection;
