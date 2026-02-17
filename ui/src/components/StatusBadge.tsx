type Status = 'healthy' | 'degraded' | 'error' | 'unknown' | 'active' | 'inactive';

interface StatusBadgeProps {
  status: Status;
  label?: string;
}

const statusConfig: Record<Status, { color: string; bg: string; dot: string }> = {
  healthy: { color: 'text-success', bg: 'bg-success/10', dot: 'bg-success' },
  active: { color: 'text-success', bg: 'bg-success/10', dot: 'bg-success' },
  degraded: { color: 'text-warning', bg: 'bg-warning/10', dot: 'bg-warning' },
  error: { color: 'text-error', bg: 'bg-error/10', dot: 'bg-error' },
  inactive: { color: 'text-text-muted', bg: 'bg-surface-overlay', dot: 'bg-text-muted' },
  unknown: { color: 'text-text-muted', bg: 'bg-surface-overlay', dot: 'bg-text-muted' },
};

export function StatusBadge({ status, label }: StatusBadgeProps) {
  const config = statusConfig[status] || statusConfig.unknown;
  const displayLabel = label || status.charAt(0).toUpperCase() + status.slice(1);

  return (
    <span class={`badge ${config.bg} ${config.color}`}>
      <span class={`w-1.5 h-1.5 rounded-full ${config.dot} mr-1.5`} />
      {displayLabel}
    </span>
  );
}
