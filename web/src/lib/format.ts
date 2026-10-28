const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['second', 60],
  ['minute', 60],
  ['hour', 24],
  ['day', 7],
  ['week', 4.35],
  ['month', 12],
  ['year', Number.POSITIVE_INFINITY],
];

const relative = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });

/** ago renders a timestamp as "3 minutes ago", falling back to the raw value. */
export function ago(timestamp: string | null | undefined): string {
  if (!timestamp) {
    return 'never';
  }

  const parsed = Date.parse(timestamp);
  if (Number.isNaN(parsed)) {
    return timestamp;
  }

  let delta = (parsed - Date.now()) / 1000;

  for (const [unit, span] of units) {
    if (Math.abs(delta) < span) {
      return relative.format(Math.round(delta), unit);
    }

    delta /= span;
  }

  return timestamp;
}

export function at(timestamp: string | null | undefined): string {
  if (!timestamp) {
    return '';
  }

  const parsed = new Date(timestamp);

  return Number.isNaN(parsed.getTime()) ? timestamp : parsed.toLocaleString();
}

export function duration(milliseconds: number): string {
  if (milliseconds < 1000) {
    return `${Math.round(milliseconds)} ms`;
  }

  if (milliseconds < 60_000) {
    return `${(milliseconds / 1000).toFixed(1)} s`;
  }

  const minutes = Math.floor(milliseconds / 60_000);
  const seconds = Math.round((milliseconds % 60_000) / 1000);

  return `${minutes}m ${seconds}s`;
}
