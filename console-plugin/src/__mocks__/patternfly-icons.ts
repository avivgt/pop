const makeMock = (name: string) => {
  const fn = () => null;
  fn.displayName = name;
  return fn;
};

export const CheckCircleIcon = makeMock('CheckCircleIcon');
export const ExclamationCircleIcon = makeMock('ExclamationCircleIcon');
export const InProgressIcon = makeMock('InProgressIcon');
export const PendingIcon = makeMock('PendingIcon');
export const PauseCircleIcon = makeMock('PauseCircleIcon');
export const SyncAltIcon = makeMock('SyncAltIcon');
export const OutlinedClockIcon = makeMock('OutlinedClockIcon');
