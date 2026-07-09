export const last = (a) => (a && a.length ? a[a.length - 1] : undefined);
export const head = (a) => (a && a.length ? a[0] : undefined);
export const findLastIndex = (a, pred) => { for (let i = a.length - 1; i >= 0; i--) if (pred(a[i], i, a)) return i; return -1; };
export const flatMap = (a, fn) => a.map(fn).flat();
export const flatten = (a) => a.flat();
export const isEmpty = (v) => {
  if (v == null) return true;
  if (Array.isArray(v) || typeof v === 'string') return v.length === 0;
  if (v instanceof Map || v instanceof Set) return v.size === 0;
  if (typeof v === 'object') return Object.keys(v).length === 0;
  return true;
};
export const zip = (...as) => {
  const len = Math.max(...as.map((a) => a.length), 0);
  return Array.from({ length: len }, (_, i) => as.map((a) => a[i]));
};
export const forIn = (obj, fn) => { for (const k in obj) fn(obj[k], k, obj); };
export const partition = (a, pred) => {
  const t = [], f = [];
  for (const x of a) (pred(x) ? t : f).push(x);
  return [t, f];
};
export const uniq = (a) => [...new Set(a)];
export const uniqBy = (a, fn) => {
  const seen = new Set(), out = [];
  const key = typeof fn === 'function' ? fn : (x) => x[fn];
  for (const x of a) { const k = key(x); if (!seen.has(k)) { seen.add(k); out.push(x); } }
  return out;
};
export const compact = (a) => a.filter(Boolean);
export const difference = (a, ...rest) => { const ex = new Set(rest.flat()); return a.filter((x) => !ex.has(x)); };
export const filter = (a, pred) => {
  if (Array.isArray(a)) return a.filter(pred);
  if (a == null) return [];
  return Object.values(a).filter(pred);
};
export const groupBy = (a, fn) => {
  const key = typeof fn === 'function' ? fn : (x) => x[fn];
  const out = {};
  for (const x of a) { const k = key(x); (out[k] ||= []).push(x); }
  return out;
};
export const keyBy = (a, fn) => {
  const key = typeof fn === 'function' ? fn : (x) => x[fn];
  const out = {};
  for (const x of a) out[key(x)] = x;
  return out;
};
