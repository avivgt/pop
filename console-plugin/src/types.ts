import { K8sResourceCommon } from '@openshift-console/dynamic-plugin-sdk';

// --- ClusterLifecycleConfig ---

export interface BackupStatusSummary {
  lastBackupTime?: string;
  lastSuccessfulBackup?: string;
  nextBackupTime?: string;
  activeBackup?: string;
  totalBackups?: number;
  failedBackups?: number;
  resolvedSchedule?: string;
}

export interface PowerStatusSummary {
  status?: string;
  activeEvent?: string;
  lastEventTime?: string;
  alertEndpointReady?: boolean;
}

export interface ClusterStatusSummary {
  certificateExpiry?: string;
  certificateRemainingDays?: number;
  nodeCount?: number;
  controlPlaneNodes?: number;
  workerNodes?: number;
}

export interface BackupScheduleConfig {
  enabled?: boolean;
  frequency?: string;
  time?: string;
  dayOfWeek?: string;
  dayOfMonth?: number;
}

export interface BackupStorageConfig {
  pvcName: string;
}

export interface LifecycleBackupConfig {
  storage: BackupStorageConfig;
  schedule?: BackupScheduleConfig;
  timeoutMinutes?: number;
}

export interface LifecyclePowerConfig {
  enabled: boolean;
  clusterPriority?: string;
  batteryCapacityMinutes?: number;
  powerConfirmationSeconds?: number;
  autonomousShutdown?: boolean;
}

export interface ClusterLifecycleConfigResource extends K8sResourceCommon {
  spec?: {
    backup?: LifecycleBackupConfig;
    shutdown?: {
      defaultDelayMinutes?: number;
      cordonNodes?: boolean;
      notifyApplications?: boolean;
    };
    powerProtection?: LifecyclePowerConfig;
  };
  status?: {
    backup?: BackupStatusSummary;
    power?: PowerStatusSummary;
    cluster?: ClusterStatusSummary;
  };
}

// --- EtcdBackup ---

export interface EtcdBackupResource extends K8sResourceCommon {
  spec?: {
    pvcName?: string;
    controlPlaneNode?: string;
    backupTimeoutMinutes?: number;
  };
  status?: {
    phase?: string;
    backupNode?: string;
    backupPath?: string;
    startTime?: string;
    completionTime?: string;
    message?: string;
  };
}

// --- ClusterShutdown ---

export interface ClusterShutdownResource extends K8sResourceCommon {
  spec?: {
    pvcName?: string;
    dryRun?: boolean;
    shutdownDelayMinutes?: number;
    reason?: string;
  };
  status?: {
    phase?: string;
    totalNodes?: number;
    shutdownNodes?: number;
    certificateExpiry?: string;
    message?: string;
  };
}

// --- ClusterRestart ---

export interface ClusterRestartResource extends K8sResourceCommon {
  spec?: {
    autoApproveCSRs?: boolean;
    waitTimeoutMinutes?: number;
  };
  status?: {
    phase?: string;
    totalNodes?: number;
    readyNodes?: number;
    approvedCSRs?: number;
    message?: string;
  };
}

// --- PowerEvent ---

export interface PowerEventResource extends K8sResourceCommon {
  spec?: {
    eventType?: string;
    source?: string;
    triggeredBy?: string;
    message?: string;
  };
  status?: {
    phase?: string;
    detectedAt?: string;
    confirmedAt?: string;
    cancelledAt?: string;
    message?: string;
  };
}
