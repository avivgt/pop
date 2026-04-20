import React from 'react';
import {
  Modal,
  ModalVariant,
  Button,
  TextInput,
  Checkbox,
  Alert,
  Form,
  FormGroup,
} from '@patternfly/react-core';
import { k8sCreate } from '@openshift-console/dynamic-plugin-sdk';
import { ClusterShutdownModel } from '../../models';

interface ShutdownModalProps {
  isOpen: boolean;
  onClose: () => void;
  pvcName: string;
  namespace: string;
}

const ShutdownModal: React.FC<ShutdownModalProps> = ({
  isOpen,
  onClose,
  pvcName,
  namespace,
}) => {
  const [reason, setReason] = React.useState('');
  const [dryRun, setDryRun] = React.useState(true);
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const [error, setError] = React.useState<string | undefined>(undefined);

  const handleConfirm = () => {
    if (!reason.trim()) {
      setError('A reason is required for cluster shutdown.');
      return;
    }

    setIsSubmitting(true);
    setError(undefined);

    const shutdownName = `shutdown-${Date.now()}`;

    k8sCreate({
      model: ClusterShutdownModel,
      data: {
        apiVersion: 'pop.openshift.io/v1alpha1',
        kind: 'ClusterShutdown',
        metadata: {
          name: shutdownName,
          namespace,
        },
        spec: {
          pvcName,
          dryRun,
          reason: reason.trim(),
        },
      },
    })
      .then(() => {
        setIsSubmitting(false);
        setReason('');
        setDryRun(true);
        onClose();
      })
      .catch((err: Error) => {
        setIsSubmitting(false);
        setError(err.message || 'Failed to create cluster shutdown');
      });
  };

  const handleClose = () => {
    setError(undefined);
    setIsSubmitting(false);
    setReason('');
    setDryRun(true);
    onClose();
  };

  return (
    <Modal
      variant={ModalVariant.small}
      title="Shutdown Cluster"
      isOpen={isOpen}
      onClose={handleClose}
      actions={[
        <Button
          key="confirm"
          variant="danger"
          onClick={handleConfirm}
          isLoading={isSubmitting}
          isDisabled={isSubmitting || !reason.trim()}
        >
          {dryRun ? 'Shutdown (Dry Run)' : 'Shutdown Cluster'}
        </Button>,
        <Button key="cancel" variant="link" onClick={handleClose} isDisabled={isSubmitting}>
          Cancel
        </Button>,
      ]}
    >
      <Form>
        {!dryRun && (
          <Alert
            variant="danger"
            isInline
            title="This will shut down the cluster!"
            style={{ marginBottom: 16 }}
          >
            All nodes will be powered off. Ensure you have a valid reason and a plan to restart.
          </Alert>
        )}
        <FormGroup label="Reason" isRequired fieldId="shutdown-reason">
          <TextInput
            id="shutdown-reason"
            value={reason}
            onChange={(_event, value) => setReason(value)}
            placeholder="e.g. Scheduled maintenance window"
            isRequired
          />
        </FormGroup>
        <FormGroup fieldId="shutdown-dryrun">
          <Checkbox
            id="shutdown-dryrun"
            label="Dry run (validate and backup only, do not shut down nodes)"
            isChecked={dryRun}
            onChange={(_event, checked) => setDryRun(checked)}
          />
        </FormGroup>
      </Form>
      {error && (
        <Alert variant="danger" isInline title="Error" style={{ marginTop: 16 }}>
          {error}
        </Alert>
      )}
    </Modal>
  );
};

export default ShutdownModal;
