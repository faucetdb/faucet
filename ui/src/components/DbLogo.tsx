import { engineById, engineForDriver } from '../lib/drivers';

/**
 * Database logo on a white tile, so vendor colors read the same in light
 * and dark themes. Pass `engine` (e.g. "mariadb") or `driver`.
 */
export function DbLogo({ engine, driver, size = 32 }: { engine?: string; driver?: string; size?: number }) {
  const e = engine ? engineById(engine) : engineForDriver(driver || '');
  const pad = Math.round(size * (e.wide ? 0.12 : 0.18));
  return (
    <span
      class="inline-flex items-center justify-center shrink-0 bg-white rounded-[7px] ring-1 ring-black/5"
      style={{ width: `${size}px`, height: `${size}px`, padding: `${pad}px` }}
    >
      <img src={e.logo} alt="" class="max-w-full max-h-full object-contain" draggable={false} />
    </span>
  );
}
