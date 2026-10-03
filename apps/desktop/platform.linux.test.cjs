// Runs the whole file with process.platform faked to linux (must happen
// before platform.cjs is loaded) so the guided local-mode database setup is
// exercised on any host OS.
Object.defineProperty(process,'platform',{value:'linux'});
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
const platform=require('./platform.cjs');

test('linux guided database setup writes sql+env, then is idempotent',async()=>{
	const dir=fs.mkdtempSync(path.join(os.tmpdir(),'aihub-linux-test-'));
	const sqlFile=path.join(dir,'aihub-setup.sql');
	const envFile=path.join(dir,'server.env');
	let err=null;
	try{await platform.ensureDatabase(dir);}catch(e){err=e;}
	assert.ok(err&&err.guided,'first call must fail with the guided setup error');
	assert.match(err.message,/sudo -u postgres psql -v ON_ERROR_STOP=1 -f /);
	assert.ok(err.message.includes(sqlFile),'guidance must reference the generated sql file');
	assert.ok(fs.existsSync(sqlFile));
	const env=fs.readFileSync(envFile,'utf8');
	assert.match(env,/^export AIHUB_DATABASE_URL="postgres:\/\/aihub_m0:[0-9a-f]{48}@127\.0\.0\.1:5432\/aihub_m0\?sslmode=disable"\n$/);
	const url=await platform.ensureDatabase(dir);
	assert.equal(url,env.trim().slice('export AIHUB_DATABASE_URL='.length).replace(/^"|"$/g,''));
	assert.match(platform.serverStartupHint(dir),/aihub-setup\.sql/);
});

test('linux skips login item and app user model id',()=>{
	assert.equal(platform.loginItemSupported(),false);
	assert.equal(platform.appUserModelIdSupported(),false);
});
