"""Write plotly.js's own style rules as a stylesheet, for a page whose CSP
blocks the <style> element Plotly would add them to (0294).

Plotly keeps them in src/css/plotcss.js as an object of selector -> rules,
with X standing for ".js-plotly-plot .plotly" and Y for ".plotly-notifier".
This reads that object out of the bundle, as strings only (nothing is run),
and writes the rules as plotcss.js would add them.

usage: plotly_css.py <plotly-basic.min.js> <out.css> <version>
"""
import re
import sys

src = open(sys.argv[1], encoding='utf-8').read()
start = src.index('={"X,X div"') + 1

# Scan the object literal: string-aware, to its closing brace.
depth, j, quote = 0, start, None
while True:
    c = src[j]
    if quote:
        if c == '\\':
            j += 2
            continue
        if c == quote:
            quote = None
    elif c in '"\'':
        quote = c
    elif c == '{':
        depth += 1
    elif c == '}':
        depth -= 1
        if depth == 0:
            break
    j += 1
lit = src[start:j + 1]

# Its entries: "key":"value" or "key":'value', nothing else.
entry = re.compile(r'''\s*("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|[A-Za-z_$][\w$]*)\s*:\s*("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')\s*(,|$)''')
body = lit[1:-1]
pos, rules = 0, []
while pos < len(body):
    m = entry.match(body, pos)
    if not m:
        sys.exit(f'not a plain string entry at {pos}: {body[pos:pos + 80]!r}')
    def unquote(x):
        return x[1:-1].encode().decode('unicode_escape') if x[0] in '"\'' else x
    key, val = unquote(m.group(1)), unquote(m.group(2))
    rules.append((key, val))
    pos = m.end()

# The same rewriting plotcss.js does before adding a rule.
after = src[j + 1:j + 400]
if 'replace(/X/g,".js-plotly-plot .plotly").replace(/Y/g,".plotly-notifier")' not in after:
    sys.exit('the selector rewriting is not where plotcss.js has it')

out = [f'/* plotly.js {sys.argv[3]}: its own style rules (src/css), as it would add them\n'
       '   itself where a page lets it (0294). Copyright Plotly, Inc., MIT licensed. */']
for key, val in rules:
    sel = re.sub(r'^,', ' ,', key).replace('X', '.js-plotly-plot .plotly').replace('Y', '.plotly-notifier')
    out.append(f'{sel}{{{val}}}')
open(sys.argv[2], 'w', encoding='utf-8', newline='\n').write('\n'.join(out) + '\n')
print(len(rules), 'rules')
