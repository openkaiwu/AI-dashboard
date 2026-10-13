"""Record checksum for the Windows NSIS installer."""
from pathlib import Path
import hashlib
import json
root = Path(__file__).resolve().parents[1]
installer_dir = root / 'artifacts/desktop/installer'
version = json.loads((root / 'apps/desktop/package.json').read_text(encoding='utf-8'))['version']
target = installer_dir / f'AIHub-正式版v{version}.exe'
assert target.is_file(), f'Current installer not found: {target}'
meta = {
    'file': target.name,
    'path': str(target.relative_to(root)).replace('\\', '/'),
    'bytes': target.stat().st_size,
    'sha256': hashlib.sha256(target.read_bytes()).hexdigest(),
}
(root / 'artifacts/DESKTOP-INSTALLER-SHA256.json').write_text(json.dumps(meta, indent=2) + '\n', encoding='utf-8')
print(json.dumps(meta))
