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

export interface S3Config {
  bucket?: string;
  region?: string;
  endpoint?: string;
  credentialsSecret?: string;
  prefix?: string;
}

export interface ExportTarget {
  type?: string;
  s3?: S3Config;
}

export interface BackupStorageConfig {
  pvcName: string;
  exportTarget?: ExportTarget;
}

export interface LifecycleBackupConfig {
  storage: BackupStorageConfig;
  schedule?: BackupScheduleConfig;
  timeoutMinutes?: number;
}

export interface AlertEndpointConfig {
  port?: number;
  authTokenSecret?: string;
}

export interface WebhookNotificationConfig {
  enabled: boolean;
  url?: string;
}

export interface NotificationConfig {
  webhook?: WebhookNotificationConfig;
  kubernetesEvents?: { enabled: boolean };
}

export interface ClusterLifecycleConfigResource extends K8sResourceCommon {
  spec?: {
    backup?: LifecycleBackupConfig;
    shutdown?: {
      defaultDelayMinutes?: number;
      cordonNodes?: boolean;
      notifyApplications?: boolean;
    };
    notification?: NotificationConfig;
  };
  status?: {
    backup?: BackupStatusSummary;
    cluster?: ClusterStatusSummary;
  };
}

// --- EtcdBackup ---

export interface EtcdBackupResource extends K8sResourceCommon {
  spec?: {
    pvcName?: string;
    controlPlaneNode?: string;
    backupTimeoutMinutes?: number;
    retentionCount?: number;
  };
  status?: {
    phase?: string;
    backupNode?: string;
    backupPath?: string;
    startTime?: string;
    completionTime?: string;
    message?: string;
    backupSizeBytes?: number;
    backupSize?: string;
    pvcUsedBytes?: number;
    pvcCapacityBytes?: number;
    pvcUsagePercent?: number;
    estimatedCapacity?: number;
    recommendedPVCSize?: string;
  };
}

// --- ClusterShutdown ---

export interface NodeShutdownStatus {
  name: string;
  role: string;
  status: string;
  message?: string;
}

export interface HookStatus {
  name: string;
  phase: string;
  message?: string;
}

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
    cordonedNodes?: number;
    certificateExpiry?: string;
    message?: string;
    etcdBackupRef?: string;
    startTime?: string;
    completionTime?: string;
    estimatedCompletionTime?: string;
    nodeStatuses?: NodeShutdownStatus[];
    hookStatuses?: HookStatus[];
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

// --- AutomatedProceduresConfig ---

export interface SNMPSourceConfig {
  enabled: boolean;
  host?: string;
  port?: number;
  community?: string;
  version?: string;
  pollingIntervalSeconds?: number;
  batteryStatusOID?: string;
  outputSourceOID?: string;
  batteryRemainingOID?: string;
  authSecret?: string;
}

export interface IPMISourceConfig {
  enabled: boolean;
  host?: string;
  port?: number;
  protocol?: string;
  pollingIntervalSeconds?: number;
  authSecret?: string;
  tlsInsecure?: boolean;
}

export interface NUTSourceConfig {
  enabled: boolean;
  host?: string;
  port?: number;
  upsName?: string;
  pollingIntervalSeconds?: number;
  authSecret?: string;
}

export interface EventSourcesConfig {
  webhook?: AlertEndpointConfig;
  snmp?: SNMPSourceConfig;
  ipmi?: IPMISourceConfig;
  nut?: NUTSourceConfig;
}

export interface EventSourceStatus {
  type: string;
  connected: boolean;
  lastCheck?: string;
  message?: string;
}

export interface AutomatedProceduresConfigResource extends K8sResourceCommon {
  spec?: {
    enabled?: boolean;
    clusterPriority?: string;
    batteryCapacityMinutes?: number;
    powerConfirmationSeconds?: number;
    autonomousShutdown?: boolean;
    backupConfig?: {
      pvcName?: string;
      shutdownDelayMinutes?: number;
    };
    alertEndpoint?: AlertEndpointConfig;
    eventSources?: EventSourcesConfig;
    managementURL?: string;
    notification?: NotificationConfig;
  };
  status?: {
    alertEndpointReady?: boolean;
    powerStatus?: string;
    lastPowerEvent?: string;
    activeEventRef?: string;
    eventSources?: EventSourceStatus[];
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

// --- FleetShutdown ---

export interface FleetClusterStatus {
  name: string;
  phase?: string;
  message?: string;
  backupRef?: string;
}

export interface FleetShutdownResource extends K8sResourceCommon {
  spec?: {
    placementRef?: string;
    pvcName?: string;
    reason?: string;
    dryRun?: boolean;
    shutdownPolicy?: {
      onStuck?: string;
      hardDeadlineSeconds?: number;
    };
  };
  status?: {
    phase?: string;
    clusters?: FleetClusterStatus[];
    selectedClusters?: number;
    completedClusters?: number;
    message?: string;
    conditions?: Array<{ type: string; status: string; reason: string; message: string }>;
  };
}

// --- FleetBootSequence ---

export interface FleetBootTierStatus {
  name: string;
  ready: boolean;
  clustersTotal?: number;
  clustersReady?: number;
  message?: string;
}

export interface FleetBootSequenceResource extends K8sResourceCommon {
  spec?: {
    tiers?: Array<{
      name: string;
      placementRef: string;
      dependsOn?: string[];
      timeoutSeconds?: number;
    }>;
  };
  status?: {
    phase?: string;
    tiers?: FleetBootTierStatus[];
    startTime?: string;
    message?: string;
  };
}

// --- ManagedCluster ---

export interface ManagedClusterClaim {
  name: string;
  value: string;
}

export interface ManagedClusterResource extends K8sResourceCommon {
  spec?: {
    hubAcceptsClient?: boolean;
  };
  status?: {
    conditions?: Array<{ type: string; status: string; reason?: string; message?: string }>;
    clusterClaims?: ManagedClusterClaim[];
    version?: { kubernetes?: string };
  };
}

// --- Placement ---

export interface PlacementResource extends K8sResourceCommon {
  status?: {
    numberOfSelectedClusters?: number;
    conditions?: Array<{ type: string; status: string }>;
  };
}
