import { ComponentChildren } from 'preact';
import { createPortal } from 'preact/compat';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Icon } from './Icon';
import { Button, IconButton } from './ui';

/* Focus management shared by Drawer and Modal: trap Tab inside the dialog,
   close on Escape, restore focus to the opener on close. */
function useDialog(open: boolean, onClose: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement as HTMLElement | null;
    const el = ref.current;
    // Focus the first field, or the dialog itself.
    requestAnimationFrame(() => {
      const first = el?.querySelector<HTMLElement>('[data-autofocus], input:not([type=hidden]):not([disabled]), select, textarea');
      (first || el)?.focus();
    });
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        closeRef.current();
      } else if (e.key === 'Tab' && el) {
        const nodes = el.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])');
        if (!nodes.length) return;
        const first = nodes[0];
        const last = nodes[nodes.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          last.focus();
          e.preventDefault();
        } else if (!e.shiftKey && document.activeElement === last) {
          first.focus();
          e.preventDefault();
        }
      }
    };
    document.addEventListener('keydown', onKey, true);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', onKey, true);
      document.body.style.overflow = prevOverflow;
      opener?.focus?.();
    };
  }, [open]);

  return ref;
}

/** Right-hand sheet for create/edit flows that need room. */
export function Drawer({ open, onClose, title, description, children, footer, width = 'max-w-[600px]' }: {
  open: boolean; onClose: () => void; title: ComponentChildren; description?: ComponentChildren; children: ComponentChildren; footer?: ComponentChildren; width?: string;
}) {
  const ref = useDialog(open, onClose);
  if (!open) return null;
  return createPortal(
    <div class="fixed inset-0 z-50 flex justify-end">
      <div class="absolute inset-0 bg-[var(--overlay)] anim-fade" onClick={onClose} aria-hidden="true" />
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === 'string' ? title : undefined}
        tabIndex={-1}
        class={`relative w-full ${width} h-full flex flex-col bg-panel shadow-[var(--shadow-float)] anim-drawer outline-none sm:rounded-l-[var(--radius-float)]`}
      >
        <header class="flex items-start justify-between gap-4 px-6 pt-5 pb-4 border-b border-line">
          <div class="min-w-0">
            <h2 class="text-lg font-semibold text-fg">{title}</h2>
            {description && <p class="text-sm text-fg-muted mt-0.5">{description}</p>}
          </div>
          <IconButton icon="x" label="Close" onClick={onClose} class="-mr-2 -mt-1" />
        </header>
        <div class="flex-1 overflow-y-auto px-6 py-5">{children}</div>
        {footer && <footer class="flex flex-wrap items-center justify-end gap-2 px-4 sm:px-6 py-3.5 border-t border-line bg-panel">{footer}</footer>}
      </div>
    </div>,
    document.body
  );
}

/** Centered dialog for short, focused tasks. */
export function Modal({ open, onClose, title, children, footer, width = 'max-w-md' }: {
  open: boolean; onClose: () => void; title: ComponentChildren; children: ComponentChildren; footer?: ComponentChildren; width?: string;
}) {
  const ref = useDialog(open, onClose);
  if (!open) return null;
  return createPortal(
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div class="absolute inset-0 bg-[var(--overlay)] anim-fade" onClick={onClose} aria-hidden="true" />
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === 'string' ? title : undefined}
        tabIndex={-1}
        class={`relative w-full ${width} max-h-[90vh] flex flex-col bg-panel rounded-[var(--radius-float)] shadow-[var(--shadow-float)] anim-pop outline-none`}
      >
        <header class="flex items-start justify-between gap-4 px-5 pt-4 pb-3">
          <h2 class="text-lg font-semibold text-fg">{title}</h2>
          <IconButton icon="x" label="Close" onClick={onClose} class="-mr-2 -mt-1" />
        </header>
        <div class="px-5 pb-4 overflow-y-auto">{children}</div>
        {footer && <footer class="flex items-center justify-end gap-2 px-5 py-3 border-t border-line">{footer}</footer>}
      </div>
    </div>,
    document.body
  );
}

/* ----------------------------------------------------- confirm() + toast() */

interface ConfirmRequest {
  title: string;
  body?: ComponentChildren;
  confirmLabel: string;
  danger?: boolean;
  resolve: (ok: boolean) => void;
}

interface ToastItem {
  id: number;
  message: string;
  tone: 'ok' | 'bad' | 'info';
}

type Listener = () => void;
let confirmQueue: ConfirmRequest[] = [];
let toasts: ToastItem[] = [];
let toastSeq = 0;
const listeners = new Set<Listener>();
const emit = () => listeners.forEach((l) => l());

/**
 * Promise-based replacement for window.confirm.
 *   if (await confirm({ title: 'Delete "shop"?', confirmLabel: 'Delete', danger: true })) ...
 */
export function confirm(opts: Omit<ConfirmRequest, 'resolve'>): Promise<boolean> {
  return new Promise((resolve) => {
    confirmQueue = [...confirmQueue, { ...opts, resolve }];
    emit();
  });
}

export function toast(message: string, tone: ToastItem['tone'] = 'ok') {
  const id = ++toastSeq;
  toasts = [...toasts, { id, message, tone }];
  emit();
  window.setTimeout(() => {
    toasts = toasts.filter((t) => t.id !== id);
    emit();
  }, tone === 'bad' ? 6000 : 3200);
}

/** Mount once near the app root. */
export function OverlayHost() {
  const [, force] = useState(0);
  useEffect(() => {
    const l = () => force((n) => n + 1);
    listeners.add(l);
    return () => {
      listeners.delete(l);
    };
  }, []);

  const current = confirmQueue[0];
  const settle = (ok: boolean) => {
    current?.resolve(ok);
    confirmQueue = confirmQueue.slice(1);
    emit();
  };

  return (
    <>
      <Modal
        open={!!current}
        onClose={() => settle(false)}
        title={current?.title || ''}
        footer={
          <>
            <Button variant="ghost" onClick={() => settle(false)}>Cancel</Button>
            <Button variant={current?.danger ? 'danger' : 'primary'} onClick={() => settle(true)} data-autofocus>
              {current?.confirmLabel}
            </Button>
          </>
        }
      >
        {current?.body && <div class="text-base text-fg-muted">{current.body}</div>}
      </Modal>

      <div class="fixed bottom-4 right-4 z-[60] flex flex-col gap-2 items-end pointer-events-none" aria-live="polite">
        {toasts.map((t) => (
          <div
            key={t.id}
            class="pointer-events-auto flex items-center gap-2.5 max-w-sm pl-3 pr-4 py-2.5 rounded-[10px] bg-panel text-sm text-fg shadow-[var(--shadow-float)] anim-pop"
            role={t.tone === 'bad' ? 'alert' : 'status'}
          >
            <span
              class={`inline-flex items-center justify-center w-5 h-5 rounded-full ${
                t.tone === 'ok' ? 'bg-ok/15 text-ok' : t.tone === 'bad' ? 'bg-bad/15 text-bad' : 'bg-brand/15 text-brand-fg'
              }`}
            >
              <Icon name={t.tone === 'ok' ? 'check' : t.tone === 'bad' ? 'alert' : 'info'} size={12} />
            </span>
            {t.message}
          </div>
        ))}
      </div>
    </>
  );
}
