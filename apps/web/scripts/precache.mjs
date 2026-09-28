import { readFile, writeFile, readdir } from 'node:fs/promises';
import { createHash } from 'node:crypto';
// A self-contained shell avoids a cached HTML/new bundle mismatch after updates.
// The app has no dynamic chunks; inline its small JS/CSS bundle before precaching.
let html=await readFile('dist/index.html','utf8');
for(const match of [...html.matchAll(/<script\b[^>]*src="(\/assets\/[^\"]+)"[^>]*><\/script>/g)]){
 const js=await readFile('dist'+match[1],'utf8');
 html=html.replace(match[0],()=>'<script type="module">'+js.replace(/<\/script/gi,'<\\/script')+'</script>');
}
for(const match of [...html.matchAll(/<link\b[^>]*href="(\/assets\/[^\"]+\.css)"[^>]*>/g)]){
 const css=await readFile('dist'+match[1],'utf8');
 html=html.replace(match[0],()=>'<style>'+css.replace(/<\/style/gi,'<\\/style')+'</style>');
}
await writeFile('dist/index.html',html);
const id=createHash('sha256').update(html).digest('hex').slice(0,12);
const assets=(await readdir('dist/assets')).map(name=>'/assets/'+name);
let sw=await readFile('public/sw.js','utf8');
sw=sw.replace('const CACHE="aihub-shell-v1";',`const CACHE="aihub-shell-${id}";`);
sw=sw.replace('["/","/manifest.webmanifest","/icon.svg"]',JSON.stringify(['/','/manifest.webmanifest','/icon.svg',...assets]));
await writeFile('dist/sw.js',sw);
console.log(`Precached ${assets.length} versioned assets for offline launch.`);
