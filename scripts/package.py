"""Package only allowlisted source/build files. Never include local credentials or databases."""
from pathlib import Path
import hashlib
import json
import zipfile
root=Path(__file__).resolve().parents[1]
artifacts=root/'artifacts'
artifacts.mkdir(exist_ok=True)
excluded={'.git','.runtime','.tools','upstream','node_modules','.dart_tool','build','.gradle','.idea','ephemeral'}
excluded_names={'.env','local.properties','.flutter-plugins','.flutter-plugins-dependencies','bridge.json','bridge-process.json'}
def eligible(p):
    relative=p.relative_to(root)
    return not(set(relative.parts)&excluded) and p.name not in excluded_names and p.suffix not in {'.db','.log','.exe','.iml','.pyc'} and not p.is_symlink()
source_files=[]
for folder in ['apps','server','docs','deploy','schemas','bridge','extension','scripts','.github']:
    for p in (root/folder).rglob('*'):
        if p.is_file() and eligible(p):
            if p.relative_to(root).parts[:3]==('apps','web','dist'): continue
            source_files.append(p)
for name in ['README.md','Makefile','.gitignore','.gitattributes','.dockerignore']:
    source_files.append(root/name)
source_zip=artifacts/'aihub-m0-source.zip'
with zipfile.ZipFile(source_zip,'w',zipfile.ZIP_DEFLATED) as archive:
    for p in sorted(source_files):archive.write(p,'aihub-m0/'+p.relative_to(root).as_posix())
runtime_zip=artifacts/'aihub-m0-runtime.zip'
with zipfile.ZipFile(runtime_zip,'w',zipfile.ZIP_DEFLATED) as archive:
    for name in ['aihub-linux-amd64','aihub-windows-amd64.exe','aihub-bridge-linux-amd64','aihub-bridge-windows-amd64.exe','aihub-admin-linux-amd64','aihub-admin-windows-amd64.exe']:
        p=artifacts/'aihub-m0'/name
        if not p.is_file() or p.stat().st_size<1_000_000:raise RuntimeError('Missing built server: '+name)
        info=zipfile.ZipInfo('aihub-m0/'+name)
        info.external_attr=(0o100755 << 16)
        archive.writestr(info,p.read_bytes(),compress_type=zipfile.ZIP_DEFLATED)
    for name in ['DEPLOYMENT_ZH.md','VALIDATION.md','CONTRACTS.md','UPSTREAM.md','CODEX_CONNECTION_ZH.md','CODEX_ADVISOR_ZH.md','CODEX_WORKSPACE_ZH.md','DESKTOP_APP_ZH.md','M1_M2_IMPLEMENTATION_20260928_ZH.md']:
        archive.write(root/'docs'/name,'aihub-m0/docs/'+name)
    for name in ['Start-Codex-Bridge.ps1','Stop-Codex-Bridge.ps1','bridge.example.json']:
        archive.write(root/'bridge'/name,'aihub-m0/bridge/'+name)
    for name in ['server.env.example','aihub.service','install-ubuntu-ip.sh']:
        archive.write(root/'deploy'/name,'aihub-m0/'+name)
# Audit the actual ZIP entries and embedded config files, not merely the staging plan.
for path in [source_zip,runtime_zip]:
    with zipfile.ZipFile(path) as archive:
        if archive.testzip() is not None:raise RuntimeError('Corrupt zip')
        for name in archive.namelist():
            parts=set(Path(name).parts)
            if parts & excluded or Path(name).name in excluded_names:
                raise RuntimeError('Unexpected private/runtime path: '+name)
            if Path(name).suffix in {'.db','.log'}:raise RuntimeError('Runtime data in archive')
checksums={}
for p in [source_zip,runtime_zip,*sorted((artifacts/'aihub-m0').glob('*'))]:
    if p.is_file():checksums[p.name]={'sha256':hashlib.sha256(p.read_bytes()).hexdigest(),'bytes':p.stat().st_size}
(artifacts/'SHA256SUMS.json').write_text(json.dumps(checksums,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'source_files':len(source_files),'packages':checksums},indent=2))
