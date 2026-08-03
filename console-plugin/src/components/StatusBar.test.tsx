import '@testing-library/jest-dom';
import React from 'react';
import { render, screen } from '@testing-library/react';
import StatusBar from './StatusBar';
import {
  ClusterLifecycleConfigResource,
  AutomatedProceduresConfigResource,
} from '../types';

const baseConfig = (): ClusterLifecycleConfigResource => ({
  apiVersion: 'pop.openshift.io/v1alpha1',
  kind: 'ClusterLifecycleConfig',
  metadata: { name: 'cluster' },
  status: {
    cluster: {
      certificateRemainingDays: 45,
      nodeCount: 6,
      controlPlaneNodes: 3,
      workerNodes: 3,
    },
  },
});

const apConfig = (powerStatus: string): AutomatedProceduresConfigResource => ({
  apiVersion: 'pop.openshift.io/v1alpha1',
  kind: 'AutomatedProceduresConfig',
  metadata: { name: 'ap' },
  status: { powerStatus },
});

describe('StatusBar', () => {
  it('renders nothing until loaded', () => {
    const { container } = render(
      <StatusBar config={baseConfig()} loaded={false} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing when config is undefined', () => {
    const { container } = render(<StatusBar config={undefined} loaded={true} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('shows the power status from the automated-procedures config', () => {
    render(
      <StatusBar config={baseConfig()} apConfig={apConfig('Normal')} loaded={true} />,
    );
    expect(screen.getByText(/Power:\s*Normal/)).toBeInTheDocument();
  });

  it('defaults power status to Unknown when no AP config is provided', () => {
    render(<StatusBar config={baseConfig()} loaded={true} />);
    expect(screen.getByText(/Power:\s*Unknown/)).toBeInTheDocument();
  });

  it('renders certificate days and node breakdown', () => {
    render(
      <StatusBar config={baseConfig()} apConfig={apConfig('Normal')} loaded={true} />,
    );
    expect(screen.getByText(/Cert:\s*45 days/)).toBeInTheDocument();
    expect(screen.getByText(/Nodes:\s*6\s*\(3 CP \/ 3 W\)/)).toBeInTheDocument();
  });

  it('shows N/A when certificate and node data are absent', () => {
    const cfg: ClusterLifecycleConfigResource = {
      ...baseConfig(),
      status: { cluster: {} },
    };
    render(<StatusBar config={cfg} apConfig={apConfig('OnBattery')} loaded={true} />);
    expect(screen.getByText(/Cert:\s*N\/A/)).toBeInTheDocument();
    // nodeCount undefined -> "Nodes: N/A" with no CP/W suffix.
    expect(screen.getByText(/Nodes:\s*N\/A/)).toBeInTheDocument();
    expect(screen.queryByText(/CP \//)).not.toBeInTheDocument();
  });

  it('reflects a non-normal power status', () => {
    render(
      <StatusBar config={baseConfig()} apConfig={apConfig('OnBattery')} loaded={true} />,
    );
    expect(screen.getByText(/Power:\s*OnBattery/)).toBeInTheDocument();
  });
});
