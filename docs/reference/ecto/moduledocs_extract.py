#!/usr/bin/env python3
"""Extract Elixir module documentation into Markdown, verbatim.

Usage: moduledocs_extract.py FILE.ex [FILE.ex ...]

For each source file, every `@moduledoc`, `@doc` and `@typedoc` heredoc is
copied out unmodified (dedented by the closing-delimiter indentation, per
Elixir heredoc semantics) into <module.name>.md in the current directory,
named after the file's title module — the defmodule/defprotocol whose name
matches the filename (query.ex also defines Ecto.SubQuery; the title is
Ecto.Query). Docs attached by attribute indirection (a heredoc assigned to
`@query_doc`, then `@doc @query_doc`, as in Ecto.Adapters.SQL) are
resolved. The only
added text is a provenance comment, the title heading, one heading per
documented function/callback/type carrying its signature, and — in files
where several modules contribute docs — a `## defmodule X` heading whenever
the owning module changes, so docs that follow a nested module (Ecto.
Migration's API after its Command struct) stay attributed to their real
module. `@doc false` entries are skipped, matching what ExDoc publishes.

Run by fetch.sh; prints one stats line per file so a refresh that suddenly
extracts far fewer entries is visible.
"""
import os.path
import re
import sys

MODULE_RE = re.compile(
    r'^(\s*)(?:defmodule|defprotocol)\s+([A-Z][A-Za-z0-9_.]*)\s+do\s*$')
END_RE = re.compile(r'^(\s*)end\b')
HEREDOC_RE = re.compile(
    r"^(\s*)@(moduledoc|doc|typedoc)\s+~?[sS]?(\"\"\"|''')\s*$")
ONELINE_RE = re.compile(r'^\s*@(moduledoc|doc|typedoc)\s+~?[sS]?"(.+)"\s*$')
ATTR_HEREDOC_RE = re.compile(r"^(\s*)@([a-z_]\w*)\s+~?[sS]?(\"\"\"|''')\s*$")
DOCREF_RE = re.compile(r'^\s*@doc\s+@([a-z_]\w*)\s*$')
SIG_RE = re.compile(
    r'^\s*(def|defmacro|defguard|defdelegate|defstruct|@callback'
    r'|@macrocallback|@type|@typep|@opaque)\s'
)


def heredoc_read(lines, start, delim):
    """Return (dedented content, index after the closing delimiter).

    The closer must match the opener — with_cte/3 in query.ex is a ~S'''
    heredoc whose body may legitimately contain triple double-quotes.
    """
    close = re.compile(rf'^(\s*){delim}\s*$')
    for i in range(start, len(lines)):
        m = close.match(lines[i])
        if m:
            indent = len(m.group(1))
            body = [l[indent:] if l[:indent].isspace() else l
                    for l in lines[start:i]]
            return "\n".join(l.rstrip() for l in body).strip("\n"), i + 1
    sys.exit(f"unterminated heredoc opened on line {start}")


def signature_find(lines, start):
    """Return the signature line(s) following a @doc/@typedoc block.

    Multiline signatures (unbalanced brackets, a line ending in `::`, `|`
    or `,`, or a union type wrapping onto a `|`-led next line) are joined,
    then capped so headings stay readable.
    """
    for i in range(start, min(start + 30, len(lines))):
        if not SIG_RE.match(lines[i]):
            continue
        sig = lines[i].strip()
        for _ in range(15):
            open_ = sum(sig.count(o) - sig.count(c) for o, c in
                        [("(", ")"), ("{", "}"), ("[", "]")])
            if i + 1 >= len(lines) or (
                    open_ <= 0 and not sig.endswith(("::", "|", ","))
                    and not lines[i + 1].lstrip().startswith("|")):
                break
            i += 1
            sig += " " + lines[i].strip()
        sig = body_strip(sig)
        sig = re.sub(r'(\s+do|\s+when\s.+)$', "", sig)
        sig = re.sub(r'^(def|defmacro|defguard|defdelegate)\s+', "", sig)
        sig = re.sub(r'\s+', " ", sig)
        return sig if len(sig) <= 110 else sig[:110].rstrip() + " …"
    return None


def body_strip(sig):
    """Cut a one-line `, do:` body — but only outside brackets, so a
    `do: block` that is part of the parameter list (embeds_one, create)
    stays in the signature."""
    depth = 0
    for j, c in enumerate(sig):
        if c in "([{":
            depth += 1
        elif c in ")]}":
            depth -= 1
        elif depth == 0 and sig.startswith(", do:", j):
            return sig[:j]
    return sig


def file_scan(path, lines):
    """One pass over the source: (kind, content, sig, module) doc events.

    Heredocs are consumed wholesale, so `defmodule`/`end` lines inside doc
    examples never affect the module stack, which pairs each defmodule
    with the `end` at its own indentation (gofmt-grade formatting is a
    given upstream) to know which module owns each doc.
    """
    events, attrs, stack, skipped = [], {}, [], 0
    i = 0
    while i < len(lines):
        line = lines[i]
        m = MODULE_RE.match(line)
        if m:
            stack.append((len(m.group(1)), m.group(2)))
            i += 1
            continue
        m = END_RE.match(line)
        if m and stack and len(m.group(1)) == stack[-1][0]:
            stack.pop()
            i += 1
            continue
        owner = stack[-1][1] if stack else "?"
        m = HEREDOC_RE.match(line) or ONELINE_RE.match(line)
        if m:
            kind = m.group(2) if m.re is HEREDOC_RE else m.group(1)
            if m.re is HEREDOC_RE:
                content, i = heredoc_read(lines, i + 1, m.group(3))
            else:
                content, i = m.group(2), i + 1
            sig = None if kind == "moduledoc" else signature_find(lines, i)
            events.append((kind, content, sig, owner))
            continue
        m = DOCREF_RE.match(line)
        if m:
            if m.group(1) not in attrs:
                print(f"warning: {path}:{i + 1}: @doc @{m.group(1)} "
                      f"references no known heredoc", file=sys.stderr)
                i += 1
                continue
            i += 1
            events.append(("doc", attrs[m.group(1)],
                           signature_find(lines, i), owner))
            continue
        m = ATTR_HEREDOC_RE.match(line)
        if m:
            attrs[m.group(2)], i = heredoc_read(lines, i + 1, m.group(3))
            continue
        skipped += 1 if re.match(r'^\s*@(moduledoc|doc)\s+false\s*$',
                                 line) else 0
        i += 1
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
    events, skipped = file_scan(path, open(path).read().splitlines())
    owners = [e[3] for e in events]
    title = title_find(path, list(dict.fromkeys(owners)))
    multi = len(set(owners)) > 1
    out, current, seen = [f"# {title}\n"], None, set()
    for kind, content, sig, owner in events:
        if multi and owner != current:
            cont = " (continued)" if owner in seen else ""
            out.append(f"\n---\n\n## defmodule {owner}{cont}\n")
            current = owner
            seen.add(owner)
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
    print(f"{dest}: {len(events)} doc blocks ({skipped} @doc false skipped)")


for p in sys.argv[1:]:
    file_extract(p)
