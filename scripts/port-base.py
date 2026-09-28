"""One-time PostgreSQL port of pinned upstream SQL."""
from pathlib import Path
import re
root = Path(__file__).resolve().parents[1]
for p in (root / 'server/internal/api').glob('*.go'):
    text = p.read_text(encoding='utf-8')
    def convert(m):
        q = m.group(1)
        if not re.search(r'\b(SELECT|INSERT|UPDATE|DELETE)\b', q):
            return m.group(0)
        if 'INSERT OR IGNORE' in q:
            q = q.replace('INSERT OR IGNORE', 'INSERT') + ' ON CONFLICT DO NOTHING'
        n = iter(range(1, 100))
        q = re.sub(r'\?', lambda _: '$' + str(next(n)), q)
        return chr(96) + q + chr(96)
    p.write_text(re.sub(chr(96)+'([^'+chr(96)+']*)'+chr(96), convert, text), encoding='utf-8')
p = root / 'server/migrations/001_init.sql'
p.write_text(p.read_text().replace(' COLLATE NOCASE', ''), encoding='utf-8')
