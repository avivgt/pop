import { POP_NAMESPACE } from '../constants';
import React from 'react';
import {
  PageSection,
  Title,
  EmptyState,
  EmptyStateBody,
  Spinner,
  Card,
  CardTitle,
  CardBody,
  CardFooter,
  Grid,
  GridItem,
  Button,
  Label,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import {
  FleetShutdownModel,
  FleetBootSequenceModel,
  ManagedClusterModel,
} from '../models';
import {
  FleetShutdownResource,
  FleetBootSequenceResource,
  ManagedClusterResource,
} from '../types';
import { getClusterClaim, formatTimestamp } from '../utils';

const NAMESPACE = POP_NAMESPACE;

const ActiveFleetShutdownsCard: React.FC = () => {
  const [fleetShutdowns, loaded] = useK8sWatchResource<FleetShutdownResource[]>({
    groupVersionKind: {
      group: FleetShutdownModel.apiGroup,
      version: FleetShutdownModel.apiVersion,
      kind: FleetShutdownModel.kind,
    },
    namespace: NAMESPACE,
    isList: true,
    namespaced: true,
  });

  const count = loaded
    ? (fleetShutdowns || []).filter(
        (s) => s.status?.phase && !['Completed', 'PartiallyFailed'].includes(s.status.phase),
      ).length
    : 0;

  return (
    <Card isFullHeight>
      <CardTitle>Active Fleet Shutdowns</CardTitle>
      <CardBody>
        <Title headingLevel="h2" size="3xl">
          {loaded ? count : <Spinner size="md" />}
        </Title>
      </CardBody>
      <CardFooter>
        <Button variant="link" component="a" href="/multicloud/fleet-operations/shutdowns" isInline>
          View All
        </Button>
      </CardFooter>
    </Card>
  );
};

const ActiveBootSequencesCard: React.FC = () => {
  const [bootSequences, loaded] = useK8sWatchResource<FleetBootSequenceResource[]>({
    groupVersionKind: {
      group: FleetBootSequenceModel.apiGroup,
      version: FleetBootSequenceModel.apiVersion,
      kind: FleetBootSequenceModel.kind,
    },
    namespace: NAMESPACE,
    isList: true,
    namespaced: true,
  });

  const count = loaded
    ? (bootSequences || []).filter(
        (b) => b.status?.phase && !['Complete', 'TimedOut'].includes(b.status.phase),
      ).length
    : 0;

  return (
    <Card isFullHeight>
      <CardTitle>Active Boot Sequences</CardTitle>
      <CardBody>
        <Title headingLevel="h2" size="3xl">
          {loaded ? count : <Spinner size="md" />}
        </Title>
      </CardBody>
      <CardFooter>
        <Button variant="link" component="a" href="/multicloud/fleet-operations/boot-sequences" isInline>
          View All
        </Button>
      </CardFooter>
    </Card>
  );
};

const FleetDashboardPage: React.FC = () => {
  const [clusters, clustersLoaded] = useK8sWatchResource<ManagedClusterResource[]>({
    groupVersionKind: {
      group: ManagedClusterModel.apiGroup,
      version: ManagedClusterModel.apiVersion,
      kind: ManagedClusterModel.kind,
    },
    isList: true,
    namespaced: false,
    selector: {
      matchLabels: { 'pop.openshift.io/enabled': 'true' },
    },
  });

  const popClusters = clusters || [];

  const onBatteryCount = popClusters.filter(
    (c) => getClusterClaim(c.status?.clusterClaims, 'power-status.pop.openshift.io') === 'OnBattery',
  ).length;

  const clusterAvailability = (c: ManagedClusterResource): string => {
    const cond = c.status?.conditions?.find((x) => x.type === 'ManagedClusterConditionAvailable');
    return cond?.status === 'True' ? 'Available' : cond?.status === 'Unknown' ? 'Unknown' : 'Unavailable';
  };

  return (
    <>
      <PageSection variant="default">
        <Title headingLevel="h1">Fleet Operations</Title>
      </PageSection>

      <PageSection>
        <Grid hasGutter>
          <GridItem md={3}>
            <Card isFullHeight>
              <CardTitle>POP-Enabled Clusters</CardTitle>
              <CardBody>
                <Title headingLevel="h2" size="3xl">
                  {clustersLoaded ? popClusters.length : <Spinner size="md" />}
                </Title>
              </CardBody>
            </Card>
          </GridItem>
          <GridItem md={3}>
            <Card isFullHeight>
              <CardTitle>On Battery</CardTitle>
              <CardBody>
                <Title headingLevel="h2" size="3xl" style={{ color: onBatteryCount > 0 ? 'var(--pf-t--global--color--status--danger--default)' : undefined }}>
                  {clustersLoaded ? onBatteryCount : <Spinner size="md" />}
                </Title>
              </CardBody>
            </Card>
          </GridItem>
          <GridItem md={3}>
            <ActiveFleetShutdownsCard />
          </GridItem>
          <GridItem md={3}>
            <ActiveBootSequencesCard />
          </GridItem>
        </Grid>
      </PageSection>

      <PageSection>
        <Card>
          <CardTitle>Managed Clusters</CardTitle>
          <CardBody>
            {!clustersLoaded ? (
              <EmptyState>
                <Spinner size="xl" />
                <EmptyStateBody>Loading cluster data...</EmptyStateBody>
              </EmptyState>
            ) : popClusters.length === 0 ? (
              <EmptyState>
                <EmptyStateBody>
                  No clusters labeled with pop.openshift.io/enabled=true.
                </EmptyStateBody>
              </EmptyState>
            ) : (
              <Table aria-label="Managed clusters table" variant="compact">
                <Thead>
                  <Tr>
                    <Th>Cluster</Th>
                    <Th>Availability</Th>
                    <Th>Power Status</Th>
                    <Th>Last Backup</Th>
                    <Th>POP Version</Th>
                    <Th>Kubernetes</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {popClusters.map((cluster) => {
                    const claims = cluster.status?.clusterClaims;
                    const powerStatus = getClusterClaim(claims, 'power-status.pop.openshift.io');
                    const lastBackup = getClusterClaim(claims, 'last-backup-time.pop.openshift.io');
                    const popVersion = getClusterClaim(claims, 'operator-version.pop.openshift.io');
                    const availability = clusterAvailability(cluster);

                    return (
                      <Tr key={cluster.metadata?.uid}>
                        <Td dataLabel="Cluster">{cluster.metadata?.name || '-'}</Td>
                        <Td dataLabel="Availability">
                          <Label color={availability === 'Available' ? 'green' : availability === 'Unknown' ? 'orange' : 'red'}>
                            {availability}
                          </Label>
                        </Td>
                        <Td dataLabel="Power Status">
                          {powerStatus ? (
                            <Label color={powerStatus === 'Normal' ? 'green' : powerStatus === 'OnBattery' ? 'red' : 'grey'}>
                              {powerStatus}
                            </Label>
                          ) : (
                            '-'
                          )}
                        </Td>
                        <Td dataLabel="Last Backup">
                          {lastBackup ? formatTimestamp(lastBackup) : '-'}
                        </Td>
                        <Td dataLabel="POP Version">{popVersion || '-'}</Td>
                        <Td dataLabel="Kubernetes">
                          {cluster.status?.version?.kubernetes || '-'}
                        </Td>
                      </Tr>
                    );
                  })}
                </Tbody>
              </Table>
            )}
          </CardBody>
        </Card>
      </PageSection>
    </>
  );
};

export default FleetDashboardPage;
