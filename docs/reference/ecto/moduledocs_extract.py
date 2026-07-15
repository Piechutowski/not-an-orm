#!/usr/bin/env python3
"""Extract Elixir module documentation into Markdown, verbatim.

Usage: moduledocs_extract.py FILE.ex [FILE.ex ...]

For each source file, every `@moduledoc`, `@doc` and `@typedoc` heredoc is
copied out unmodified (dedented by the closing-delimiter indentation, per
Elixir heredoc semantics) into <module.name>.md in the current directory,
named after the file's title module — the defmodule/defprotocol whose last
segment matches the filename (query.ex also defines Ecto.SubQuery; the
title is Ecto.Query). The only added text is a provenance comment, the
title heading, one heading per documented function/callback/type carrying
its signature, and per-module headings when a file holds several modules.
`@doc false` entries are skipped, matching what ExDoc publishes.

Run by fetch.sh; prints one stats line per file so a refresh that suddenly
extracts far fewer entries is visible.
"""
import os.path
import re
import sys

MODULE_RE = re.compile(
    r'^\s*(?:defmodule|defprotocol)\s+([A-Z][A-Za-z0-9_.]*)\s+do\s*$')
HEREDOC_RE = re.compile(r'^(\s*)@(moduledoc|doc|typedoc)\s+~?[sS]?"""\s*$')
ONELINE_RE = re.compile(r'^\s*@(moduledoc|doc|typedoc)\s+~?[sS]?"(.+)"\s*$')
CLOSE_RE = re.compile(r'^(\s*)"""\s*$')
SIG_RE = re.compile(
    r'^\s*(def|defmacro|defguard|defdelegate|defstruct|@callback'
    r'|@macrocallback|@type|@typep|@opaque)\s'
)


def heredoc_read(lines, start):
    """Return (dedented content, index after closing delimiter)."""
    for i in range(start, len(lines)):
        m = CLOSE_RE.match(lines[i])
        if m:
            indent = len(m.group(1))
            body = [l[indent:] if l[:indent].isspace() else l
                    for l in lines[start:i]]
            return "\n".join(l.rstrip() for l in body).strip("\n"), i + 1
    sys.exit(f"unterminated heredoc opened on line {start}")


def signature_find(lines, start):
    """Return the signature line(s) following a @doc/@typedoc block.

    Multiline signatures (unbalanced brackets, or a line ending in `::`,
    `|` or `,` — callback return types and union types wrap) are joined,
    then capped so headings stay readable.
    """
    for i in range(start, min(start + 30, len(lines))):
        if not SIG_RE.match(lines[i]):
            continue
        sig = lines[i].strip()
        for _ in range(15):
            open_ = sum(sig.count(o) - sig.count(c) for o, c in
                        [("(", ")"), ("{", "}"), ("[", "]")])
            if i + 1 >= len(lines) or (open_ <= 0 and
                                       not sig.endswith(("::", "|", ","))):
                break
            i += 1
            sig += " " + lines[i].strip()
        sig = re.sub(r'\s+(do|when\s.*|,\s*do:.*)$', "", sig)
        sig = re.sub(r'^(def|defmacro|defguard|defdelegate)\s+', "", sig)
        sig = re.sub(r'\s+', " ", sig)
        return sig if len(sig) <= 110 else sig[:110].rstrip() + " …"
    return None


def file_scan(lines):
    """One pass over the source: ('module', name) and doc-block events.

    Heredocs are consumed wholesale, so `defmodule` lines inside doc
    examples never count as modules.
    """
    events, skipped = [], 0
    i = 0
    while i < len(lines):
        m = MODULE_RE.match(lines[i])
        if m:
            events.append(("module", m.group(1)))
            i += 1
            continue
        m = HEREDOC_RE.match(lines[i]) or ONELINE_RE.match(lines[i])
        if not m:
            skipped += 1 if re.match(r'^\s*@(moduledoc|doc)\s+false\s*$',
                                     lines[i]) else 0
            i += 1
            continue
        kind = m.group(2) if m.re is HEREDOC_RE else m.group(1)
        if m.re is HEREDOC_RE:
            content, i = heredoc_read(lines, i + 1)
        else:
            content, i = m.group(2), i + 1
        sig = None if kind == "moduledoc" else signature_find(lines, i)
        events.append((kind, content, sig))
    return events, skipped


def title_find(path, modules):
    """The module matching the filename, else the first.

    Matching is dot-boundary-aware so query.ex prefers Ecto.Query over
    Ecto.SubQuery, and works for Mix tasks whose filenames span segments
    (ecto.migrate.ex -> Mix.Tasks.Ecto.Migrate).
    """
    stem = os.path.basename(path).removesuffix(".ex").replace("_", "")
    if not modules:
        sys.exit(f"{path}: no defmodule/defprotocol found")
    for name in modules:
        if name.lower() == stem or name.lower().endswith("." + stem):
            return name
    print(f"warning: {path}: no module matches filename, "
          f"using {modules[0]}", file=sys.stderr)
    return modules[0]


def file_extract(path):
    events, skipped = file_scan(open(path).read().splitlines())
    modules = [e[1] for e in events if e[0] == "module"]
    title = title_find(path, modules)
    multi = len(modules) > 1
    out, docs = [f"# {title}\n"], 0
    for e in events:
        if e[0] == "module":
            if multi:
                out.append(f"\n---\n\n## defmodule {e[1]}\n")
            continue
        kind, content, sig = e
        docs += 1
        if kind == "moduledoc":
            out.append("\n" + content + "\n")
        else:
            head = f"`{sig}`" if sig else "(unattached doc)"
            level = "###" if multi else "##"
            out.append(f"\n---\n\n{level} {head}\n\n{content}\n")
    dest = title.lower() + ".md"
    src = re.sub(r'.*/(lib/.*)', r'\1', path)
    header = (f"<!-- {title}: extracted verbatim from {src} "
              f"({'ecto_sql' if 'ecto_sql' in path else 'ecto'} repo) "
              f"by fetch.sh. Apache-2.0. -->\n\n")
    with open(dest, "w") as f:
        f.write(header + "\n".join(out).rstrip() + "\n")
    print(f"{dest}: {docs} doc blocks ({skipped} @doc false skipped)")


for p in sys.argv[1:]:
    file_extract(p)
