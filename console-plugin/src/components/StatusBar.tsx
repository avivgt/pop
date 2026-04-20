import React from 'react';
import {
  Flex,
  FlexItem,
  Label,
  Card,
  CardBody,
} from '@patternfly/react-core';
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  OutlinedClockIcon,
  ServerIcon,
} from '@patternfly/react-icons';
import { ClusterLifecycleConfigResource } from '../types';

interface StatusBarProps {
  config: ClusterLifecycleConfigResource | undefined;
  loaded: boolean;
}

const StatusBar: React.FC<StatusBarProps> = ({ config, loaded }) => {
  if (!loaded || !config) {
    return null;
  }

  const powerStatus = config.status?.power?.status || 'Unknown';
  const isNormalPower = powerStatus === 'Normal';
  const certDays = config.status?.cluster?.certificateRemainingDays;
  const nodeCount = config.status?.cluster?.nodeCount;
  const controlPlaneNodes = config.status?.cluster?.controlPlaneNodes;
  const workerNodes = config.status?.cluster?.workerNodes;

  return (
    <Card isCompact>
      <CardBody>
        <Flex
          spaceItems={{ default: 'spaceItemsXl' }}
          alignItems={{ default: 'alignItemsCenter' }}
        >
          <FlexItem>
            <Flex
              spaceItems={{ default: 'spaceItemsSm' }}
              alignItems={{ default: 'alignItemsCenter' }}
            >
              <FlexItem>
                <Label
                  color={isNormalPower ? 'green' : 'red'}
                  icon={isNormalPower ? <CheckCircleIcon /> : <ExclamationCircleIcon />}
                >
                  Power: {powerStatus}
                </Label>
              </FlexItem>
            </Flex>
          </FlexItem>

          <FlexItem>
            <Flex
              spaceItems={{ default: 'spaceItemsSm' }}
              alignItems={{ default: 'alignItemsCenter' }}
            >
              <FlexItem>
                <Label
                  color={
                    certDays !== undefined
                      ? certDays > 30
                        ? 'green'
                        : certDays > 7
                        ? 'orange'
                        : 'red'
                      : 'grey'
                  }
                  icon={<OutlinedClockIcon />}
                >
                  Cert: {certDays !== undefined ? `${certDays} days` : 'N/A'}
                </Label>
              </FlexItem>
            </Flex>
          </FlexItem>

          <FlexItem>
            <Flex
              spaceItems={{ default: 'spaceItemsSm' }}
              alignItems={{ default: 'alignItemsCenter' }}
            >
              <FlexItem>
                <Label color="blue" icon={<ServerIcon />}>
                  Nodes: {nodeCount !== undefined ? nodeCount : 'N/A'}
                  {controlPlaneNodes !== undefined && workerNodes !== undefined
                    ? ` (${controlPlaneNodes} CP / ${workerNodes} W)`
                    : ''}
                </Label>
              </FlexItem>
            </Flex>
          </FlexItem>
        </Flex>
      </CardBody>
    </Card>
  );
};

export default StatusBar;
