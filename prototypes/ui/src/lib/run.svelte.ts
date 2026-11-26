import { reducedMotion } from './motion';

/**
 * Scripted answer for the prototype. Stages:
 * 0 idle · 1 understanding · 2 building visual · 3 visual shown · 4 streaming text
 * 5 done · 6 follow-up asked, approval waiting · 7 approved · 8 denied
 */
export class AnswerRun {
  stage = $state(0);
  words = $state(0);
  seconds = $state(0);
  #timers: ReturnType<typeof setTimeout>[] = [];
  #totalWords = 0;

  constructor(totalWords: number) {
    this.#totalWords = totalWords;
  }

  start() {
    this.#clear();
    this.stage = 0; this.words = 0; this.seconds = 0;
    if (reducedMotion()) { this.stage = 6; this.words = this.#totalWords; this.seconds = 9; return; }
    const at = (ms: number, fn: () => void) => this.#timers.push(setTimeout(fn, ms));
    at(200, () => (this.stage = 1));
    at(1100, () => (this.stage = 2));
    at(2600, () => (this.stage = 3));
    at(3400, () => { this.stage = 4; this.#stream(); });
    const tick = setInterval(() => { if (this.stage < 5) this.seconds += 1; else clearInterval(tick); }, 1000);
    this.#timers.push(tick as unknown as ReturnType<typeof setTimeout>);
  }

  #stream() {
    const step = () => {
      if (this.words >= this.#totalWords) {
        this.stage = 5;
        this.#timers.push(setTimeout(() => (this.stage = 6), 2200));
        return;
      }
      this.words += 2;
      this.#timers.push(setTimeout(step, 45));
    };
    step();
  }

  approve() { this.stage = 7; }
  deny() { this.stage = 8; }
  #clear() { this.#timers.forEach((t) => clearTimeout(t)); this.#timers = []; }
  destroy() { this.#clear(); }
}

export type Token = { w: string; bold: boolean; p: number };
export function tokenize(paras: string[]): Token[] {
  return paras.flatMap((para, p) =>
    para.split(/(\*\*[^*]+\*\*)/).flatMap((chunk) => {
      const bold = chunk.startsWith('**');
      return chunk.replace(/\*\*/g, '').split(/\s+/).filter(Boolean).map((w) => ({ w, bold, p }));
    }),
  );
}
