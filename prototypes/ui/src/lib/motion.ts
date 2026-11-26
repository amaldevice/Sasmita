export const reducedMotion = () =>
  typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;

/** Pure training model for the demo: loss for an epoch at a learning rate. */
export function lossAt(epoch: number, lr: number): number {
  const decay = Math.min(lr, 0.06) * 6;
  const base = 0.06 + 0.94 * Math.exp(-epoch * decay);
  const wobble = lr > 0.06 ? (lr - 0.06) * 9 * Math.abs(Math.sin(epoch * 1.3)) : 0;
  return Math.min(1, base + wobble);
}

export const EPOCHS = 50;
export const curve = (lr: number) => Array.from({ length: EPOCHS + 1 }, (_, e) => lossAt(e, lr));
