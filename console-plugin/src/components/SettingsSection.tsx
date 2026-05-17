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
  Title,
  Divider,
  Grid,
  GridItem,
  FormSelect,
  FormSelectOption,
  ExpandableSection,
} from '@patternfly/react-core';
import { k8sPatch } from '@openshift-console/dynamic-plugin-sdk';
import { ClusterLifecycleConfigModel, AutomatedProceduresConfigModel } from '../models';
import { ClusterLifecycleConfigResource, AutomatedProceduresConfigResource } from '../types';

interface SettingsSectionProps {
  config?: ClusterLifecycleConfigResource;
  apConfig?: AutomatedProceduresConfigResource;
  namespace: string;
}

const SettingsSection: React.FC<SettingsSectionProps> = ({ config, apConfig, namespace: _namespace }) => {
  const spec = config?.spec;
  const apSpec = apConfig?.spec;

  // Storage
  const [pvcName, setPvcName] = useState(spec?.backup?.storage?.pvcName || '');
  const [s3Enabled, setS3Enabled] = useState(spec?.backup?.storage?.exportTarget?.type === 'S3');
  const [s3Bucket, setS3Bucket] = useState(spec?.backup?.storage?.exportTarget?.s3?.bucket || '');
  const [s3Region, setS3Region] = useState(spec?.backup?.storage?.exportTarget?.s3?.region || 'us-east-1');
  const [s3Endpoint, setS3Endpoint] = useState(spec?.backup?.storage?.exportTarget?.s3?.endpoint || '');
  const [s3CredSecret, setS3CredSecret] = useState(spec?.backup?.storage?.exportTarget?.s3?.credentialsSecret || '');
  const [s3Prefix, setS3Prefix] = useState(spec?.backup?.storage?.exportTarget?.s3?.prefix || '');

  // Notifications
  const [webhookEnabled, setWebhookEnabled] = useState(spec?.notification?.webhook?.enabled || false);
  const [webhookURL, setWebhookURL] = useState(spec?.notification?.webhook?.url || '');

  // Schedule
  const [scheduleEnabled, setScheduleEnabled] = useState(spec?.backup?.schedule?.enabled !== false);
  const [frequency, setFrequency] = useState(spec?.backup?.schedule?.frequency || 'Daily');
  const [scheduleTime, setScheduleTime] = useState(spec?.backup?.schedule?.time || '02:00');

  // Event Sources - Webhook
  const [esWebhookPort, setEsWebhookPort] = useState(apSpec?.eventSources?.webhook?.port || 8082);
  const [esWebhookAuth, setEsWebhookAuth] = useState(apSpec?.eventSources?.webhook?.authTokenSecret || '');

  // Event Sources - SNMP
  const [snmpEnabled, setSnmpEnabled] = useState(apSpec?.eventSources?.snmp?.enabled || false);
  const [snmpHost, setSnmpHost] = useState(apSpec?.eventSources?.snmp?.host || '');
  const [snmpPort, setSnmpPort] = useState(apSpec?.eventSources?.snmp?.port || 161);
  const [snmpCommunity, setSnmpCommunity] = useState(apSpec?.eventSources?.snmp?.community || 'public');
  const [snmpVersion, setSnmpVersion] = useState(apSpec?.eventSources?.snmp?.version || 'v2c');
  const [snmpInterval, setSnmpInterval] = useState(apSpec?.eventSources?.snmp?.pollingIntervalSeconds || 30);

  // Event Sources - IPMI/Redfish
  const [ipmiEnabled, setIpmiEnabled] = useState(apSpec?.eventSources?.ipmi?.enabled || false);
  const [ipmiHost, setIpmiHost] = useState(apSpec?.eventSources?.ipmi?.host || '');
  const [ipmiPort, setIpmiPort] = useState(apSpec?.eventSources?.ipmi?.port || 443);
  const [ipmiProtocol, setIpmiProtocol] = useState(apSpec?.eventSources?.ipmi?.protocol || 'redfish');
  const [ipmiAuthSecret, setIpmiAuthSecret] = useState(apSpec?.eventSources?.ipmi?.authSecret || '');
  const [ipmiInterval, setIpmiInterval] = useState(apSpec?.eventSources?.ipmi?.pollingIntervalSeconds || 30);
  const [ipmiTlsInsecure, setIpmiTlsInsecure] = useState(apSpec?.eventSources?.ipmi?.tlsInsecure || false);

  // Event Sources - NUT
  const [nutEnabled, setNutEnabled] = useState(apSpec?.eventSources?.nut?.enabled || false);
  const [nutHost, setNutHost] = useState(apSpec?.eventSources?.nut?.host || '');
  const [nutPort, setNutPort] = useState(apSpec?.eventSources?.nut?.port || 3493);
  const [nutUpsName, setNutUpsName] = useState(apSpec?.eventSources?.nut?.upsName || '');
  const [nutInterval, setNutInterval] = useState(apSpec?.eventSources?.nut?.pollingIntervalSeconds || 10);
  const [nutAuthSecret, setNutAuthSecret] = useState(apSpec?.eventSources?.nut?.authSecret || '');

  // Expandable sections
  const [snmpExpanded, setSnmpExpanded] = useState(snmpEnabled);
  const [ipmiExpanded, setIpmiExpanded] = useState(ipmiEnabled);
  const [nutExpanded, setNutExpanded] = useState(nutEnabled);

  const [saving, setSaving] = useState(false);
  const [saveResult, setSaveResult] = useState<{ type: 'success' | 'danger'; message: string } | null>(null);

  const handleSave = async () => {
    if (!config?.metadata?.name) return;
    setSaving(true);
    setSaveResult(null);

    const lifecyclePatch = [
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
        data: lifecyclePatch,
      });

      if (apConfig?.metadata?.name) {
        const eventSourcesPatch = [
          {
            op: (apConfig.spec?.eventSources ? 'replace' : 'add') as 'replace' | 'add',
            path: '/spec/eventSources',
            value: {
              webhook: { port: esWebhookPort, authTokenSecret: esWebhookAuth || undefined },
              snmp: snmpEnabled ? {
                enabled: true,
                host: snmpHost,
                port: snmpPort,
                community: snmpCommunity,
                version: snmpVersion,
                pollingIntervalSeconds: snmpInterval,
              } : { enabled: false },
              ipmi: ipmiEnabled ? {
                enabled: true,
                host: ipmiHost,
                port: ipmiPort,
                protocol: ipmiProtocol,
                authSecret: ipmiAuthSecret,
                pollingIntervalSeconds: ipmiInterval,
                tlsInsecure: ipmiTlsInsecure,
              } : { enabled: false },
              nut: nutEnabled ? {
                enabled: true,
                host: nutHost,
                port: nutPort,
                upsName: nutUpsName,
                pollingIntervalSeconds: nutInterval,
                authSecret: nutAuthSecret || undefined,
              } : { enabled: false },
            },
          },
        ];

        await k8sPatch({
          model: AutomatedProceduresConfigModel,
          resource: apConfig,
          data: eventSourcesPatch,
        });
      }

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
          <Title headingLevel="h3" style={{ marginBottom: 12 }}>Backup Schedule</Title>
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

          <Divider style={{ margin: '24px 0' }} />

          {/* Storage */}
          <Title headingLevel="h3" style={{ marginBottom: 12 }}>Backup Storage</Title>
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

          <Divider style={{ margin: '24px 0' }} />

          {/* Notifications */}
          <Title headingLevel="h3" style={{ marginBottom: 12 }}>Notifications</Title>
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

          <Divider style={{ margin: '24px 0' }} />

          {/* Event Sources */}
          <Title headingLevel="h3" style={{ marginBottom: 12 }}>Event Sources</Title>
          <div style={{ marginBottom: 8, color: '#6a6e73', fontSize: '0.9rem' }}>
            Configure how power and infrastructure events are detected. Multiple sources can be active
            simultaneously -- if multiple sources detect the same failure, only one event is created.
          </div>

          <Grid hasGutter style={{ marginBottom: 16 }}>
            <GridItem span={4}>
              <FormGroup label="Alert Webhook Port" fieldId="es-webhook-port">
                <TextInput
                  id="es-webhook-port"
                  type="number"
                  value={String(esWebhookPort)}
                  onChange={(_e, val) => setEsWebhookPort(Number(val))}
                />
              </FormGroup>
            </GridItem>
            <GridItem span={4}>
              <FormGroup label="Auth Token Secret" fieldId="es-webhook-auth">
                <TextInput
                  id="es-webhook-auth"
                  value={esWebhookAuth}
                  onChange={(_e, val) => setEsWebhookAuth(val)}
                  placeholder="psu-alert-token"
                />
              </FormGroup>
            </GridItem>
          </Grid>

          <ExpandableSection
            toggleText={`SNMP ${snmpEnabled ? '(enabled)' : ''}`}
            isExpanded={snmpExpanded}
            onToggle={(_e, val) => setSnmpExpanded(val)}
            style={{ marginBottom: 8 }}
          >
            <Grid hasGutter>
              <GridItem span={3}>
                <FormGroup label="Enable SNMP" fieldId="snmp-enabled">
                  <Switch id="snmp-enabled" isChecked={snmpEnabled} onChange={(_e, val) => setSnmpEnabled(val)} label="Enabled" labelOff="Disabled" />
                </FormGroup>
              </GridItem>
              <GridItem span={4}>
                <FormGroup label="UPS/PDU Host" fieldId="snmp-host">
                  <TextInput id="snmp-host" value={snmpHost} onChange={(_e, val) => setSnmpHost(val)} placeholder="192.168.1.100" isDisabled={!snmpEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={2}>
                <FormGroup label="Port" fieldId="snmp-port">
                  <TextInput id="snmp-port" type="number" value={String(snmpPort)} onChange={(_e, val) => setSnmpPort(Number(val))} isDisabled={!snmpEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="Community" fieldId="snmp-community">
                  <TextInput id="snmp-community" value={snmpCommunity} onChange={(_e, val) => setSnmpCommunity(val)} isDisabled={!snmpEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="Version" fieldId="snmp-version">
                  <FormSelect id="snmp-version" value={snmpVersion} onChange={(_e, val) => setSnmpVersion(val)} isDisabled={!snmpEnabled}>
                    <FormSelectOption value="v2c" label="v2c" />
                    <FormSelectOption value="v3" label="v3" />
                  </FormSelect>
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="Poll Interval (sec)" fieldId="snmp-interval">
                  <TextInput id="snmp-interval" type="number" value={String(snmpInterval)} onChange={(_e, val) => setSnmpInterval(Number(val))} isDisabled={!snmpEnabled} />
                </FormGroup>
              </GridItem>
            </Grid>
          </ExpandableSection>

          <ExpandableSection
            toggleText={`IPMI / Redfish ${ipmiEnabled ? '(enabled)' : ''}`}
            isExpanded={ipmiExpanded}
            onToggle={(_e, val) => setIpmiExpanded(val)}
            style={{ marginBottom: 8 }}
          >
            <Grid hasGutter>
              <GridItem span={3}>
                <FormGroup label="Enable IPMI" fieldId="ipmi-enabled">
                  <Switch id="ipmi-enabled" isChecked={ipmiEnabled} onChange={(_e, val) => setIpmiEnabled(val)} label="Enabled" labelOff="Disabled" />
                </FormGroup>
              </GridItem>
              <GridItem span={4}>
                <FormGroup label="BMC Host" fieldId="ipmi-host">
                  <TextInput id="ipmi-host" value={ipmiHost} onChange={(_e, val) => setIpmiHost(val)} placeholder="192.168.1.200" isDisabled={!ipmiEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={2}>
                <FormGroup label="Port" fieldId="ipmi-port">
                  <TextInput id="ipmi-port" type="number" value={String(ipmiPort)} onChange={(_e, val) => setIpmiPort(Number(val))} isDisabled={!ipmiEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="Protocol" fieldId="ipmi-protocol">
                  <FormSelect id="ipmi-protocol" value={ipmiProtocol} onChange={(_e, val) => setIpmiProtocol(val)} isDisabled={!ipmiEnabled}>
                    <FormSelectOption value="redfish" label="Redfish" />
                    <FormSelectOption value="ipmi" label="IPMI" />
                  </FormSelect>
                </FormGroup>
              </GridItem>
              <GridItem span={4}>
                <FormGroup label="Auth Secret" fieldId="ipmi-auth">
                  <TextInput id="ipmi-auth" value={ipmiAuthSecret} onChange={(_e, val) => setIpmiAuthSecret(val)} placeholder="Secret with username, password" isDisabled={!ipmiEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="Poll Interval (sec)" fieldId="ipmi-interval">
                  <TextInput id="ipmi-interval" type="number" value={String(ipmiInterval)} onChange={(_e, val) => setIpmiInterval(Number(val))} isDisabled={!ipmiEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="Skip TLS Verify" fieldId="ipmi-tls">
                  <Switch id="ipmi-tls" isChecked={ipmiTlsInsecure} onChange={(_e, val) => setIpmiTlsInsecure(val)} label="Yes" labelOff="No" isDisabled={!ipmiEnabled} />
                </FormGroup>
              </GridItem>
            </Grid>
          </ExpandableSection>

          <ExpandableSection
            toggleText={`NUT (Network UPS Tools) ${nutEnabled ? '(enabled)' : ''}`}
            isExpanded={nutExpanded}
            onToggle={(_e, val) => setNutExpanded(val)}
            style={{ marginBottom: 16 }}
          >
            <Grid hasGutter>
              <GridItem span={3}>
                <FormGroup label="Enable NUT" fieldId="nut-enabled">
                  <Switch id="nut-enabled" isChecked={nutEnabled} onChange={(_e, val) => setNutEnabled(val)} label="Enabled" labelOff="Disabled" />
                </FormGroup>
              </GridItem>
              <GridItem span={4}>
                <FormGroup label="NUT Server Host" fieldId="nut-host">
                  <TextInput id="nut-host" value={nutHost} onChange={(_e, val) => setNutHost(val)} placeholder="192.168.1.50" isDisabled={!nutEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={2}>
                <FormGroup label="Port" fieldId="nut-port">
                  <TextInput id="nut-port" type="number" value={String(nutPort)} onChange={(_e, val) => setNutPort(Number(val))} isDisabled={!nutEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="UPS Name" fieldId="nut-ups">
                  <TextInput id="nut-ups" value={nutUpsName} onChange={(_e, val) => setNutUpsName(val)} placeholder="ups-1" isDisabled={!nutEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={3}>
                <FormGroup label="Poll Interval (sec)" fieldId="nut-interval">
                  <TextInput id="nut-interval" type="number" value={String(nutInterval)} onChange={(_e, val) => setNutInterval(Number(val))} isDisabled={!nutEnabled} />
                </FormGroup>
              </GridItem>
              <GridItem span={4}>
                <FormGroup label="Auth Secret" fieldId="nut-auth">
                  <TextInput id="nut-auth" value={nutAuthSecret} onChange={(_e, val) => setNutAuthSecret(val)} placeholder="Secret with username, password (optional)" isDisabled={!nutEnabled} />
                </FormGroup>
              </GridItem>
            </Grid>
          </ExpandableSection>

          <Divider style={{ margin: '24px 0' }} />

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
