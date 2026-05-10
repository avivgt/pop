import React from 'react';
import {
  Card,
  CardTitle,
  CardBody,
  Button,
  EmptyState,
  EmptyStateBody,
  Title,
  Label,
  Split,
  SplitItem,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  ExpandableSection,
} from '@patternfly/react-core';
import {
  CheckCircleIcon,
  InProgressIcon,
  PendingIcon,
  ExclamationCircleIcon,
} from '@patternfly/react-icons';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  ClusterLifecycleConfigResource,
  ClusterShutdownResource,
} from '../types';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';
import ShutdownModal from './actions/ShutdownModal';

const SHUTDOWN_PHASES = [
  { key: 'Validating', label: 'Validate' },
  { key: 'Cordoning', label: 'Cordon' },
  { key: 'RunningHooks', label: 'Hooks' },
  { key: 'NotifyingApps', label: 'Notify' },
  { key: 'BackingUp', label: 'Backup' },
  { key: 'ShuttingDown', label: 'Shutdown' },
  { key: 'Complete', label: 'Done' },
];

interface ShutdownSectionProps {
  config: ClusterLifecycleConfigResource | undefined;
  shutdowns: ClusterShutdownResource[];
  shutdownsLoaded: boolean;
  namespace: string;
}

const ShutdownSection: React.FC<ShutdownSectionProps> = ({
  config,
  shutdowns,
  shutdownsLoaded,
  namespace,
}) => {
  const [isTestModalOpen, setIsTestModalOpen] = React.useState(false);
  const [isShutdownModalOpen, setIsShutdownModalOpen] = React.useState(false);
  const [nodesExpanded, setNodesExpanded] = React.useState(false);

  const pvcName = config?.spec?.backup?.storage?.pvcName || 'etcd-backup-pvc';

  const activeShutdown = (shutdowns || []).find(
    (s) =>
      s.status?.phase &&
      s.status.phase !== 'Complete' &&
      s.status.phase !== 'Failed',
  );

  const sortedShutdowns = [...(shutdowns || [])]
    .sort((a, b) => {
      const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
      const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
      return bTime - aTime;
    })
    .slice(0, 10);

  const currentPhaseIndex = activeShutdown
    ? SHUTDOWN_PHASES.findIndex((p) => p.key === activeShutdown.status?.phase)
    : -1;

  return (
    <Card>
      <CardTitle>
        <Title headingLevel="h2">Cluster Shutdown</Title>
      </CardTitle>
      <CardBody>
        <Split hasGutter style={{ marginBottom: 16 }}>
          <SplitItem>
            <Button variant="primary" onClick={() => setIsTestModalOpen(true)}>
              Test Shutdown
            </Button>
          </SplitItem>
          <SplitItem>
            <Button variant="danger" onClick={() => setIsShutdownModalOpen(true)}>
              Shutdown Cluster
            </Button>
          </SplitItem>
        </Split>

        {activeShutdown && (
          <Card isFlat style={{ marginBottom: 16, backgroundColor: '#f0f0f0' }}>
            <CardBody>
              <Title headingLevel="h3" style={{ marginBottom: 12 }}>
                {activeShutdown.spec?.dryRun ? 'Test Run' : 'Shutdown'} in
                progress: {activeShutdown.metadata?.name}
              </Title>

              <div
                style={{
                  display: 'flex',
                  gap: 4,
                  marginBottom: 16,
                  flexWrap: 'wrap',
                }}
              >
                {SHUTDOWN_PHASES.map((phase, idx) => {
                  let icon = <PendingIcon />;
                  let color: 'grey' | 'blue' | 'green' | 'red' = 'grey';

                  if (activeShutdown.status?.phase === 'Failed') {
                    if (idx < currentPhaseIndex) {
                      icon = <CheckCircleIcon />;
                      color = 'green';
                    } else if (idx === currentPhaseIndex) {
                      icon = <ExclamationCircleIcon />;
                      color = 'red';
                    }
                  } else if (idx < currentPhaseIndex) {
                    icon = <CheckCircleIcon />;
                    color = 'green';
                  } else if (idx === currentPhaseIndex) {
                    icon = <InProgressIcon />;
                    color = 'blue';
                  }

                  return (
                    <Label key={phase.key} color={color} icon={icon}>
                      {phase.label}
                    </Label>
                  );
                })}
              </div>

              <DescriptionList isCompact isHorizontal>
                <DescriptionListGroup>
                  <DescriptionListTerm>Status</DescriptionListTerm>
                  <DescriptionListDescription>
                    {activeShutdown.status?.message || activeShutdown.status?.phase}
                  </DescriptionListDescription>
                </DescriptionListGroup>
                {activeShutdown.status?.totalNodes != null && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Nodes</DescriptionListTerm>
                    <DescriptionListDescription>
                      Cordoned: {activeShutdown.status?.cordonedNodes ?? 0}/
                      {activeShutdown.status.totalNodes} | Shut down:{' '}
                      {activeShutdown.status?.shutdownNodes ?? 0}/
                      {activeShutdown.status.totalNodes}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}
                {activeShutdown.status?.etcdBackupRef && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Backup</DescriptionListTerm>
                    <DescriptionListDescription>
                      {activeShutdown.status.etcdBackupRef}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}
              </DescriptionList>

              {activeShutdown.status?.nodeStatuses &&
                activeShutdown.status.nodeStatuses.length > 0 && (
                  <ExpandableSection
                    toggleText={
                      nodesExpanded ? 'Hide node details' : 'Show node details'
                    }
                    isExpanded={nodesExpanded}
                    onToggle={(_event, expanded) => setNodesExpanded(expanded)}
                    style={{ marginTop: 12 }}
                  >
                    <Table aria-label="Node shutdown status" variant="compact">
                      <Thead>
                        <Tr>
                          <Th>Node</Th>
                          <Th>Role</Th>
                          <Th>Status</Th>
                          <Th>Message</Th>
                        </Tr>
                      </Thead>
                      <Tbody>
                        {activeShutdown.status.nodeStatuses.map((ns) => (
                          <Tr key={ns.name}>
                            <Td dataLabel="Node">{ns.name}</Td>
                            <Td dataLabel="Role">{ns.role}</Td>
                            <Td dataLabel="Status">
                              <PhaseLabel phase={ns.status} />
                            </Td>
                            <Td dataLabel="Message">{ns.message || '-'}</Td>
                          </Tr>
                        ))}
                      </Tbody>
                    </Table>
                  </ExpandableSection>
                )}
            </CardBody>
          </Card>
        )}

        <Title headingLevel="h3" style={{ marginBottom: 8 }}>
          Recent shutdowns
        </Title>

        {!shutdownsLoaded ? (
          <EmptyState>
            <EmptyStateBody>Loading shutdowns...</EmptyStateBody>
          </EmptyState>
        ) : sortedShutdowns.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No shutdowns recorded.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="Recent shutdowns" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Phase</Th>
                <Th>Test</Th>
                <Th>Nodes</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {sortedShutdowns.map((s) => (
                <Tr key={s.metadata?.uid || s.metadata?.name}>
                  <Td dataLabel="Name">{s.metadata?.name || '-'}</Td>
                  <Td dataLabel="Phase">
                    <PhaseLabel phase={s.status?.phase} />
                  </Td>
                  <Td dataLabel="Test">{s.spec?.dryRun ? 'Yes' : 'No'}</Td>
                  <Td dataLabel="Nodes">
                    {s.status?.totalNodes != null
                      ? `${s.status?.shutdownNodes ?? 0}/${s.status.totalNodes}`
                      : '-'}
                  </Td>
                  <Td dataLabel="Age">
                    {formatTimestamp(s.metadata?.creationTimestamp)}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </CardBody>

      <ShutdownModal
        isOpen={isTestModalOpen}
        onClose={() => setIsTestModalOpen(false)}
        pvcName={pvcName}
        namespace={namespace}
        mode="test"
      />
      <ShutdownModal
        isOpen={isShutdownModalOpen}
        onClose={() => setIsShutdownModalOpen(false)}
        pvcName={pvcName}
        namespace={namespace}
        mode="shutdown"
      />
    </Card>
  );
};

export default ShutdownSection;
