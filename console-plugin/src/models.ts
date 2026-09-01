import { K8sModel } from '@openshift-console/dynamic-plugin-sdk';

export const ClusterLifecycleConfigModel: K8sModel = {
  apiGroup: 'pop.openshift.io',
  apiVersion: 'v1alpha1',
  kind: 'ClusterLifecycleConfig',
  plural: 'clusterlifecycleconfigs',
  label: 'ClusterLifecycleConfig',
  labelPlural: 'ClusterLifecycleConfigs',
  abbr: 'CLC',
  namespaced: true,
};

export const EtcdBackupModel: K8sModel = {
  apiGroup: 'pop.openshift.io',
  apiVersion: 'v1alpha1',
  kind: 'EtcdBackup',
  plural: 'etcdbackups',
  label: 'EtcdBackup',
  labelPlural: 'EtcdBackups',
  abbr: 'EB',
  namespaced: true,
};

export const ClusterShutdownModel: K8sModel = {
  apiGroup: 'pop.openshift.io',
  apiVersion: 'v1alpha1',
  kind: 'ClusterShutdown',
  plural: 'clustershutdowns',
  label: 'ClusterShutdown',
  labelPlural: 'ClusterShutdowns',
  abbr: 'CS',
  namespaced: true,
};

export const BackupScheduleModel: K8sModel = {
  apiGroup: 'pop.openshift.io',
  apiVersion: 'v1alpha1',
  kind: 'BackupSchedule',
  plural: 'backupschedules',
  label: 'BackupSchedule',
  labelPlural: 'BackupSchedules',
  abbr: 'BS',
  namespaced: true,
};

export const ClusterRestartModel: K8sModel = {
  apiGroup: 'pop.openshift.io',
  apiVersion: 'v1alpha1',
  kind: 'ClusterRestart',
  plural: 'clusterrestarts',
  label: 'ClusterRestart',
  labelPlural: 'ClusterRestarts',
  abbr: 'CR',
  namespaced: true,
};

export const AutomatedProceduresConfigModel: K8sModel = {
  apiGroup: 'pop.openshift.io',
  apiVersion: 'v1alpha1',
  kind: 'AutomatedProceduresConfig',
  plural: 'automatedproceduresconfigs',
  label: 'AutomatedProceduresConfig',
  labelPlural: 'AutomatedProceduresConfigs',
  abbr: 'APC',
  namespaced: true,
};

export const PowerEventModel: K8sModel = {
  apiGroup: 'pop.openshift.io',
  apiVersion: 'v1alpha1',
  kind: 'PowerEvent',
  plural: 'powerevents',
  label: 'PowerEvent',
  labelPlural: 'PowerEvents',
  abbr: 'PE',
  namespaced: true,
};

// --- Fleet / Hub models ---

export const FleetShutdownModel: K8sModel = {
  apiGroup: 'pop.openshift.io',
  apiVersion: 'v1alpha1',
  kind: 'FleetShutdown',
  plural: 'fleetshutdowns',
  label: 'FleetShutdown',
  labelPlural: 'FleetShutdowns',
  abbr: 'FS',
  namespaced: true,
};

export const FleetBootSequenceModel: K8sModel = {
  apiGroup: 'pop.openshift.io',
  apiVersion: 'v1alpha1',
  kind: 'FleetBootSequence',
  plural: 'fleetbootsequences',
  label: 'FleetBootSequence',
  labelPlural: 'FleetBootSequences',
  abbr: 'FBS',
  namespaced: true,
};

export const ManagedClusterModel: K8sModel = {
  apiGroup: 'cluster.open-cluster-management.io',
  apiVersion: 'v1',
  kind: 'ManagedCluster',
  plural: 'managedclusters',
  label: 'ManagedCluster',
  labelPlural: 'ManagedClusters',
  abbr: 'MC',
  namespaced: false,
};

export const PlacementModel: K8sModel = {
  apiGroup: 'cluster.open-cluster-management.io',
  apiVersion: 'v1beta1',
  kind: 'Placement',
  plural: 'placements',
  label: 'Placement',
  labelPlural: 'Placements',
  abbr: 'PL',
  namespaced: true,
};
