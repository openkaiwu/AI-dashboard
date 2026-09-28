"""Record checksum for the Windows NSIS installer."""
from pathlib import Path
import hashlib
import json
root = Path(__file__).resolve().parents[1]
installer_dir = root / 'artifacts/desktop/installer'
matches = sorted(installer_dir.glob('AIHub-Setup-*.exe'))
assert matches, f'No installer found under {installer_dir}'
target = matches[-1]
meta = {
    'file': target.name,
    'path': str(target.relative_to(root)).replace('\\', '/'),
    'bytes': target.stat().st_size,
    'sha256': hashlib.sha256(target.read_bytes()).hexdigest(),
}
(root / 'artifacts/DESKTOP-INSTALLER-SHA256.json').write_text(json.dumps(meta, indent=2) + '\n')
print(json.dumps(meta))
