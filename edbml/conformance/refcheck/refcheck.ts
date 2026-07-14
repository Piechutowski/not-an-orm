// Run conformance snippets through the reference @dbml/parse compiler
// (source extracted from git history) and compare verdicts.
import { readdirSync, readFileSync } from 'node:fs';
import { join, basename } from 'node:path';
import Compiler from './src/compiler';
import { MemoryProjectLayout } from './src/compiler/projectLayout/layout';
import { DEFAULT_ENTRY } from './src/constants';

const snippetsRoot = process.argv[2];
const SKIP = new Set([
  '28_modules.dbml', // imports resolve against other files; needs a multi-file layout
  'i30_bad_import_kind.dbml',
  'i31_import_missing_from.dbml',
]);

let checked = 0, skipped = 0, disagreements = 0;
for (const [dir, wantError] of [['valid', false], ['invalid', true]] as const) {
  for (const f of readdirSync(join(snippetsRoot, dir)).sort()) {
    if (!f.endsWith('.dbml')) continue;
    if (SKIP.has(f)) { skipped++; console.log(`SKIP  ${dir}/${f}`); continue; }
    const src = readFileSync(join(snippetsRoot, dir, f), 'utf8');
    let errors: any[] = [];
    let crash: unknown = null;
    try {
      const layout = new MemoryProjectLayout();
      layout.setSource(DEFAULT_ENTRY, src);
      const compiler = new Compiler(layout);
      const report = compiler.interpretFile(DEFAULT_ENTRY);
      errors = report.getErrors();
    } catch (e) {
      crash = e;
    }
    checked++;
    const gotError = crash !== null || errors.length > 0;
    if (gotError === wantError) {
      console.log(`AGREE ${dir}/${f}`);
    } else {
      disagreements++;
      const detail = crash ? `crash: ${crash}` : errors[0]?.message ?? '';
      console.log(`DIFF  ${dir}/${f}: spec says ${wantError ? 'invalid' : 'valid'}, reference ${gotError ? 'rejected' : 'accepted'}  ${detail}`);
    }
  }
}
console.log(`\n${checked} checked, ${skipped} skipped, ${disagreements} disagreements`);
