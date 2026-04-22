import React from 'react';
import {
  Card,
  CardTitle,
  CardBody,
  Button,
  EmptyState,
  EmptyStateBody,
  Title,
  List,
  ListItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  ClusterLifecycleConfigResource,
  ClusterShutdownResource,
  ClusterRestartResource,
} from '../types';
import { formatTimestamp } from '../utils';
import PhaseLabel from './shared/PhaseLabel';
import ShutdownModal from './actions/ShutdownModal';

interface OperationRow {
  name: string;
  kind: string;
  phase: string | undefined;
  createdAt: string | undefined;
}

interface OperationsSectionProps {
  config: ClusterLifecycleConfigResource | undefined;
  shutdowns: ClusterShutdownResource[];
  restarts: ClusterRestartResource[];
  shutdownsLoaded: boolean;
  restartsLoaded: boolean;
  namespace: string;
}

const OperationsSection: React.FC<OperationsSectionProps> = ({
  config,
  shutdowns,
  restarts,
  shutdownsLoaded,
  restartsLoaded,
  namespace,
}) => {
  const [isShutdownOpen, setIsShutdownOpen] = React.useState(false);

  const pvcName = config?.spec?.backup?.storage?.pvcName || 'etcd-backup-pvc';
  const loaded = shutdownsLoaded && restartsLoaded;

  const recentOperations: OperationRow[] = [
    ...(shutdowns || []).map((s) => ({
      name: s.metadata?.name || '-',
      kind: 'Shutdown',
      phase: s.status?.phase,
      createdAt: s.metadata?.creationTimestamp,
    })),
    ...(restarts || []).map((r) => ({
      name: r.metadata?.name || '-',
      kind: 'Recovery',
      phase: r.status?.phase,
      createdAt: r.metadata?.creationTimestamp,
    })),
  ]
    .sort((a, b) => {
      const aTime = new Date(a.createdAt || 0).getTime();
      const bTime = new Date(b.createdAt || 0).getTime();
      return bTime - aTime;
    })
    .slice(0, 5);

  return (
    <Card>
      <CardTitle>
        <Title headingLevel="h2">Cluster Operations</Title>
      </CardTitle>
      <CardBody>
        <List isPlain style={{ marginBottom: 16 }}>
          <ListItem><strong>Shutdown Cluster</strong> — Backs up etcd, cordons nodes, runs pre-shutdown hooks, and gracefully powers down all nodes.</ListItem>
          <ListItem><strong>Post-Restart Recovery</strong> — Runs automatically after the cluster boots back up. Approves pending CSRs and verifies all operators are healthy.</ListItem>
        </List>

        <div style={{ marginBottom: 16 }}>
          <Button variant="danger" onClick={() => setIsShutdownOpen(true)}>
            Shutdown Cluster
          </Button>
        </div>

        <Title headingLevel="h3" style={{ marginBottom: 8 }}>
          Recent operations
        </Title>

        {!loaded ? (
          <EmptyState>
            <EmptyStateBody>Loading operations...</EmptyStateBody>
          </EmptyState>
        ) : recentOperations.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No recent operations.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="Recent operations" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Type</Th>
                <Th>Phase</Th>
                <Th>Age</Th>
              </Tr>
            </Thead>
            <Tbody>
              {recentOperations.map((op) => (
                <Tr key={`${op.kind}-${op.name}`}>
                  <Td dataLabel="Name">{op.name}</Td>
                  <Td dataLabel="Type">{op.kind}</Td>
                  <Td dataLabel="Phase">
                    <PhaseLabel phase={op.phase} />
                  </Td>
                  <Td dataLabel="Age">{formatTimestamp(op.createdAt)}</Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </CardBody>

      <ShutdownModal
        isOpen={isShutdownOpen}
        onClose={() => setIsShutdownOpen(false)}
        pvcName={pvcName}
        namespace={namespace}
      />
    </Card>
  );
};

export default OperationsSection;
