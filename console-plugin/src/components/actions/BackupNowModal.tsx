import React from 'react';
import {
  Modal,
  ModalVariant,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Alert,
} from '@patternfly/react-core';
import { k8sCreate } from '@openshift-console/dynamic-plugin-sdk';
import { EtcdBackupModel } from '../../models';

interface BackupNowModalProps {
  isOpen: boolean;
  onClose: () => void;
  pvcName: string;
  namespace: string;
}

const BackupNowModal: React.FC<BackupNowModalProps> = ({
  isOpen,
  onClose,
  pvcName,
  namespace,
}) => {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const [error, setError] = React.useState<string | undefined>(undefined);

  const handleConfirm = () => {
    setIsSubmitting(true);
    setError(undefined);

    const backupName = `manual-backup-${Date.now()}`;

    k8sCreate({
      model: EtcdBackupModel,
      data: {
        apiVersion: 'pop.openshift.io/v1alpha1',
        kind: 'EtcdBackup',
        metadata: {
          name: backupName,
          namespace,
        },
        spec: {
          pvcName,
        },
      },
    })
      .then(() => {
        setIsSubmitting(false);
        onClose();
      })
      .catch((err: Error) => {
        setIsSubmitting(false);
        setError(err.message || 'Failed to create backup');
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
      isOpen={isOpen}
      onClose={handleClose}
    >
      <ModalHeader title="Trigger Backup" />
      <ModalBody>
        <p>
          Are you sure you want to trigger an etcd backup now? The backup will be stored on
          PVC <strong>{pvcName}</strong>.
        </p>
        {error && (
          <Alert variant="danger" isInline title="Error creating backup" style={{ marginTop: 16 }}>
            {error}
          </Alert>
        )}
      </ModalBody>
      <ModalFooter>
        <Button
          variant="primary"
          onClick={handleConfirm}
          isLoading={isSubmitting}
          isDisabled={isSubmitting}
        >
          Backup Now
        </Button>
        <Button variant="link" onClick={handleClose} isDisabled={isSubmitting}>
          Cancel
        </Button>
      </ModalFooter>
    </Modal>
  );
};

export default BackupNowModal;
