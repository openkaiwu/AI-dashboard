const {spawn}=require('node:child_process'),path=require('node:path');
const env={...process.env};delete env.ELECTRON_RUN_AS_NODE;
const child=spawn(require('../apps/desktop/node_modules/electron'),[path.join(__dirname,'test-desktop-timezone.cjs'),...process.argv.slice(2)],{env,stdio:'inherit',windowsHide:true});
child.on('error',e=>{console.error(e.message);process.exitCode=1;});
child.on('exit',code=>{process.exitCode=code??1;});
