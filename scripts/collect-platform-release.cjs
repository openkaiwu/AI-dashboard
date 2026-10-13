const fs=require('node:fs');
const path=require('node:path');
const crypto=require('node:crypto');
const {version}=require('../apps/desktop/package.json');
const platform=process.env.RELEASE_PLATFORM,arch=process.env.RELEASE_ARCH;
if(!['linux','darwin'].includes(platform)||!['x64','arm64'].includes(arch))throw Error('Unsupported release platform or architecture');
const source='artifacts/desktop/installer',destination=`artifacts/release/${platform}-${arch}`;
const names=platform==='linux'?[`AIHub-${version}-linux-${arch}.AppImage`,`AIHub-${version}-linux-${arch}.deb`]:[`AIHub-${version}-${arch}.dmg`,`AIHub-${version}-${arch}.zip`];
fs.mkdirSync(destination,{recursive:true});
for(const name of names){
 const sourceName=platform==='linux'&&arch==='x64'?name.replace('-x64.AppImage','-x86_64.AppImage').replace('-x64.deb','-amd64.deb'):name;
 const file=path.join(source,sourceName);
 if(fs.statSync(file).size<1000000)throw Error('Invalid package: '+file);
 fs.copyFileSync(file,path.join(destination,name));
}
const goarch=arch==='x64'?'amd64':arch;
for(const kind of ['aihub','aihub-bridge','aihub-admin']){
 const name=`${kind}-${platform}-${goarch}`;
 fs.copyFileSync(`artifacts/aihub-m0/${name}`,path.join(destination,name));
 names.push(name);
}
const manifest=names.map(name=>`${crypto.createHash('sha256').update(fs.readFileSync(path.join(destination,name))).digest('hex')}  ${name}`).join('\n')+'\n';
fs.writeFileSync(path.join(destination,`SHA256SUMS-${platform}-${arch}.txt`),manifest);
console.log(manifest);
