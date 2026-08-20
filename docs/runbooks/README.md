# POP Alert Runbooks

This directory contains runbooks for POP Prometheus alerts. Each runbook corresponds to an alert defined in `config/prometheus/rules.yaml` and is linked via the `runbook_url` annotation.

Runbook files follow the naming convention `<alert-name>.md` where the alert name is lowercase and hyphen-separated (e.g., `POPBackupFailed` becomes `pop-backup-failed.md`).
