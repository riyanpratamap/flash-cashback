export const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'] as const;

/** "2026-10-03T14:32:00+07:00" -> "3 Oct, 14:32". Pure text, no Date: the device zone cannot move it (KP). */
export function formatStamp(createdAt: string): string {
  const month = MONTHS[Number(createdAt.slice(5, 7)) - 1] ?? '';
  return `${Number(createdAt.slice(8, 10))} ${month}, ${createdAt.slice(11, 16)}`;
}

const two = (n: number) => String(n).padStart(2, '0');

/**
 * A time the device recorded itself (a saved attempt), shown in the device zone, which is what its owner expects.
 * Times from the API use `formatStamp` instead.
 */
export function formatDeviceStamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  // The device zone enters only through its offset at that moment; the wall clock is then read off in UTC fields.
  const wall = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return `${wall.getUTCDate()} ${MONTHS[wall.getUTCMonth()] ?? ''}, ${two(wall.getUTCHours())}:${two(wall.getUTCMinutes())}`;
}
