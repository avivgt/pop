import { phaseToColor, phaseToIcon, formatTimestamp, formatAbsoluteTimestamp } from './utils';

describe('phaseToColor', () => {
  it('returns green for completed phases', () => {
    expect(phaseToColor('Completed')).toBe('green');
    expect(phaseToColor('Complete')).toBe('green');
    expect(phaseToColor('Active')).toBe('green');
  });

  it('returns red for Failed', () => {
    expect(phaseToColor('Failed')).toBe('red');
  });

  it('returns blue for active operation phases', () => {
    expect(phaseToColor('Running')).toBe('blue');
    expect(phaseToColor('ShuttingDown')).toBe('blue');
    expect(phaseToColor('BackingUp')).toBe('blue');
    expect(phaseToColor('ApprovingCSRs')).toBe('blue');
    expect(phaseToColor('WaitingForControlPlane')).toBe('blue');
    expect(phaseToColor('WaitingForWorkers')).toBe('blue');
    expect(phaseToColor('VerifyingOperators')).toBe('blue');
  });

  it('returns orange for Suspended', () => {
    expect(phaseToColor('Suspended')).toBe('orange');
  });

  it('returns cyan for Validating', () => {
    expect(phaseToColor('Validating')).toBe('cyan');
  });

  it('returns grey for Pending and unknown phases', () => {
    expect(phaseToColor('Pending')).toBe('grey');
    expect(phaseToColor('SomeUnknownPhase')).toBe('grey');
  });
});

describe('phaseToIcon', () => {
  it('returns distinct icon types for each phase category', () => {
    const completed = phaseToIcon('Completed');
    const failed = phaseToIcon('Failed');
    const running = phaseToIcon('Running');
    const pending = phaseToIcon('Pending');

    expect(completed).not.toBe(failed);
    expect(running).not.toBe(pending);
    expect(failed).not.toBe(pending);
  });

  it('returns the same icon for all completed-family phases', () => {
    expect(phaseToIcon('Completed')).toBe(phaseToIcon('Complete'));
    expect(phaseToIcon('Complete')).toBe(phaseToIcon('Active'));
  });
});

describe('formatTimestamp', () => {
  it('returns dash for undefined input', () => {
    expect(formatTimestamp(undefined)).toBe('-');
  });

  it('returns dash for empty string', () => {
    expect(formatTimestamp('')).toBe('-');
  });

  it('returns dash for invalid date', () => {
    expect(formatTimestamp('not-a-date')).toBe('-');
  });

  it('returns relative time for recent timestamps', () => {
    const tenMinutesAgo = new Date(Date.now() - 10 * 60 * 1000).toISOString();
    expect(formatTimestamp(tenMinutesAgo)).toBe('10 minutes ago');
  });

  it('returns relative time for hours ago', () => {
    const threeHoursAgo = new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString();
    expect(formatTimestamp(threeHoursAgo)).toBe('3 hours ago');
  });

  it('returns relative time for days ago', () => {
    const twoDaysAgo = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString();
    expect(formatTimestamp(twoDaysAgo)).toBe('2 days ago');
  });

  it('returns "a few seconds ago" for very recent timestamps', () => {
    const justNow = new Date(Date.now() - 5000).toISOString();
    expect(formatTimestamp(justNow)).toBe('a few seconds ago');
  });

  it('handles future timestamps', () => {
    const tenMinutesFromNow = new Date(Date.now() + 10 * 60 * 1000).toISOString();
    expect(formatTimestamp(tenMinutesFromNow)).toBe('in 10 minutes');
  });

  it('returns singular form for 1 unit', () => {
    const oneHourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString();
    expect(formatTimestamp(oneHourAgo)).toBe('1 hour ago');
  });
});

describe('formatAbsoluteTimestamp', () => {
  it('returns dash for undefined input', () => {
    expect(formatAbsoluteTimestamp(undefined)).toBe('-');
  });

  it('returns dash for invalid date', () => {
    expect(formatAbsoluteTimestamp('garbage')).toBe('-');
  });

  it('returns a locale string for valid timestamps', () => {
    const ts = '2026-01-15T10:30:00Z';
    const result = formatAbsoluteTimestamp(ts);
    expect(result).not.toBe('-');
    expect(result.length).toBeGreaterThan(0);
  });
});
