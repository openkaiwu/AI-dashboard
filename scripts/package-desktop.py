"""Package the already-built desktop client without user data.

Default (no arguments) reproduces the legacy Windows zip flow:
    artifacts/desktop/AIHub-win32-x64  ->  artifacts/aihub-desktop-windows-x64.zip

Linux/macOS builds from electron-packager are packaged as tar.gz so that
executable permissions survive:
    python scripts/package-desktop.py --platform linux --arch x64
"""
from pathlib import Path
import argparse
import hashlib
import json
import tarfile
import zipfile

platforms={
    'win32': dict(folder='AIHub-win32-x64', entry='AIHub.exe', archive='zip', out='aihub-desktop-windows-x64.zip', manifest='DESKTOP-SHA256.json'),
    'linux': dict(folder='AIHub-linux-x64', entry='AIHub', archive='tar', out='aihub-desktop-linux-x64.tar.gz', manifest='DESKTOP-SHA256-linux.json'),
    'darwin': dict(folder='AIHub-darwin-arm64', entry='AIHub.app', archive='tar', out='aihub-desktop-darwin-arm64.tar.gz', manifest='DESKTOP-SHA256-darwin.json'),
}

parser=argparse.ArgumentParser()
parser.add_argument('--platform', choices=sorted(platforms), default='win32')
args=parser.parse_args()
spec=platforms[args.platform]

root=Path(__file__).resolve().parents[1]
folder=root/'artifacts/desktop'/spec['folder']
assert (folder/spec['entry']).exists(), f"Build the desktop client first (missing {folder/spec['entry']})"
target=root/'artifacts'/spec['out']
top=spec['folder']

def entries():
    for p in sorted(folder.rglob('*')):
        rel=p.relative_to(folder)
        assert not {'.runtime','backups'} & set(rel.parts)
        assert p.suffix not in {'.log','.db'}, p
        yield p, rel

if spec['archive']=='zip':
    with zipfile.ZipFile(target,'w',zipfile.ZIP_DEFLATED,compresslevel=6) as z:
        for p,rel in entries():
            z.write(p, top+'/'+rel.as_posix())
        z.write(root/'docs/DESKTOP_APP_ZH.md', top+'/使用说明.md')
    with zipfile.ZipFile(target) as z:
        assert z.testzip() is None
else:
    with tarfile.open(target,'w:gz') as t:
        for p,rel in entries():
            t.add(p, arcname=f'{top}/{rel.as_posix()}')
        t.add(root/'docs/DESKTOP_APP_ZH.md', arcname=f'{top}/使用说明.md')

meta={'file':target.name,'bytes':target.stat().st_size,'sha256':hashlib.sha256(target.read_bytes()).hexdigest()}
(root/'artifacts'/spec['manifest']).write_text(json.dumps(meta,indent=2)+'\n')
print(json.dumps(meta))
