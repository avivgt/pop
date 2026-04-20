import React, { useState } from 'react';
import {
  Card,
  CardTitle,
  CardBody,
  Form,
  FormGroup,
  TextInput,
  Switch,
  Button,
  ActionGroup,
  Alert,
  ExpandableSection,
  Grid,
  GridItem,
  FormSelect,
  FormSelectOption,
} from '@patternfly/react-core';
import { k8sPatch } from '@openshift-console/dynamic-plugin-sdk';
import { ClusterLifecycleConfigModel } from '../models';
import { ClusterLifecycleConfigResource } from '../types';

interface SettingsSectionProps {
  config?: ClusterLifecycleConfigResource;
  namespace: string;
}

const SettingsSection: React.FC<SettingsSectionProps> = ({ config, namespace }) => {
  const spec = config?.spec;

  // Storage
  const [pvcName, setPvcName] = useState(spec?.backup?.storage?.pvcName || '');
  const [s3Enabled, setS3Enabled] = useState(spec?.backup?.storage?.exportTarget?.type === 'S3');
  const [s3Bucket, setS3Bucket] = useState(spec?.backup?.storage?.exportTarget?.s3?.bucket || '');
  const [s3Region, setS3Region] = useState(spec?.backup?.storage?.exportTarget?.s3?.region || 'us-east-1');
  const [s3Endpoint, setS3Endpoint] = useState(spec?.backup?.storage?.exportTarget?.s3?.endpoint || '');
  const [s3CredSecret, setS3CredSecret] = useState(spec?.backup?.storage?.exportTarget?.s3?.credentialsSecret || '');
  const [s3Prefix, setS3Prefix] = useState(spec?.backup?.storage?.exportTarget?.s3?.prefix || '');

  // PSU
  const [psuManagementURL, setPsuManagementURL] = useState(spec?.powerProtection?.managementURL || '');
  const [psuAlertPort, setPsuAlertPort] = useState(spec?.powerProtection?.alertEndpoint?.port || 8082);
  const [psuAuthSecret, setPsuAuthSecret] = useState(spec?.powerProtection?.alertEndpoint?.authTokenSecret || '');

  // Notifications
  const [webhookEnabled, setWebhookEnabled] = useState(spec?.notification?.webhook?.enabled || false);
  const [webhookURL, setWebhookURL] = useState(spec?.notification?.webhook?.url || '');

  // Schedule
  const [scheduleEnabled, setScheduleEnabled] = useState(spec?.backup?.schedule?.enabled !== false);
  const [frequency, setFrequency] = useState(spec?.backup?.schedule?.frequency || 'Daily');
  const [scheduleTime, setScheduleTime] = useState(spec?.backup?.schedule?.time || '02:00');

  const [saving, setSaving] = useState(false);
  const [saveResult, setSaveResult] = useState<{ type: 'success' | 'danger'; message: string } | null>(null);

  const handleSave = async () => {
    if (!config?.metadata?.name) return;
    setSaving(true);
    setSaveResult(null);

    const patch = [
      {
        op: 'replace' as const,
        path: '/spec/backup/storage',
        value: {
          pvcName,
          ...(s3Enabled
            ? {
                exportTarget: {
                  type: 'S3',
                  s3: {
                    bucket: s3Bucket,
                    region: s3Region,
                    endpoint: s3Endpoint || undefined,
                    credentialsSecret: s3CredSecret,
                    prefix: s3Prefix || undefined,
                  },
                },
              }
            : { exportTarget: { type: 'PVC' } }),
        },
      },
      {
        op: 'replace' as const,
        path: '/spec/backup/schedule',
        value: {
          enabled: scheduleEnabled,
          frequency,
          time: scheduleTime,
        },
      },
      {
        op: 'replace' as const,
        path: '/spec/powerProtection/managementURL',
        value: psuManagementURL,
      },
      {
        op: 'replace' as const,
        path: '/spec/powerProtection/alertEndpoint',
        value: {
          port: psuAlertPort,
          authTokenSecret: psuAuthSecret,
        },
      },
      {
        op: 'replace' as const,
        path: '/spec/notification',
        value: {
          webhook: { enabled: webhookEnabled, url: webhookURL },
          kubernetesEvents: { enabled: true },
        },
      },
    ];

    try {
      await k8sPatch({
        model: ClusterLifecycleConfigModel,
        resource: config,
        data: patch,
      });
      setSaveResult({ type: 'success', message: 'Settings saved successfully' });
    } catch (e) {
      setSaveResult({ type: 'danger', message: `Failed to save: ${e}` });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardTitle>Settings</CardTitle>
      <CardBody>
        {saveResult && (
          <Alert variant={saveResult.type} isInline title={saveResult.message} style={{ marginBottom: 16 }} />
        )}

        <Form>
          {/* Backup Schedule */}
          <ExpandableSection toggleText="Backup Schedule" isExpanded>
            <Grid hasGutter>
              <GridItem span={4}>
                <FormGroup label="Scheduled Backups" fieldId="schedule-enabled">
                  <Switch
                    id="schedule-enabled"
                    isChecked={scheduleEnabled}
                    onChange={(_e, val) => setScheduleEnabled(val)}
                    label="Enabled"
                    labelOff="Disabled"
                  />
                </FormGroup>
              </GridItem>
              <GridItem span={4}>
                <FormGroup label="Frequency" fieldId="frequency">
                  <FormSelect
                    id="frequency"
                    value={frequency}
                    onChange={(_e, val) => setFrequency(val)}
                    isDisabled={!scheduleEnabled}
                  >
                    <FormSelectOption value="Every4Hours" label="Every 4 hours" />
                    <FormSelectOption value="Every8Hours" label="Every 8 hours" />
                    <FormSelectOption value="Every12Hours" label="Every 12 hours" />
                    <FormSelectOption value="Daily" label="Daily" />
                    <FormSelectOption value="Weekly" label="Weekly" />
                    <FormSelectOption value="Monthly" label="Monthly" />
                  </FormSelect>
                </FormGroup>
              </GridItem>
              <GridItem span={4}>
                <FormGroup label="Time (UTC)" fieldId="schedule-time">
                  <TextInput
                    id="schedule-time"
                    value={scheduleTime}
                    onChange={(_e, val) => setScheduleTime(val)}
                    placeholder="02:00"
                    isDisabled={!scheduleEnabled}
                  />
                </FormGroup>
              </GridItem>
            </Grid>
          </ExpandableSection>

          {/* Storage */}
          <ExpandableSection toggleText="Backup Storage" isExpanded>
            <Grid hasGutter>
              <GridItem span={6}>
                <FormGroup label="PVC Name" fieldId="pvc-name" isRequired>
                  <TextInput id="pvc-name" value={pvcName} onChange={(_e, val) => setPvcName(val)} isRequired />
                </FormGroup>
              </GridItem>
              <GridItem span={6}>
                <FormGroup label="Export to S3" fieldId="s3-enabled">
                  <Switch
                    id="s3-enabled"
                    isChecked={s3Enabled}
                    onChange={(_e, val) => setS3Enabled(val)}
                    label="Enabled"
                    labelOff="PVC only"
                  />
                </FormGroup>
              </GridItem>
              {s3Enabled && (
                <>
                  <GridItem span={4}>
                    <FormGroup label="S3 Bucket" fieldId="s3-bucket" isRequired>
                      <TextInput id="s3-bucket" value={s3Bucket} onChange={(_e, val) => setS3Bucket(val)} />
                    </FormGroup>
                  </GridItem>
                  <GridItem span={4}>
                    <FormGroup label="Region" fieldId="s3-region">
                      <TextInput id="s3-region" value={s3Region} onChange={(_e, val) => setS3Region(val)} />
                    </FormGroup>
                  </GridItem>
                  <GridItem span={4}>
                    <FormGroup label="Endpoint (MinIO/ODF)" fieldId="s3-endpoint">
                      <TextInput
                        id="s3-endpoint"
                        value={s3Endpoint}
                        onChange={(_e, val) => setS3Endpoint(val)}
                        placeholder="https://minio.example.com"
                      />
                    </FormGroup>
                  </GridItem>
                  <GridItem span={6}>
                    <FormGroup label="Credentials Secret" fieldId="s3-cred" isRequired>
                      <TextInput
                        id="s3-cred"
                        value={s3CredSecret}
                        onChange={(_e, val) => setS3CredSecret(val)}
                        placeholder="Secret name with access-key and secret-key"
                      />
                    </FormGroup>
                  </GridItem>
                  <GridItem span={6}>
                    <FormGroup label="Path Prefix" fieldId="s3-prefix">
                      <TextInput
                        id="s3-prefix"
                        value={s3Prefix}
                        onChange={(_e, val) => setS3Prefix(val)}
                        placeholder="backups/cluster-1"
                      />
                    </FormGroup>
                  </GridItem>
                </>
              )}
            </Grid>
          </ExpandableSection>

          {/* PSU / Power */}
          <ExpandableSection toggleText="PSU Management">
            <Grid hasGutter>
              <GridItem span={6}>
                <FormGroup label="UPS/PDU Management URL" fieldId="psu-url">
                  <TextInput
                    id="psu-url"
                    value={psuManagementURL}
                    onChange={(_e, val) => setPsuManagementURL(val)}
                    placeholder="https://192.168.1.100"
                  />
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="Alert Webhook Port" fieldId="psu-port">
                  <TextInput
                    id="psu-port"
                    type="number"
                    value={String(psuAlertPort)}
                    onChange={(_e, val) => setPsuAlertPort(Number(val))}
                  />
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="Auth Token Secret" fieldId="psu-auth">
                  <TextInput
                    id="psu-auth"
                    value={psuAuthSecret}
                    onChange={(_e, val) => setPsuAuthSecret(val)}
                    placeholder="psu-alert-token"
                  />
                </FormGroup>
              </GridItem>
            </Grid>
          </ExpandableSection>

          {/* Notifications */}
          <ExpandableSection toggleText="Notifications">
            <Grid hasGutter>
              <GridItem span={4}>
                <FormGroup label="Webhook Notifications" fieldId="webhook-enabled">
                  <Switch
                    id="webhook-enabled"
                    isChecked={webhookEnabled}
                    onChange={(_e, val) => setWebhookEnabled(val)}
                    label="Enabled"
                    labelOff="Disabled"
                  />
                </FormGroup>
              </GridItem>
              {webhookEnabled && (
                <GridItem span={8}>
                  <FormGroup label="Webhook URL" fieldId="webhook-url" isRequired>
                    <TextInput
                      id="webhook-url"
                      value={webhookURL}
                      onChange={(_e, val) => setWebhookURL(val)}
                      placeholder="https://hooks.slack.com/services/..."
                    />
                  </FormGroup>
                </GridItem>
              )}
            </Grid>
          </ExpandableSection>

          <ActionGroup>
            <Button variant="primary" onClick={handleSave} isLoading={saving} isDisabled={saving || !pvcName}>
              Save Settings
            </Button>
          </ActionGroup>
        </Form>
      </CardBody>
    </Card>
  );
};

export default SettingsSection;
