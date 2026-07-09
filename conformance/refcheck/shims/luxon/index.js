// Minimal luxon shim covering the fixed set of formats used by
// @dbml/parse records date validation (values.ts).
const MONTHS = { jan: 1, feb: 2, mar: 3, apr: 4, may: 5, jun: 6, jul: 7, aug: 8, sep: 9, oct: 10, nov: 11, dec: 12 };
const OFF = '(Z|[+-]\\d{2}:?\\d{2})';

const FORMAT_RE = {
  'HH:mm:ss': `^(\\d{2}):(\\d{2}):(\\d{2})$`,
  'HH:mm:ssZZ': `^(\\d{2}):(\\d{2}):(\\d{2})${OFF}$`,
  'HH:mm:ss.SSS': `^(\\d{2}):(\\d{2}):(\\d{2})\\.(\\d{3})$`,
  'HH:mm:ss.SSSZZ': `^(\\d{2}):(\\d{2}):(\\d{2})\\.(\\d{3})${OFF}$`,
  'yyyy-MM-dd': `^(\\d{4})-(\\d{2})-(\\d{2})$`,
  'M/d/yyyy': `^(\\d{1,2})\\/(\\d{1,2})\\/(\\d{4})$`,
  'd MMM yyyy': `^(\\d{1,2}) ([A-Za-z]{3}) (\\d{4})$`,
  'MMM d, yyyy': `^([A-Za-z]{3}) (\\d{1,2}), (\\d{4})$`,
  'yyyy-MM-dd HH:mm:ss': `^(\\d{4})-(\\d{2})-(\\d{2}) (\\d{2}):(\\d{2}):(\\d{2})$`,
  'yyyy-MM-dd HH:mm:ss.SSS': `^(\\d{4})-(\\d{2})-(\\d{2}) (\\d{2}):(\\d{2}):(\\d{2})\\.(\\d{3})$`,
  'yyyy-MM-dd HH:mm:ss.SSSZZ': `^(\\d{4})-(\\d{2})-(\\d{2}) (\\d{2}):(\\d{2}):(\\d{2})\\.(\\d{3})${OFF}$`,
  'yyyy-MM-dd HH:mm:ssZZ': `^(\\d{4})-(\\d{2})-(\\d{2}) (\\d{2}):(\\d{2}):(\\d{2})${OFF}$`,
};

const pad = (n, w = 2) => String(n).padStart(w, '0');

class DT {
  constructor(fields) { Object.assign(this, { isValid: false, y: 0, mo: 1, d: 1, h: 0, mi: 0, s: 0, ms: 0, off: null, ...fields }); }
  get zone() { return { type: this.off === null ? 'system' : 'fixed' }; }
  offStr() {
    if (this.off === null || this.off === 0) return this.off === null ? '' : 'Z';
    const sign = this.off < 0 ? '-' : '+';
    const a = Math.abs(this.off);
    return `${sign}${pad(Math.floor(a / 60))}:${pad(a % 60)}`;
  }
  toISOTime({ suppressMilliseconds, includeOffset } = {}) {
    let s = `${pad(this.h)}:${pad(this.mi)}:${pad(this.s)}`;
    if (!(suppressMilliseconds && this.ms === 0)) s += `.${pad(this.ms, 3)}`;
    if (includeOffset) s += this.offStr() || '+00:00';
    return s;
  }
  toISODate() { return `${pad(this.y, 4)}-${pad(this.mo)}-${pad(this.d)}`; }
  toISO({ suppressMilliseconds, includeOffset } = {}) {
    return `${this.toISODate()}T${this.toISOTime({ suppressMilliseconds, includeOffset })}`;
  }
}

function parseOffset(tok) {
  if (tok === undefined) return null;
  if (tok === 'Z') return 0;
  const m = tok.match(/^([+-])(\d{2}):?(\d{2})$/);
  if (!m) return null;
  return (m[1] === '-' ? -1 : 1) * (Number(m[2]) * 60 + Number(m[3]));
}

function checked(f) {
  if (f.mo < 1 || f.mo > 12 || f.d < 1 || f.d > 31 || f.h > 23 || f.mi > 59 || f.s > 59) return new DT({});
  return new DT({ ...f, isValid: true });
}

export const DateTime = {
  fromFormat(value, format) {
    const re = FORMAT_RE[format];
    if (!re) return new DT({});
    const m = String(value).match(new RegExp(re));
    if (!m) return new DT({});
    const g = m.slice(1);
    switch (format) {
      case 'HH:mm:ss': return checked({ h: +g[0], mi: +g[1], s: +g[2] });
      case 'HH:mm:ssZZ': return checked({ h: +g[0], mi: +g[1], s: +g[2], off: parseOffset(g[3]) });
      case 'HH:mm:ss.SSS': return checked({ h: +g[0], mi: +g[1], s: +g[2], ms: +g[3] });
      case 'HH:mm:ss.SSSZZ': return checked({ h: +g[0], mi: +g[1], s: +g[2], ms: +g[3], off: parseOffset(g[4]) });
      case 'yyyy-MM-dd': return checked({ y: +g[0], mo: +g[1], d: +g[2] });
      case 'M/d/yyyy': return checked({ y: +g[2], mo: +g[0], d: +g[1] });
      case 'd MMM yyyy': return checked({ y: +g[2], mo: MONTHS[g[1].toLowerCase()] ?? 0, d: +g[0] });
      case 'MMM d, yyyy': return checked({ y: +g[2], mo: MONTHS[g[0].toLowerCase()] ?? 0, d: +g[1] });
      case 'yyyy-MM-dd HH:mm:ss': return checked({ y: +g[0], mo: +g[1], d: +g[2], h: +g[3], mi: +g[4], s: +g[5] });
      case 'yyyy-MM-dd HH:mm:ss.SSS': return checked({ y: +g[0], mo: +g[1], d: +g[2], h: +g[3], mi: +g[4], s: +g[5], ms: +g[6] });
      case 'yyyy-MM-dd HH:mm:ss.SSSZZ': return checked({ y: +g[0], mo: +g[1], d: +g[2], h: +g[3], mi: +g[4], s: +g[5], ms: +g[6], off: parseOffset(g[7]) });
      case 'yyyy-MM-dd HH:mm:ssZZ': return checked({ y: +g[0], mo: +g[1], d: +g[2], h: +g[3], mi: +g[4], s: +g[5], off: parseOffset(g[6]) });
    }
    return new DT({});
  },
  fromISO(value) {
    const m = String(value).match(
      /^(\d{4})-(\d{2})-(\d{2})(?:[T](\d{2}):(\d{2})(?::(\d{2}))?(?:\.(\d{1,3}))?(Z|[+-]\d{2}:?\d{2})?)?$/,
    );
    if (!m) return new DT({});
    return checked({
      y: +m[1], mo: +m[2], d: +m[3],
      h: +(m[4] ?? 0), mi: +(m[5] ?? 0), s: +(m[6] ?? 0),
      ms: +((m[7] ?? '0').padEnd(3, '0')),
      off: m[8] !== undefined ? parseOffset(m[8]) : null,
    });
  },
};
