// Runs the whole file with process.platform faked to darwin (must happen
// before platform.cjs is loaded) to exercise the macOS branches on any host.
Object.defineProperty(process,'platform',{value:'darwin'});
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
const platform=require('./platform.cjs');

test('darwin keeps login item support and names darwin binaries',()=>{
	assert.equal(platform.loginItemSupported(),true);
	assert.equal(platform.appUserModelIdSupported(),false);
	assert.equal(platform.binNames('darwin','arm64').serverDev,'aihub-darwin-arm64');
	assert.equal(platform.binNames().serverPackaged,'aihub-server');
});

test('darwin database setup fails with actionable brew guidance when absent',async()=>{
	const dir=fs.mkdtempSync(path.join(os.tmpdir(),'aihub-darwin-test-'));
	assert.equal(platform.binNames().codex,'codex');
	await assert.rejects(()=>platform.ensureDatabase(dir),/brew install postgresql/);
	assert.equal(fs.existsSync(path.join(dir,'server.env')),false,'env must not be written when bootstrap fails');
});
