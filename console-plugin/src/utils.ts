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
): 'green' | 'red' | 'blue' | 'grey' | 'orange' | 'teal' {
  switch (phase) {
    case 'Completed':
    case 'Complete':
    case 'Active':
      return 'green';
    case 'Failed':
    case 'PartiallyFailed':
      return 'red';
    case 'Running':
    case 'ShuttingDown':
    case 'BackingUp':
    case 'ApprovingCSRs':
    case 'WaitingForControlPlane':
    case 'WaitingForWorkers':
    case 'VerifyingOperators':
    case 'InProgress':
      return 'blue';
    case 'Suspended':
    case 'TimedOut':
      return 'orange';
    case 'Validating':
      return 'teal';
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
    case 'InProgress':
      return InProgressIcon;
    case 'ApprovingCSRs':
    case 'WaitingForControlPlane':
    case 'WaitingForWorkers':
    case 'VerifyingOperators':
      return SyncAltIcon;
    case 'Suspended':
    case 'TimedOut':
      return PauseCircleIcon;
    case 'PartiallyFailed':
      return ExclamationCircleIcon;
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

import { ManagedClusterClaim } from './types';

/**
 * Extracts a ClusterClaim value by name from a list of claims.
 */
export function getClusterClaim(claims: ManagedClusterClaim[] | undefined, name: string): string {
  return claims?.find((c) => c.name === name)?.value ?? '';
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
