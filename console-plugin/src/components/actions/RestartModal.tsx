import React from 'react';
import {
  Modal,
  ModalVariant,
  Button,
  Alert,
} from '@patternfly/react-core';
import { k8sCreate } from '@openshift-console/dynamic-plugin-sdk';
import { ClusterRestartModel } from '../../models';

interface RestartModalProps {
  isOpen: boolean;
  onClose: () => void;
  namespace: string;
}

const RestartModal: React.FC<RestartModalProps> = ({
  isOpen,
  onClose,
  namespace,
}) => {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const [error, setError] = React.useState<string | undefined>(undefined);

  const handleConfirm = () => {
    setIsSubmitting(true);
    setError(undefined);

    const restartName = `restart-${Date.now()}`;

    k8sCreate({
      model: ClusterRestartModel,
      data: {
        apiVersion: 'pop.openshift.io/v1alpha1',
        kind: 'ClusterRestart',
        metadata: {
          name: restartName,
          namespace,
        },
        spec: {
          autoApproveCSRs: true,
        },
      },
    })
      .then(() => {
        setIsSubmitting(false);
        onClose();
      })
      .catch((err: Error) => {
        setIsSubmitting(false);
        setError(err.message || 'Failed to create cluster restart');
      });
  };

  const handleClose = () => {
    setError(undefined);
    setIsSubmitting(false);
    onClose();
  };

  return (
    <Modal
      variant={ModalVariant.small}
      title="Restart Cluster"
      isOpen={isOpen}
      onClose={handleClose}
      actions={[
        <Button
          key="confirm"
          variant="primary"
          onClick={handleConfirm}
          isLoading={isSubmitting}
          isDisabled={isSubmitting}
        >
          Restart Cluster
        </Button>,
        <Button key="cancel" variant="link" onClick={handleClose} isDisabled={isSubmitting}>
          Cancel
        </Button>,
      ]}
    >
      <p>
        Are you sure you want to initiate a cluster restart? This will wait for all nodes to
        come back online and automatically approve pending CSRs.
      </p>
      {error && (
        <Alert variant="danger" isInline title="Error creating restart" style={{ marginTop: 16 }}>
          {error}
        </Alert>
      )}
    </Modal>
  );
};

export default RestartModal;
