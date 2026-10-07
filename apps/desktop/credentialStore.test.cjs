const {test}=require('node:test');
const assert=require('node:assert/strict');
const {execFileSync}=require('node:child_process');
const store=require('./credentialStore.cjs');
test('existing Electron encrypted values retain compatibility',()=>{
 assert.equal(store.decrypt(Buffer.from('legacy-cipher'),{decryptString:()=> 'legacy-session'}),'legacy-session');
});
test('Windows credentials decrypt after a new process starts without Chromium encryption keys',{skip:process.platform!=='win32'},()=>{
 const encrypted=store.encrypt('isolated-test-session',{});
 const code="const s=require('./apps/desktop/credentialStore.cjs');process.stdout.write(s.decrypt(Buffer.from(process.env.AIHUB_TEST_CIPHER,'base64'),{}));";
 const output=execFileSync(process.execPath,['-e',code],{cwd:require('node:path').resolve(__dirname,'../..'),env:{...process.env,AIHUB_TEST_CIPHER:encrypted.toString('base64')},encoding:'utf8'});
 assert.equal(output,'isolated-test-session');
});
