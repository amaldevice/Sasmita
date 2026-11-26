/** Answer layout formats from DESIGN.md ("Bentuk dan layout"). */
export type AnswerFormat = 'one' | 'twoBelow' | 'sideBySide';
export type ReadingMode = 'chat' | 'baca';

/** Chat defaults to one column; Baca defaults to two columns below. */
export const DEFAULT_FORMATS: Record<ReadingMode, AnswerFormat> = {
  chat: 'one',
  baca: 'twoBelow',
};

/** Every non-reading shell view uses the Chat reading layout. */
export function readingMode(view: string): ReadingMode {
  return view === 'baca' ? 'baca' : 'chat';
}
