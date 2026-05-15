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
  TextInput,
  ExpandableSection,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  PluggedIcon,
  UnpluggedIcon,
} from '@patternfly/react-icons';
import { k8sCreate, k8sPatch } from '@openshift-console/dynamic-plugin-sdk';
import {
  AutomatedProceduresConfigResource,
  PowerEventResource,
} from '../types';
import { AutomatedProceduresConfigModel, PowerEventModel } from '../models';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';

interface AutomatedProceduresSectionProps {
  apConfig: AutomatedProceduresConfigResource | undefined;
  powerEvents: PowerEventResource[];
  powerEventsLoaded: boolean;
  namespace: string;
}

const AutomatedProceduresSection: React.FC<AutomatedProceduresSectionProps> = ({
  apConfig,
  powerEvents,
  powerEventsLoaded,
  namespace,
}) => {
  const [testError, setTestError] = React.useState<string | undefined>(undefined);
  const [isTesting, setIsTesting] = React.useState(false);
  const [batteryMinutes, setBatteryMinutes] = React.useState<number | undefined>(undefined);
  const [batterySaving, setBatterySaving] = React.useState(false);
  const [sourcesExpanded, setSourcesExpanded] = React.useState(false);

  const spec = apConfig?.spec;
  const status = apConfig?.status;
  const currentBattery = spec?.batteryCapacityMinutes ?? 15;

  React.useEffect(() => {
    setBatteryMinutes(currentBattery);
  }, [currentBattery]);

  const isEnabled = spec?.enabled ?? false;
  const currentStatus = status?.powerStatus || 'Unknown';
  const isNormal = currentStatus === 'Normal';

  const sortedEvents = [...(powerEvents || [])]
    .sort((a, b) => {
      const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
      const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
      return bTime - aTime;
    })
    .slice(0, 5);

  const batteryChanged = batteryMinutes !== currentBattery;

  const handleSaveBattery = () => {
    if (!apConfig || batteryMinutes == null) return;
    setBatterySaving(true);
    k8sPatch({
      model: AutomatedProceduresConfigModel,
      resource: apConfig,
      data: [
        {
          op: 'replace',
          path: '/spec/batteryCapacityMinutes',
          value: batteryMinutes,
        },
      ],
    })
      .then(() => setBatterySaving(false))
      .catch(() => setBatterySaving(false));
  };

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
      .then(() => setIsTesting(false))
      .catch((err: Error) => {
        setIsTesting(false);
        setTestError(err.message || 'Failed to create test alert');
      });
  };

  const eventSources = status?.eventSources || [];

  return (
    <Card>
      <CardTitle>
        <Flex
          alignItems={{ default: 'alignItemsCenter' }}
          spaceItems={{ default: 'spaceItemsMd' }}
        >
          <FlexItem>
            <Title headingLevel="h2">Automated Procedures</Title>
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
        <div style={{ marginBottom: 16, color: '#6a6e73', fontSize: '0.9rem', lineHeight: 1.6 }}>
          Automated event response for power outages and infrastructure events. Event sources
          (webhook, SNMP, IPMI/Redfish, NUT) detect power failures and trigger etcd backup,
          wait for a confirmation period, then gracefully shut down the cluster if power is
          not restored.
        </div>

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
                  {spec?.clusterPriority || 'Standard'}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Battery (min)</DescriptionListTerm>
                <DescriptionListDescription>
                  <Flex alignItems={{ default: 'alignItemsCenter' }} spaceItems={{ default: 'spaceItemsSm' }}>
                    <FlexItem>
                      <TextInput
                        type="number"
                        value={String(batteryMinutes ?? '')}
                        onChange={(_e, val) => setBatteryMinutes(val === '' ? undefined : Number(val))}
                        style={{ width: 80 }}
                        min={1}
                        aria-label="Battery capacity in minutes"
                      />
                    </FlexItem>
                    {batteryChanged && (
                      <FlexItem>
                        <Button
                          variant="primary"
                          size="sm"
                          onClick={handleSaveBattery}
                          isLoading={batterySaving}
                          isDisabled={batterySaving || batteryMinutes == null || batteryMinutes < 1}
                        >
                          Save
                        </Button>
                      </FlexItem>
                    )}
                  </Flex>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Autonomous</DescriptionListTerm>
                <DescriptionListDescription>
                  {spec?.autonomousShutdown ? 'Yes' : 'No'}
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

        {eventSources.length > 0 && (
          <ExpandableSection
            toggleText={`Event Sources (${eventSources.length})`}
            isExpanded={sourcesExpanded}
            onToggle={(_e, expanded) => setSourcesExpanded(expanded)}
            style={{ marginBottom: 16 }}
          >
            <DescriptionList isCompact isHorizontal>
              {eventSources.map((src) => (
                <DescriptionListGroup key={src.type}>
                  <DescriptionListTerm>{src.type.toUpperCase()}</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Flex alignItems={{ default: 'alignItemsCenter' }} spaceItems={{ default: 'spaceItemsSm' }}>
                      <FlexItem>
                        <Label
                          color={src.connected ? 'green' : 'red'}
                          icon={src.connected ? <PluggedIcon /> : <UnpluggedIcon />}
                        >
                          {src.connected ? 'Connected' : 'Disconnected'}
                        </Label>
                      </FlexItem>
                      {src.message && <FlexItem>{src.message}</FlexItem>}
                      {src.lastCheck && (
                        <FlexItem style={{ color: '#6a6e73', fontSize: '0.85rem' }}>
                          checked {formatTimestamp(src.lastCheck)}
                        </FlexItem>
                      )}
                    </Flex>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              ))}
            </DescriptionList>
          </ExpandableSection>
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

export default AutomatedProceduresSection;
