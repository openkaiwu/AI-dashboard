const {test}=require('node:test');
const assert=require('node:assert/strict');
const path=require('node:path');
const {binNames,loginItemSupportedFor,appUserModelIdSupportedFor,codexSearchRoots,askModeLocalHint,localModeHelp}=require('./platform.cjs');

test('binary names match build.sh outputs and packaged staging per platform',()=>{
	const win=binNames('win32','x64');
	assert.equal(win.serverDev,'aihub-windows-amd64.exe');
	assert.equal(win.bridgeDev,'aihub-bridge-windows-amd64.exe');
	assert.equal(win.serverPackaged,'aihub-server.exe');
	assert.equal(win.bridgePackaged,'aihub-bridge.exe');
	assert.equal(win.codex,'codex.exe');
	const linux=binNames('linux','x64');
	assert.equal(linux.serverDev,'aihub-linux-amd64');
	assert.equal(linux.bridgeDev,'aihub-bridge-linux-amd64');
	assert.equal(linux.serverPackaged,'aihub-server');
	assert.equal(linux.bridgePackaged,'aihub-bridge');
	assert.equal(linux.codex,'codex');
	const mac=binNames('darwin','arm64');
	assert.equal(mac.serverDev,'aihub-darwin-arm64');
	assert.equal(mac.bridgeDev,'aihub-bridge-darwin-arm64');
	assert.equal(mac.serverPackaged,'aihub-server');
	assert.equal(binNames('darwin','x64').serverDev,'aihub-darwin-amd64');
});

test('platform capability guards keep Linux from calling unsupported Electron APIs',()=>{
	assert.equal(loginItemSupportedFor('win32'),true);
	assert.equal(loginItemSupportedFor('darwin'),true);
	assert.equal(loginItemSupportedFor('linux'),false);
	assert.equal(appUserModelIdSupportedFor('win32'),true);
	assert.equal(appUserModelIdSupportedFor('darwin'),false);
	assert.equal(appUserModelIdSupportedFor('linux'),false);
});

test('codex search roots cover the documented CLI install locations',()=>{
	const home=path.join('home','u');
	const linuxRoots=codexSearchRoots('linux',home);
	assert.ok(linuxRoots.includes(path.join(home,'.codex','bin')));
	assert.ok(linuxRoots.includes(path.join(home,'.local','bin')));
	assert.ok(linuxRoots.includes('/usr/local/bin'));
	const darwinRoots=codexSearchRoots('darwin',home);
	assert.equal(darwinRoots[0],'/opt/homebrew/bin');
	assert.ok(darwinRoots.includes('/usr/local/bin'));
});

test('local-mode guidance copy is present for the host platform',()=>{
	assert.ok(askModeLocalHint().length>10);
	assert.ok(localModeHelp().length>10);
});
