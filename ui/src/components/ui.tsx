import { ComponentChildren, JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Icon, IconName } from './Icon';

/* ---------------------------------------------------------------- Button */

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
type ButtonSize = 'sm' | 'md';

const BUTTON_BASE =
  'inline-flex items-center justify-center gap-1.5 font-medium whitespace-nowrap rounded-[var(--radius-control)] ' +
  'transition-colors duration-100 disabled:opacity-50 disabled:pointer-events-none select-none';

const BUTTON_VARIANTS: Record<ButtonVariant, string> = {
  primary: 'bg-brand text-white hover:bg-brand-hover',
  secondary: 'bg-panel text-fg border border-line-strong hover:bg-panel-2',
  ghost: 'text-fg-muted hover:text-fg hover:bg-panel-2',
  danger: 'text-bad border border-bad/30 hover:bg-bad/10',
};

const BUTTON_SIZES: Record<ButtonSize, string> = {
  sm: 'h-7 px-2.5 text-sm',
  md: 'h-9 px-3.5 text-base',
};

export function buttonClass(variant: ButtonVariant = 'secondary', size: ButtonSize = 'md', extra = '') {
  return `${BUTTON_BASE} ${BUTTON_VARIANTS[variant]} ${BUTTON_SIZES[size]} ${extra}`;
}

type ButtonProps = Omit<JSX.ButtonHTMLAttributes<HTMLButtonElement>, 'icon' | 'size' | 'type'> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  icon?: IconName;
  loading?: boolean;
  type?: 'button' | 'submit';
  disabled?: boolean;
};

export function Button({ variant = 'secondary', size = 'md', icon, loading, children, class: cls = '', type = 'button', disabled, ...rest }: ButtonProps) {
  return (
    <button type={type} class={buttonClass(variant, size, cls as string)} disabled={disabled || loading} aria-busy={loading || undefined} {...rest}>
      {loading ? <Spinner size={size === 'sm' ? 12 : 14} /> : icon ? <Icon name={icon} size={size === 'sm' ? 14 : 16} /> : null}
      {children}
    </button>
  );
}

/** A link styled as a button (navigation, not an action). */
export function ButtonLink({ href, variant = 'secondary', size = 'md', icon, children, class: cls = '', external }: {
  href: string; variant?: ButtonVariant; size?: ButtonSize; icon?: IconName; children: ComponentChildren; class?: string; external?: boolean;
}) {
  return (
    <a href={href} class={buttonClass(variant, size, cls)} {...(external ? { target: '_blank', rel: 'noopener noreferrer' } : {})}>
      {icon && <Icon name={icon} size={size === 'sm' ? 14 : 16} />}
      {children}
    </a>
  );
}

export function IconButton({ icon, label, onClick, class: cls = '', size = 16 }: { icon: IconName; label: string; onClick?: (e: MouseEvent) => void; class?: string; size?: number }) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={label}
      aria-label={label}
      class={`inline-flex items-center justify-center w-8 h-8 rounded-[var(--radius-control)] text-fg-muted hover:text-fg hover:bg-panel-2 transition-colors ${cls}`}
    >
      <Icon name={icon} size={size} />
    </button>
  );
}

export function Spinner({ size = 14, class: cls = '' }: { size?: number; class?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" class={`animate-spin shrink-0 ${cls}`} aria-hidden="true">
      <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity="0.25" stroke-width="3" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round" />
    </svg>
  );
}

/* ----------------------------------------------------------------- Forms */

let fieldSeq = 0;
export function useId(prefix = 'f') {
  const ref = useRef<string>();
  if (!ref.current) ref.current = `${prefix}-${++fieldSeq}`;
  return ref.current;
}

export const INPUT_CLASS =
  'w-full h-9 px-3 rounded-[var(--radius-control)] bg-panel border border-line-strong text-fg placeholder:text-fg-faint ' +
  'focus:outline-none focus:border-brand focus:ring-2 focus:ring-brand/25 transition-colors ' +
  'disabled:opacity-60 aria-[invalid=true]:border-bad';

/**
 * Label + control + hint/error. Pass the control as children; give it the
 * id from `useId` so the label is associated.
 */
export function Field({ label, htmlFor, hint, error, optional, children, class: cls = '' }: {
  label: ComponentChildren; htmlFor?: string; hint?: ComponentChildren; error?: string | null; optional?: boolean; children: ComponentChildren; class?: string;
}) {
  return (
    <div class={`flex flex-col gap-1.5 ${cls}`}>
      <label for={htmlFor} class="text-sm font-medium text-fg">
        {label}
        {optional && <span class="font-normal text-fg-faint"> (optional)</span>}
      </label>
      {children}
      {error ? (
        <p class="text-xs text-bad" role="alert">{error}</p>
      ) : hint ? (
        <p class="text-xs text-fg-muted">{hint}</p>
      ) : null}
    </div>
  );
}

type InputProps = Omit<JSX.InputHTMLAttributes<HTMLInputElement>, 'onInput' | 'value'> & {
  value: string | number;
  onValue: (v: string) => void;
  mono?: boolean;
  invalid?: boolean;
};

export function Input({ value, onValue, mono, invalid, class: cls = '', ...rest }: InputProps) {
  return (
    <input
      class={`${INPUT_CLASS} ${mono ? 'font-mono text-[13px]' : ''} ${cls}`}
      value={value}
      aria-invalid={invalid || undefined}
      onInput={(e) => onValue((e.target as HTMLInputElement).value)}
      {...rest}
    />
  );
}

export function PasswordInput({ value, onValue, ...rest }: InputProps) {
  const [shown, setShown] = useState(false);
  return (
    <div class="relative">
      <Input {...rest} value={value} onValue={onValue} type={shown ? 'text' : 'password'} class="pr-10" />
      <button
        type="button"
        class="absolute right-1 top-1/2 -translate-y-1/2 w-7 h-7 inline-flex items-center justify-center rounded text-fg-faint hover:text-fg"
        onClick={() => setShown(!shown)}
        aria-label={shown ? 'Hide password' : 'Show password'}
        title={shown ? 'Hide password' : 'Show password'}
      >
        <Icon name={shown ? 'eyeOff' : 'eye'} size={15} />
      </button>
    </div>
  );
}

export function Textarea({ value, onValue, mono, class: cls = '', ...rest }: Omit<JSX.TextareaHTMLAttributes<HTMLTextAreaElement>, 'onInput' | 'value'> & { value: string; onValue: (v: string) => void; mono?: boolean }) {
  return (
    <textarea
      class={`w-full px-3 py-2 rounded-[var(--radius-control)] bg-panel border border-line-strong text-fg placeholder:text-fg-faint focus:outline-none focus:border-brand focus:ring-2 focus:ring-brand/25 ${mono ? 'font-mono text-[13px] leading-5' : ''} ${cls}`}
      value={value}
      onInput={(e) => onValue((e.target as HTMLTextAreaElement).value)}
      {...rest}
    />
  );
}

export function Select({ value, onValue, options, class: cls = '', ...rest }: Omit<JSX.SelectHTMLAttributes<HTMLSelectElement>, 'onChange' | 'value'> & {
  value: string; onValue: (v: string) => void; options: { value: string; label: string }[];
}) {
  return (
    <div class={`relative ${cls}`}>
      <select
        class={`${INPUT_CLASS} appearance-none pr-8 cursor-pointer`}
        value={value}
        onChange={(e) => onValue((e.target as HTMLSelectElement).value)}
        {...rest}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>{o.label}</option>
        ))}
      </select>
      <Icon name="chevronDown" size={14} class="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-fg-faint" />
    </div>
  );
}

/** Accessible on/off switch with a label and optional description. */
export function Switch({ checked, onChange, label, description, disabled }: {
  checked: boolean; onChange: (v: boolean) => void; label: ComponentChildren; description?: ComponentChildren; disabled?: boolean;
}) {
  const id = useId('sw');
  return (
    <div class="flex items-start justify-between gap-4">
      <div class="min-w-0">
        <label for={id} class="text-sm font-medium text-fg cursor-pointer">{label}</label>
        {description && <p class="text-xs text-fg-muted mt-0.5">{description}</p>}
      </div>
      <button
        id={id}
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        class={`relative shrink-0 w-9 h-5 rounded-full transition-colors disabled:opacity-50 ${checked ? 'bg-brand' : 'bg-line-strong'}`}
      >
        <span class={`absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform ${checked ? 'translate-x-4' : ''}`} />
      </button>
    </div>
  );
}

/** Small mutually-exclusive choice, e.g. "Fields | Connection string". */
export function Segmented<T extends string>({ value, onChange, options, label }: {
  value: T; onChange: (v: T) => void; options: { value: T; label: string }[]; label: string;
}) {
  return (
    <div role="radiogroup" aria-label={label} class="inline-flex p-0.5 rounded-[8px] bg-panel-2 border border-line">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          onClick={() => onChange(o.value)}
          class={`h-7 px-3 rounded-[6px] text-sm font-medium transition-colors ${
            value === o.value ? 'bg-panel text-fg shadow-sm ring-1 ring-line' : 'text-fg-muted hover:text-fg'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

/* ---------------------------------------------------------------- Layout */

export function PageHeader({ title, description, actions }: { title: string; description?: ComponentChildren; actions?: ComponentChildren }) {
  return (
    <div class="flex flex-wrap items-end justify-between gap-x-6 gap-y-3 mb-6">
      <div class="min-w-0">
        <h1 class="text-xl font-semibold tracking-[-0.01em] text-fg">{title}</h1>
        {description && <p class="text-base text-fg-muted mt-1 max-w-[68ch]">{description}</p>}
      </div>
      {actions && <div class="flex items-center gap-2">{actions}</div>}
    </div>
  );
}

export function Panel({ title, description, actions, children, class: cls = '', bodyClass = '' }: {
  title?: ComponentChildren; description?: ComponentChildren; actions?: ComponentChildren; children: ComponentChildren; class?: string; bodyClass?: string;
}) {
  return (
    <section class={`bg-panel border border-line rounded-[var(--radius-panel)] ${cls}`}>
      {(title || actions) && (
        <header class="flex items-start justify-between gap-4 px-5 py-3.5 border-b border-line">
          <div class="min-w-0">
            {title && <h2 class="text-base font-semibold text-fg">{title}</h2>}
            {description && <p class="text-sm text-fg-muted mt-0.5">{description}</p>}
          </div>
          {actions && <div class="flex items-center gap-2 shrink-0">{actions}</div>}
        </header>
      )}
      <div class={bodyClass}>{children}</div>
    </section>
  );
}

export function EmptyState({ icon = 'database', title, children, action }: {
  icon?: IconName; title: string; children?: ComponentChildren; action?: ComponentChildren;
}) {
  return (
    <div class="flex flex-col items-start gap-3 px-6 py-10 sm:px-10">
      <span class="inline-flex items-center justify-center w-10 h-10 rounded-[10px] bg-panel-2 border border-line text-fg-muted">
        <Icon name={icon} size={20} />
      </span>
      <div>
        <h3 class="text-lg font-semibold text-fg">{title}</h3>
        {children && <div class="text-base text-fg-muted mt-1 max-w-[60ch]">{children}</div>}
      </div>
      {action && <div class="mt-1 flex flex-wrap gap-2">{action}</div>}
    </div>
  );
}

export type Status = 'ok' | 'warn' | 'bad' | 'idle' | 'live';

const STATUS_COLOR: Record<Status, string> = {
  ok: 'bg-ok',
  warn: 'bg-warn',
  bad: 'bg-bad',
  idle: 'bg-fg-faint',
  live: 'bg-live',
};

export function StatusDot({ status, label }: { status: Status; label?: string }) {
  return (
    <span class="inline-flex items-center gap-1.5 text-sm text-fg-muted">
      <span class={`w-2 h-2 rounded-full ${STATUS_COLOR[status]}`} aria-hidden="true" />
      {label}
    </span>
  );
}

/** Quiet tag for small facts (e.g. "Read-only", "Expired"). */
export function Tag({ children, tone = 'neutral' }: { children: ComponentChildren; tone?: 'neutral' | 'brand' | 'ok' | 'warn' | 'bad' }) {
  const tones = {
    neutral: 'bg-panel-2 text-fg-muted border-line',
    brand: 'bg-brand/10 text-brand-fg border-brand/20',
    ok: 'bg-ok/10 text-ok border-ok/20',
    warn: 'bg-warn/10 text-warn border-warn/25',
    bad: 'bg-bad/10 text-bad border-bad/25',
  };
  return <span class={`inline-flex items-center h-5 px-1.5 rounded text-xs font-medium border ${tones[tone]}`}>{children}</span>;
}

export function Tabs<T extends string>({ value, onChange, tabs, class: cls = '' }: {
  value: T; onChange: (v: T) => void; tabs: { value: T; label: ComponentChildren }[]; class?: string;
}) {
  return (
    <div role="tablist" class={`flex gap-1 border-b border-line ${cls}`}>
      {tabs.map((t) => (
        <button
          key={t.value}
          type="button"
          role="tab"
          aria-selected={value === t.value}
          onClick={() => onChange(t.value)}
          class={`relative h-9 px-3 text-sm font-medium transition-colors ${
            value === t.value ? 'text-fg' : 'text-fg-muted hover:text-fg'
          }`}
        >
          {t.label}
          {value === t.value && <span class="absolute left-2 right-2 -bottom-px h-0.5 rounded-full bg-brand" />}
        </button>
      ))}
    </div>
  );
}

/** Inline callout for errors, warnings and notes inside forms and panels. */
export function Notice({ tone = 'info', title, children }: { tone?: 'info' | 'warn' | 'bad' | 'ok'; title?: ComponentChildren; children?: ComponentChildren }) {
  const styles = {
    info: { box: 'bg-brand/8 border-brand/20', icon: 'info' as IconName, color: 'text-brand-fg' },
    warn: { box: 'bg-warn/8 border-warn/25', icon: 'alert' as IconName, color: 'text-warn' },
    bad: { box: 'bg-bad/8 border-bad/25', icon: 'alert' as IconName, color: 'text-bad' },
    ok: { box: 'bg-ok/8 border-ok/25', icon: 'check' as IconName, color: 'text-ok' },
  }[tone];
  return (
    <div class={`flex gap-2.5 px-3 py-2.5 rounded-[8px] border ${styles.box}`} role={tone === 'bad' ? 'alert' : undefined}>
      <Icon name={styles.icon} size={16} class={`mt-0.5 ${styles.color}`} />
      <div class="min-w-0 text-sm text-fg">
        {title && <p class="font-medium">{title}</p>}
        {children && <div class={`${title ? 'mt-0.5 ' : ''}text-fg-muted break-words`}>{children}</div>}
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ Copy */

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
  }
}

export function CopyButton({ text, label = 'Copy', size = 'sm', variant = 'ghost', class: cls = '' }: {
  text: string; label?: string; size?: ButtonSize; variant?: ButtonVariant; class?: string;
}) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number>();
  useEffect(() => () => window.clearTimeout(timer.current), []);
  return (
    <Button
      size={size}
      variant={variant}
      icon={copied ? 'check' : 'copy'}
      class={`${copied ? '!text-ok' : ''} ${cls}`}
      onClick={async () => {
        if (await copyText(text)) {
          setCopied(true);
          window.clearTimeout(timer.current);
          timer.current = window.setTimeout(() => setCopied(false), 1600);
        }
      }}
    >
      {copied ? 'Copied' : label}
    </Button>
  );
}

/** Skeleton line for loading states. */
export function Skeleton({ class: cls = 'h-4 w-full' }: { class?: string }) {
  return <div class={`rounded bg-panel-2 animate-pulse ${cls}`} />;
}
