import React from 'react';
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  InProgressIcon,
  PendingIcon,
  PauseCircleIcon,
  SyncAltIcon,
  OutlinedClockIcon,
} from '@patternfly/react-icons';

/**
 * Maps a resource phase string to a PatternFly Label color.
 */
export function phaseToColor(
  phase: string,
): 'green' | 'red' | 'blue' | 'grey' | 'orange' | 'cyan' {
  switch (phase) {
    case 'Completed':
    case 'Complete':
    case 'Active':
      return 'green';
    case 'Failed':
      return 'red';
    case 'Running':
    case 'ShuttingDown':
    case 'BackingUp':
    case 'ApprovingCSRs':
    case 'WaitingForControlPlane':
    case 'WaitingForWorkers':
    case 'VerifyingOperators':
      return 'blue';
    case 'Suspended':
      return 'orange';
    case 'Validating':
      return 'cyan';
    case 'Pending':
    default:
      return 'grey';
  }
}

/**
 * Maps a resource phase string to a PatternFly icon component.
 */
export function phaseToIcon(phase: string): React.ComponentType {
  switch (phase) {
    case 'Completed':
    case 'Complete':
    case 'Active':
      return CheckCircleIcon;
    case 'Failed':
      return ExclamationCircleIcon;
    case 'Running':
    case 'ShuttingDown':
    case 'BackingUp':
      return InProgressIcon;
    case 'ApprovingCSRs':
    case 'WaitingForControlPlane':
    case 'WaitingForWorkers':
    case 'VerifyingOperators':
      return SyncAltIcon;
    case 'Suspended':
      return PauseCircleIcon;
    case 'Validating':
      return OutlinedClockIcon;
    case 'Pending':
    default:
      return PendingIcon;
  }
}

/**
 * Formats a Kubernetes timestamp string into a human-readable relative time.
 */
export function formatTimestamp(ts: string | undefined): string {
  if (!ts) {
    return '-';
  }

  const date = new Date(ts);
  if (isNaN(date.getTime())) {
    return '-';
  }

  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSeconds = Math.floor(Math.abs(diffMs) / 1000);
  const isFuture = diffMs < 0;

  if (diffSeconds < 60) {
    return isFuture ? 'in a few seconds' : 'a few seconds ago';
  }

  const diffMinutes = Math.floor(diffSeconds / 60);
  if (diffMinutes < 60) {
    const label = diffMinutes === 1 ? 'minute' : 'minutes';
    return isFuture ? `in ${diffMinutes} ${label}` : `${diffMinutes} ${label} ago`;
  }

  const diffHours = Math.floor(diffMinutes / 60);
  if (diffHours < 24) {
    const label = diffHours === 1 ? 'hour' : 'hours';
    return isFuture ? `in ${diffHours} ${label}` : `${diffHours} ${label} ago`;
  }

  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 30) {
    const label = diffDays === 1 ? 'day' : 'days';
    return isFuture ? `in ${diffDays} ${label}` : `${diffDays} ${label} ago`;
  }

  const diffMonths = Math.floor(diffDays / 30);
  if (diffMonths < 12) {
    const label = diffMonths === 1 ? 'month' : 'months';
    return isFuture ? `in ${diffMonths} ${label}` : `${diffMonths} ${label} ago`;
  }

  const diffYears = Math.floor(diffMonths / 12);
  const label = diffYears === 1 ? 'year' : 'years';
  return isFuture ? `in ${diffYears} ${label}` : `${diffYears} ${label} ago`;
}

/**
 * Formats a timestamp as an absolute date-time string.
 */
export function formatAbsoluteTimestamp(ts: string | undefined): string {
  if (!ts) {
    return '-';
  }
  const date = new Date(ts);
  if (isNaN(date.getTime())) {
    return '-';
  }
  return date.toLocaleString();
}
