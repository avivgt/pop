import React from 'react';
import {
  Modal,
  ModalVariant,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  TextInput,
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
  mode: 'test' | 'shutdown';
}

const ShutdownModal: React.FC<ShutdownModalProps> = ({
  isOpen,
  onClose,
  pvcName,
  namespace,
  mode,
}) => {
  const [reason, setReason] = React.useState('');
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const [error, setError] = React.useState<string | undefined>(undefined);

  const isTest = mode === 'test';

  const handleSubmit = () => {
    if (!isTest && !reason.trim()) {
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
          dryRun: isTest,
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
      isOpen={isOpen}
      onClose={handleClose}
    >
      <ModalHeader title={isTest ? 'Test Shutdown' : 'Shutdown Cluster'} />
      <ModalBody>
        <Form>
          {isTest ? (
            <Alert
              variant="info"
              isInline
              isPlain
              title="Runs the full shutdown procedure (validate, hooks, backup) without cordoning nodes or powering down."
            />
          ) : (
            <Alert
              variant="danger"
              isInline
              title="This will shut down the entire cluster"
              style={{ marginBottom: 16 }}
            >
              All nodes will be cordoned and powered off. This action cannot be undone from the console.
            </Alert>
          )}
          <FormGroup label="Reason" isRequired={!isTest} fieldId="shutdown-reason" style={{ marginTop: 16 }}>
            <TextInput
              id="shutdown-reason"
              value={reason}
              onChange={(_event, value) => setReason(value)}
              placeholder={isTest ? 'e.g. Pre-maintenance validation (optional)' : 'e.g. Scheduled maintenance window'}
            />
          </FormGroup>
        </Form>
        {error && (
          <Alert variant="danger" isInline title="Error" style={{ marginTop: 16 }}>
            {error}
          </Alert>
        )}
      </ModalBody>
      <ModalFooter>
        <Button
          variant={isTest ? 'primary' : 'danger'}
          onClick={handleSubmit}
          isLoading={isSubmitting}
          isDisabled={isSubmitting || (!isTest && !reason.trim())}
        >
          {isTest ? 'Run Test' : 'Shutdown Cluster'}
        </Button>
        <Button variant="link" onClick={handleClose} isDisabled={isSubmitting}>
          Cancel
        </Button>
      </ModalFooter>
    </Modal>
  );
};

export default ShutdownModal;
