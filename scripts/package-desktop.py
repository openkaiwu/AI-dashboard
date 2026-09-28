"""Package the already-built Windows desktop client without user data."""
from pathlib import Path
import hashlib
import json
import zipfile
root=Path(__file__).resolve().parents[1]
folder=root/'artifacts/desktop/AIHub-win32-x64'
assert (folder/'AIHub.exe').is_file(), 'Build the desktop client first'
target=root/'artifacts/aihub-desktop-windows-x64.zip'
with zipfile.ZipFile(target,'w',zipfile.ZIP_DEFLATED,compresslevel=6) as z:
    for p in sorted(folder.rglob('*')):
        if p.is_file():
            assert not {'.runtime','backups'} & set(p.relative_to(folder).parts)
            assert p.suffix not in {'.log','.db'}, p
            z.write(p, 'AIHub-win32-x64/'+p.relative_to(folder).as_posix())
    z.write(root/'docs/DESKTOP_APP_ZH.md','AIHub-win32-x64/使用说明.md')
with zipfile.ZipFile(target) as z:
    assert z.testzip() is None
meta={'file':target.name,'bytes':target.stat().st_size,'sha256':hashlib.sha256(target.read_bytes()).hexdigest()}
(root/'artifacts/DESKTOP-SHA256.json').write_text(json.dumps(meta,indent=2)+'\n')
print(json.dumps(meta))
