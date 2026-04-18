import React from 'react';
import { Label } from '@patternfly/react-core';
import { phaseToColor, phaseToIcon } from '../../utils';

interface PhaseLabelProps {
  phase: string | undefined;
}

const PhaseLabel: React.FC<PhaseLabelProps> = ({ phase }) => {
  const displayPhase = phase || 'Unknown';
  const color = phaseToColor(displayPhase);
  const Icon = phaseToIcon(displayPhase);

  return (
    <Label color={color} icon={<Icon />}>
      {displayPhase}
    </Label>
  );
};

export default PhaseLabel;
