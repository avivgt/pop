import React from 'react';
import {
  Modal,
  ModalVariant,
  Button,
  TextInput,
  Alert,
  Form,
  FormGroup,
  Title,
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
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const [error, setError] = React.useState<string | undefined>(undefined);

  const handleSubmit = (dryRun: boolean) => {
    if (!reason.trim()) {
      setError('A reason is required.');
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
    onClose();
  };

  return (
    <Modal
      variant={ModalVariant.small}
      title="Cluster Shutdown"
      isOpen={isOpen}
      onClose={handleClose}
      actions={[
        <Button
          key="test"
          variant="primary"
          onClick={() => handleSubmit(true)}
          isLoading={isSubmitting}
          isDisabled={isSubmitting || !reason.trim()}
        >
          Test Shutdown
        </Button>,
        <Button
          key="shutdown"
          variant="danger"
          onClick={() => handleSubmit(false)}
          isLoading={isSubmitting}
          isDisabled={isSubmitting || !reason.trim()}
        >
          Shutdown Cluster
        </Button>,
        <Button key="cancel" variant="link" onClick={handleClose} isDisabled={isSubmitting}>
          Cancel
        </Button>,
      ]}
    >
      <Form>
        <FormGroup label="Reason" isRequired fieldId="shutdown-reason">
          <TextInput
            id="shutdown-reason"
            value={reason}
            onChange={(_event, value) => setReason(value)}
            placeholder="e.g. Scheduled maintenance window"
            isRequired
          />
        </FormGroup>
        <Title headingLevel="h4" style={{ marginTop: 16 }}>
          Options
        </Title>
        <Alert
          variant="info"
          isInline
          isPlain
          title="Test Shutdown runs the full procedure (validate, cordon, hooks, backup) without powering down nodes."
        />
        <Alert
          variant="warning"
          isInline
          isPlain
          title="Shutdown Cluster will power off all nodes. Ensure you have a plan to restart."
          style={{ marginTop: 8 }}
        />
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
