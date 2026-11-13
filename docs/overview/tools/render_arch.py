"""Render a docs set (docs/<set>/**/*.md) into one self-contained HTML page for viewing.

Usage (Python >= 3.9, `pip install -r docs/overview/tools/requirements.txt`):

    python3 docs/overview/tools/render_arch.py            # every set in DOC_SETS, defaults below
    python3 render_arch.py <docs/<set> dir> <out.html>    # one set

Defaults: every docs set in DOC_SETS (architecture, requirements), each into its
docs/overview/v1-waap-<set>.html, resolved relative to this script, so it works from
any working directory. A link from one set into another (for example
../../requirements/02-requirements.md#...) becomes a link to the other set's HTML page.
"""
import html
import os
import re
import sys
import urllib.parse

from markdown_it import MarkdownIt

_HERE = os.path.dirname(os.path.abspath(__file__))
_DOCS = os.path.normpath(os.path.join(_HERE, "..", ".."))

# docs set name -> (output file name, page title, sidebar subtitle)
DOC_SETS = {
    "architecture": ("v1-waap-architecture.html", "WAAP Architecture", "arc42 · bản xem từ docs/architecture"),
    "requirements": ("v1-waap-requirements.html", "WAAP Requirements", "bản xem từ docs/requirements"),
    "lab": ("v1-waap-lab.html", "WAAP Lab", "hạ tầng lab · bản xem từ labs/"),
}
# Bộ lab nằm ngoài docs/ (labs/ ở gốc repo) và không theo cấu trúc arc42, nên có thư mục nguồn và thứ tự file riêng.
SET_DIRS = {"lab": os.path.join(_DOCS, "..", "labs")}
LAB_ORDER = [
    "README.md", "PLAN.md", "UPGRADE.md", "PARAMETERS.md", "PLACEMENT.md", "PLACEMENT.full.md", "CHECKLIST.md",
    "infra/README.md", "ansible/README.md",
]

SRC = OUT = None


def collect():
    if os.path.basename(os.path.normpath(SRC)) == "labs":
        return [f for f in LAB_ORDER if os.path.exists(os.path.join(SRC, f))]
    files = []
    for root, _, names in os.walk(SRC):
        for n in names:
            if n.endswith(".md") and n != "README.md":
                files.append(os.path.relpath(os.path.join(root, n), SRC))

    def key(p):
        parts = p.split(os.sep)
        if p == "index.md":
            return (0,)
        # 00-index first inside each folder, level-3 after its parent's files
        return (1,) + tuple((0 if x == "00-index.md" else 1, x) for x in parts)

    return sorted(files, key=key)


def file_id(path):
    return "f-" + re.sub(r"[^a-z0-9]+", "-", path.lower().removesuffix(".md")).strip("-")


def gh_slug(text):
    s = text.strip().lower()
    s = "".join(ch for ch in s if ch.isalnum() or ch in " -_")
    return s.replace(" ", "-")


md = MarkdownIt("commonmark", {"html": False}).enable("table").enable("strikethrough")


def render_fence(self, tokens, idx, options, env):
    t = tokens[idx]
    lang = (t.info or "").strip().split(" ")[0]
    body = html.escape(t.content)
    if lang == "mermaid":
        return f'<div class="diagram"><pre class="mermaid">{body}</pre></div>\n'
    cls = f' class="lang-{html.escape(lang)}"' if lang else ""
    return f"<pre class=\"code\"><code{cls}>{body}</code></pre>\n"


md.add_render_rule("fence", render_fence)


def resolve_link(href, cur):
    if re.match(r"^[a-z]+:", href):
        return href, True
    path, _, frag = href.partition("#")
    path, frag = urllib.parse.unquote(path), urllib.parse.unquote(frag)
    if not path:
        target = cur
    else:
        target = os.path.normpath(os.path.join(os.path.dirname(cur), path))
        if target.endswith(os.sep) or os.path.isdir(os.path.join(SRC, target)):
            target = os.path.join(target, "00-index.md")
    if not target.endswith(".md"):
        return href, True
    if target.startswith(".." + os.sep):
        # Link into another docs set: point at that set's rendered page.
        other = os.path.normpath(os.path.join(os.path.abspath(SRC), target))
        rel = os.path.relpath(other, _DOCS)
        name, _, inner = rel.partition(os.sep)
        if name in DOC_SETS and inner:
            if os.path.isdir(os.path.join(_DOCS, name, inner)):
                inner = os.path.join(inner, "00-index.md")
            fid = file_id(inner)
            page = DOC_SETS[name][0]
            return (f"{page}#{fid}--{frag}" if frag else f"{page}#{fid}"), True
        return href, True
    fid = file_id(target)
    return (f"#{fid}--{frag}" if frag else f"#{fid}"), False


def render_file(path):
    text = open(os.path.join(SRC, path), encoding="utf-8").read()
    tokens = md.parse(text)
    fid = file_id(path)
    title = path
    for i, t in enumerate(tokens):
        if t.type == "heading_open":
            inline = tokens[i + 1]
            label = inline.content
            if t.tag == "h1" and title == path:
                title = label.replace("`", "")
            t.attrSet("id", f"{fid}--{gh_slug(label)}")
        if t.type == "inline" and t.children:
            for c in t.children:
                if c.type == "link_open":
                    new, external = resolve_link(c.attrGet("href"), path)
                    c.attrSet("href", new)
                    if external:
                        c.attrSet("target", "_blank")
                        c.attrSet("rel", "noopener")
    body = md.renderer.render(tokens, md.options, {})
    return fid, title, body


def render_set(src, out, title_text, brand, brand_sub):
    global SRC, OUT
    SRC, OUT = src, out
    files = collect()
    sections, nav = [], []
    for p in files:
        fid, title, body = render_file(p)
        group = p.split(os.sep)[0] if os.sep in p else ""
        depth = p.count(os.sep)
        sections.append(
            f'<section class="doc" id="{fid}"><div class="path">{html.escape(p)}</div>{body}</section>'
        )
        is_folder_index = p.endswith("00-index.md")
        cls = "top" if (depth <= 1 and is_folder_index) or p == "index.md" else "sub"
        if os.path.basename(os.path.normpath(SRC)) == "labs":
            cls = "top" if depth == 0 else "sub"
        if "08-components" in p and not p.endswith("08-components/00-index.md"):
            cls = "sub2"
        nav.append(f'<a class="{cls}" href="#{fid}">{html.escape(title)}</a>')

    template = open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "arch_template.html"), encoding="utf-8").read()
    out = (
        template.replace("{{NAV}}", "\n".join(nav))
        .replace("{{CONTENT}}", "\n".join(sections))
        .replace("{{TITLE}}", html.escape(title_text))
        .replace("{{BRAND}}", html.escape(brand))
        .replace("{{BRAND_SUB}}", html.escape(brand_sub))
    )
    open(OUT, "w", encoding="utf-8").write(out)
    print(f"{len(files)} files -> {OUT} ({len(out)//1024} KB)")


if len(sys.argv) == 3:
    src, out = sys.argv[1], sys.argv[2]
    name = {"labs": "lab"}.get(os.path.basename(os.path.normpath(src)), os.path.basename(os.path.normpath(src)))
    _, t, sub = DOC_SETS.get(name, (None, "WAAP Docs", f"bản xem từ {src}"))
    render_set(src, out, t, t, sub)
elif len(sys.argv) == 1:
    for name, (page, t, sub) in DOC_SETS.items():
        render_set(SET_DIRS.get(name, os.path.join(_DOCS, name)), os.path.join(_HERE, "..", page), t, t, sub)
else:
    sys.exit(__doc__)
