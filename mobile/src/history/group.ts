import type { HistoryItem } from '../api/queries';

const MONTHS = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC'] as const;

export type DayGroup = { day: string; label: string; items: readonly HistoryItem[] };

/** "2026-10-03" -> "3 OCT". Pure text: no Date, so the device zone cannot move a day (KP). */
function shortDate(day: string): string {
  const month = MONTHS[Number(day.slice(5, 7)) - 1] ?? '';
  return `${Number(day.slice(8, 10))} ${month}`;
}

/** The calendar day before `day` ("2026-10-01" -> "2026-09-30"), by UTC arithmetic on the date alone. */
function previousDay(day: string): string {
  const ms = Date.UTC(Number(day.slice(0, 4)), Number(day.slice(5, 7)) - 1, Number(day.slice(8, 10)) - 1);
  return new Date(ms).toISOString().slice(0, 10);
}

/** `today` is the campaign day the API reports (`today.date`); null when it is not known, so only dates are shown. */
export function dayLabel(day: string, today: string | null): string {
  if (today !== null && day === today) return `TODAY, ${shortDate(day)}`;
  if (today !== null && day === previousDay(today)) return `YESTERDAY, ${shortDate(day)}`;
  return shortDate(day);
}

/** "HH:mm" of `created_at` as sent. */
export function timeOf(createdAt: string): string {
  return createdAt.slice(11, 16);
}

export function groupByDay(items: readonly HistoryItem[], today: string | null): readonly DayGroup[] {
  const groups: { day: string; items: HistoryItem[] }[] = [];
  for (const item of items) {
    const day = item.created_at.slice(0, 10);
    const last = groups.at(-1);
    if (last !== undefined && last.day === day) last.items.push(item);
    else groups.push({ day, items: [item] });
  }
  return groups.map((g) => ({ ...g, label: dayLabel(g.day, today) }));
}
