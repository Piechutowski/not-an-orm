export class URI {
  static parse(s) { return new URI(s); }
  static file(p) { const u = new URI(`file://${p}`); u.path = p; return u; }
  constructor(s) { this.str = String(s); this.path = this.str.replace(/^file:\/\//, ''); this.scheme = 'file'; }
  toString() { return this.str; }
}
