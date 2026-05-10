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

const SHUTDOWN_STEPS = [
  {
    key: 'Validating',
    label: 'Validate',
    description: 'Check certificate expiry, preflight checks, enumerate Ready nodes',
  },
  {
    key: 'Cordoning',
    label: 'Cordon Nodes',
    description: 'Mark all nodes as unschedulable to prevent new pod scheduling',
  },
  {
    key: 'RunningHooks',
    label: 'Pre-shutdown Hooks',
    description: 'Run user-defined jobs (flush databases, deregister from load balancers)',
  },
  {
    key: 'NotifyingApps',
    label: 'Notify Applications',
    description: 'Create shutdown signal ConfigMap so applications can react',
  },
  {
    key: 'BackingUp',
    label: 'Backup etcd',
    description: 'Defragment etcd, take snapshot, verify integrity, export to PVC',
  },
  {
    key: 'ShuttingDown',
    label: 'Shut Down Nodes',
    description: 'Send shutdown command to each node (workers first, then control plane)',
  },
  {
    key: 'Complete',
    label: 'Complete',
    description: 'All steps finished successfully',
  },
];

function getStepStatus(
  stepIndex: number,
  currentIndex: number,
  phase: string | undefined,
  isDryRun: boolean,
): 'done' | 'active' | 'pending' | 'failed' | 'skipped' {
  if (phase === 'Complete' || phase === 'Failed') {
    if (phase === 'Failed' && stepIndex === currentIndex) return 'failed';
    if (stepIndex <= currentIndex) return phase === 'Complete' ? 'done' : 'done';
    return 'pending';
  }
  if (isDryRun && (SHUTDOWN_STEPS[stepIndex].key === 'Cordoning' || SHUTDOWN_STEPS[stepIndex].key === 'ShuttingDown')) {
    if (stepIndex < currentIndex) return 'skipped';
  }
  if (stepIndex < currentIndex) return 'done';
  if (stepIndex === currentIndex) return 'active';
  return 'pending';
}

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

  const latestShutdown = activeShutdown || [...(shutdowns || [])]
    .sort((a, b) => {
      const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
      const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
      return bTime - aTime;
    })[0];

  const showProgress = latestShutdown?.status?.phase != null;

  const sortedShutdowns = [...(shutdowns || [])]
    .sort((a, b) => {
      const aTime = new Date(a.metadata?.creationTimestamp || 0).getTime();
      const bTime = new Date(b.metadata?.creationTimestamp || 0).getTime();
      return bTime - aTime;
    })
    .slice(0, 10);

  const currentPhaseIndex = latestShutdown
    ? SHUTDOWN_STEPS.findIndex((p) => p.key === latestShutdown.status?.phase)
    : -1;

  const isDryRun = latestShutdown?.spec?.dryRun === true;
  const isActive = activeShutdown != null;
  const isComplete = latestShutdown?.status?.phase === 'Complete';
  const isFailed = latestShutdown?.status?.phase === 'Failed';

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

        {showProgress && (
          <Card isFlat style={{ marginBottom: 16 }}>
            <CardBody>
              <Split style={{ marginBottom: 12 }}>
                <SplitItem isFilled>
                  <Title headingLevel="h3">
                    {isDryRun ? 'Test Run' : 'Shutdown'}
                    {isActive ? ' in progress' : isComplete ? ' completed' : ' failed'}
                    {': '}{latestShutdown.metadata?.name}
                  </Title>
                </SplitItem>
                {latestShutdown.status?.startTime && (
                  <SplitItem>
                    <span style={{ color: '#6a6e73', fontSize: '0.875rem' }}>
                      Started {formatTimestamp(latestShutdown.status.startTime)}
                    </span>
                  </SplitItem>
                )}
              </Split>

              <div style={{ marginBottom: 16 }}>
                {SHUTDOWN_STEPS.map((step, idx) => {
                  const status = getStepStatus(idx, currentPhaseIndex, latestShutdown.status?.phase, isDryRun);

                  let icon: React.ReactNode;
                  let color: 'grey' | 'blue' | 'green' | 'red' | 'orange';
                  let textStyle: React.CSSProperties = {};

                  switch (status) {
                    case 'done':
                      icon = <CheckCircleIcon color="#3e8635" />;
                      color = 'green';
                      break;
                    case 'active':
                      icon = <InProgressIcon color="#0066cc" />;
                      color = 'blue';
                      break;
                    case 'failed':
                      icon = <ExclamationCircleIcon color="#c9190b" />;
                      color = 'red';
                      break;
                    case 'skipped':
                      icon = <CheckCircleIcon color="#6a6e73" />;
                      color = 'grey';
                      textStyle = { textDecoration: 'line-through', opacity: 0.6 };
                      break;
                    default:
                      icon = <PendingIcon color="#6a6e73" />;
                      color = 'grey';
                      break;
                  }

                  return (
                    <div
                      key={step.key}
                      style={{
                        display: 'flex',
                        alignItems: 'flex-start',
                        gap: 12,
                        padding: '8px 0',
                        borderBottom: idx < SHUTDOWN_STEPS.length - 1 ? '1px solid #d2d2d2' : 'none',
                      }}
                    >
                      <div style={{ paddingTop: 2, flexShrink: 0 }}>{icon}</div>
                      <div style={{ flex: 1 }}>
                        <div style={{ fontWeight: status === 'active' ? 600 : 400, ...textStyle }}>
                          {step.label}
                          {status === 'skipped' && ' (skipped in test)'}
                        </div>
                        <div style={{ fontSize: '0.85rem', color: '#6a6e73', ...textStyle }}>
                          {step.description}
                        </div>
                        {status === 'active' && latestShutdown.status?.message && (
                          <div style={{ fontSize: '0.85rem', color: '#0066cc', marginTop: 4 }}>
                            {latestShutdown.status.message}
                          </div>
                        )}
                        {status === 'failed' && latestShutdown.status?.message && (
                          <div style={{ fontSize: '0.85rem', color: '#c9190b', marginTop: 4 }}>
                            {latestShutdown.status.message}
                          </div>
                        )}
                      </div>
                      <div style={{ flexShrink: 0 }}>
                        <Label color={color} variant="outline" isCompact>
                          {status === 'done' ? 'Done' : status === 'active' ? 'Running' : status === 'failed' ? 'Failed' : status === 'skipped' ? 'Skipped' : 'Pending'}
                        </Label>
                      </div>
                    </div>
                  );
                })}
              </div>

              {latestShutdown.status?.totalNodes != null && (
                <DescriptionList isCompact isHorizontal style={{ marginBottom: 12 }}>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Nodes</DescriptionListTerm>
                    <DescriptionListDescription>
                      {latestShutdown.status.totalNodes} total
                      {(latestShutdown.status?.cordonedNodes ?? 0) > 0 &&
                        ` | ${latestShutdown.status.cordonedNodes} cordoned`}
                      {(latestShutdown.status?.shutdownNodes ?? 0) > 0 &&
                        ` | ${latestShutdown.status.shutdownNodes} shut down`}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  {latestShutdown.status?.etcdBackupRef && (
                    <DescriptionListGroup>
                      <DescriptionListTerm>Backup</DescriptionListTerm>
                      <DescriptionListDescription>
                        {latestShutdown.status.etcdBackupRef}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  )}
                  {latestShutdown.status?.completionTime && (
                    <DescriptionListGroup>
                      <DescriptionListTerm>Duration</DescriptionListTerm>
                      <DescriptionListDescription>
                        {(() => {
                          const start = new Date(latestShutdown.status?.startTime || 0).getTime();
                          const end = new Date(latestShutdown.status.completionTime).getTime();
                          const secs = Math.round((end - start) / 1000);
                          return secs < 60 ? `${secs}s` : `${Math.floor(secs / 60)}m ${secs % 60}s`;
                        })()}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  )}
                </DescriptionList>
              )}

              {latestShutdown.status?.nodeStatuses &&
                latestShutdown.status.nodeStatuses.length > 0 && (
                  <ExpandableSection
                    toggleText={
                      nodesExpanded ? 'Hide node details' : 'Show node details'
                    }
                    isExpanded={nodesExpanded}
                    onToggle={(_event, expanded) => setNodesExpanded(expanded)}
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
                        {latestShutdown.status.nodeStatuses.map((ns) => (
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
