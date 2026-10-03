// Generate a macOS .icns from a square PNG source.
// Modern icns files can embed PNG-encoded images directly; we map the source
// size to its icon type (ic08=256, ic09=512, ic10=1024 ...). Regenerate after
// replacing icon.png with a larger source for crisper dock icons.
// Usage: node scripts/make-icns.cjs [src.png] [out.icns]
'use strict';
const fs=require('node:fs');
const path=require('node:path');

const root=path.resolve(__dirname,'..');
const src=process.argv[2]?path.resolve(process.argv[2]):path.join(root,'apps/desktop/icon.png');
const out=process.argv[3]?path.resolve(process.argv[3]):path.join(root,'apps/desktop/icon.icns');

const png=fs.readFileSync(src);
const sig=png.subarray(0,8).toString('latin1');
if(sig!=='\x89PNG\r\n\x1a\n')throw new Error(`${src} is not a PNG`);
const width=png.readUInt32BE(16), height=png.readUInt32BE(20);
if(width!==height)throw new Error(`source must be square, got ${width}x${height}`);
const types={16:'icp4',32:'icp5',64:'icp6',128:'ic07',256:'ic08',512:'ic09',1024:'ic10',2048:'ic11'};
const type=types[width];
if(!type)throw new Error(`unsupported icon size ${width} (use 16/32/64/128/256/512/1024)`);

const chunk=Buffer.alloc(8+png.length);
chunk.write(type,0,'latin1');
chunk.writeUInt32BE(8+png.length,4);
png.copy(chunk,8);
const icns=Buffer.alloc(8+chunk.length);
icns.write('icns',0,'latin1');
icns.writeUInt32BE(icns.length,4);
chunk.copy(icns,8);
fs.writeFileSync(out,icns);
console.log(`${out}: ${icns.length} bytes (${type}, ${width}x${height})`);
