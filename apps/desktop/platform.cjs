// Per-platform registry for the desktop runtime: binary names, Codex CLI
// discovery and the local-mode PostgreSQL strategy. Windows keeps the WSL
// pipeline verbatim; Linux and macOS run the server natively.
const crypto=require('node:crypto');
const fs=require('node:fs');
const net=require('node:net');
const os=require('node:os');
const path=require('node:path');
const {execFile,spawn}=require('node:child_process');
const {promisify}=require('node:util');
const execFileAsync=promisify(execFile);

const PLATFORM=process.platform;
const IS_WIN=PLATFORM==='win32';
const IS_MAC=PLATFORM==='darwin';
const IS_LINUX=PLATFORM==='linux';

function goosOf(platform){return platform==='win32'?'windows':platform==='darwin'?'darwin':'linux';}
function goarchOf(arch){return arch==='arm64'?'arm64':'amd64';}

// Pure mapping so tests can assert every platform without changing process.platform.
// Dev names must stay compatible with scripts/build.sh outputs:
//   aihub-windows-amd64.exe / aihub-bridge-windows-amd64.exe / aihub-linux-amd64 / aihub-darwin-arm64 ...
// Packaged names must stay compatible with scripts/Stage-Desktop-Binaries.ps1 (win32)
// and scripts/stage-desktop-binaries.sh (linux/darwin).
function binNames(platform=PLATFORM,arch=process.arch){
	const ext=platform==='win32'?'.exe':'';
	const suffix=`${goosOf(platform)}-${goarchOf(arch)}`;
	return {
		serverPackaged:platform==='win32'?'aihub-server.exe':'aihub-server',
		bridgePackaged:platform==='win32'?'aihub-bridge.exe':'aihub-bridge',
		serverDev:`aihub-${suffix}${ext}`,
		bridgeDev:`aihub-bridge-${suffix}${ext}`,
		adminDev:`aihub-admin-${suffix}${ext}`,
		codex:platform==='win32'?'codex.exe':'codex',
	};
}
function loginItemSupportedFor(platform){return platform==='win32'||platform==='darwin';}
function appUserModelIdSupportedFor(platform){return platform==='win32';}
function codexSearchRoots(platform,homedir){
	const roots=[path.join(homedir,'.codex','bin'),path.join(homedir,'.local','bin')];
	if(platform==='darwin')roots.unshift('/opt/homebrew/bin','/usr/local/bin');
	else roots.push(path.join(homedir,'.npm-global','bin'),'/usr/local/bin','/usr/bin');
	return roots;
}

function isExecutableFile(p){
	try{return fs.statSync(p).isFile()&&(fs.statSync(p).mode&0o111)!==0;}catch{return false;}
}

async function findCodex(){
	const names=binNames();
	if(IS_WIN){
		const roots=[path.join(process.env.LOCALAPPDATA||'','OpenAI','Codex','bin')];
		for(const root of roots){
			if(!fs.existsSync(root))continue;
			const stack=[root];
			while(stack.length){
				const dir=stack.pop();
				for(const entry of fs.readdirSync(dir,{withFileTypes:true})){
					const full=path.join(dir,entry.name);
					if(entry.isDirectory())stack.push(full);
					else if(entry.name===names.codex)return full;
				}
			}
		}
		return '';
	}
	try{
		const which=await execFileAsync('sh',['-lc','command -v codex']);
		const p=which.stdout.trim();
		if(p&&isExecutableFile(p))return p;
	}catch{}
	const home=os.homedir();
	for(const root of codexSearchRoots(PLATFORM,home)){
		const p=path.join(root,names.codex);
		if(isExecutableFile(p))return p;
	}
	return '';
}

function tcpReachable(port,host='127.0.0.1',timeoutMs=800){
	return new Promise(resolve=>{
		const sock=net.connect({host,port});
		const done=value=>{sock.destroy();resolve(value);};
		sock.setTimeout(timeoutMs,()=>done(false));
		sock.once('connect',()=>done(true));
		sock.once('error',()=>done(false));
	});
}

// ---- local-mode PostgreSQL -------------------------------------------------

function execWsl(script){return execFileAsync('wsl.exe',['-e','bash','-lc',script],{windowsHide:true});}

async function ensurePostgresWindows(dir){
	try{await execFileAsync('wsl.exe',['-e','true'],{windowsHide:true});}
	catch{throw new Error('未检测到 WSL，请先安装 WSL 与 PostgreSQL');}
	const keepFile=path.join(dir,'wsl-keepalive.json');
	let alive=false;
	if(fs.existsSync(keepFile)){
		try{const saved=JSON.parse(fs.readFileSync(keepFile,'utf8'));process.kill(saved.Id,0);alive=true;}catch{}
	}
	if(!alive){
		const keep=spawn('wsl.exe',['-e','sleep','infinity'],{stdio:'ignore',windowsHide:true,detached:true});
		keep.unref();
		fs.writeFileSync(keepFile,JSON.stringify({Id:keep.pid,StartTicks:String(keep.spawnfile||'')}));
	}
	await execWsl('for v in 14 16 15 13; do pg_ctlcluster "$v" main status >/dev/null 2>&1 && { pg_ctlcluster "$v" main start; exit 0; }; done; pg_ctlcluster 14 main start');
}

async function ensurePostgresLinux(){
	if(await tcpReachable(5432))return;
	throw new Error('未检测到本机 PostgreSQL。请先安装并启动：\n  sudo apt install postgresql\n  sudo systemctl enable --now postgresql\n完成后从托盘菜单选择「重新连接」。');
}

function darwinBrewPath(){
	for(const p of ['/opt/homebrew/bin/brew','/usr/local/bin/brew'])if(fs.existsSync(p))return p;
	return '';
}
async function darwinPostgresPrefix(brew){
	for(const v of ['16','15','14','17']){
		try{
			const {stdout}=await execFileAsync(brew,['--prefix','postgresql@'+v]);
			const prefix=stdout.trim();
			if(prefix&&fs.existsSync(path.join(prefix,'bin','postgres')))return {prefix,version:v};
		}catch{}
	}
	return null;
}
async function ensurePostgresDarwin(){
	if(await tcpReachable(5432))return;
	const brew=darwinBrewPath();
	if(!brew)throw new Error('未检测到 Homebrew。请先安装 Homebrew，再在终端执行：\n  brew install postgresql@16\n完成后从托盘菜单选择「重新连接」。');
	const pg=await darwinPostgresPrefix(brew);
	if(!pg)throw new Error('未检测到 PostgreSQL。请在终端执行：\n  brew install postgresql@16\n完成后从托盘菜单选择「重新连接」。');
	await execFileAsync(brew,['services','start','postgresql@'+pg.version]).catch(()=>{});
	for(let i=0;i<40;i++){if(await tcpReachable(5432))return;await new Promise(r=>setTimeout(r,500));}
	throw new Error('PostgreSQL 启动超时，请手动执行：brew services start postgresql@'+pg.version);
}

async function ensurePostgres(dir){
	if(IS_WIN)return ensurePostgresWindows(dir);
	if(IS_MAC)return ensurePostgresDarwin();
	return ensurePostgresLinux();
}

// ---- local-mode database bootstrap ----------------------------------------

function readEnvFile(file){
	const line=fs.readFileSync(file,'utf8').split(/\r?\n/).find(l=>l.startsWith('export AIHUB_DATABASE_URL='));
	if(!line)throw new Error('server.env 缺少数据库配置');
	return line.slice('export AIHUB_DATABASE_URL='.length).trim().replace(/^"|"$/g,'');
}

function guidedSetupError(sqlFile){
	const err=new Error(`首次使用本地模式，需要初始化数据库。请在终端执行：\n\n  sudo -u postgres psql -v ON_ERROR_STOP=1 -f "${sqlFile}"\n\n执行完成后，从托盘菜单选择「重新连接」。`);
	err.guided=true;
	return err;
}

// Creates role + database for the local server and returns the database URL.
// Windows boots psql inside WSL (legacy behaviour); macOS talks to Homebrew
// PostgreSQL directly as the current superuser; Linux cannot elevate from a
// GUI session, so it writes aihub-setup.sql + server.env and tells the user
// to run one sudo command.
async function ensureDatabase(dir){
	const envFile=path.join(dir,'server.env');
	if(fs.existsSync(envFile))return readEnvFile(envFile);
	const password=crypto.randomBytes(24).toString('hex');
	const url=`postgres://aihub_m0:${password}@127.0.0.1:5432/aihub_m0?sslmode=disable`;
	const sqlPlain=`DO $$ BEGIN CREATE ROLE aihub_m0 LOGIN PASSWORD '${password}'; EXCEPTION WHEN duplicate_object THEN NULL; END $$; DO $$ BEGIN CREATE DATABASE aihub_m0 OWNER aihub_m0; EXCEPTION WHEN duplicate_database THEN NULL; END $$;`;
	if(IS_LINUX){
		const sqlFile=path.join(dir,'aihub-setup.sql');
		fs.writeFileSync(sqlFile,sqlPlain+'\n',{mode:0o600});
		fs.writeFileSync(envFile,`export AIHUB_DATABASE_URL="${url}"\n`,{mode:0o600});
		throw guidedSetupError(sqlFile);
	}
	if(IS_MAC){
		const brew=darwinBrewPath();
		const pg=brew?await darwinPostgresPrefix(brew):null;
		if(!pg)throw new Error('未检测到 PostgreSQL。请在终端执行：\n  brew install postgresql@16\n然后从托盘菜单选择「重新连接」。');
		try{await execFileAsync(path.join(pg.prefix,'bin','psql'),['-v','ON_ERROR_STOP=1','postgres','-c',sqlPlain]);}
		catch(e){throw new Error('数据库初始化失败：'+e.message+'\n请确认 PostgreSQL 已启动（brew services start postgresql@'+pg.version+'）。');}
	}else{
		const sqlShell=sqlPlain.split('$$').join('\\$\\$');
		await execWsl(`(command -v runuser >/dev/null && runuser -u postgres -- psql -v ON_ERROR_STOP=1 -c "${sqlShell}") || sudo -u postgres psql -v ON_ERROR_STOP=1 -c "${sqlShell}"`);
	}
	fs.writeFileSync(envFile,`export AIHUB_DATABASE_URL="${url}"\n`,{mode:0o600});
	return url;
}

// Extra hint when the local server never became ready (e.g. guided setup was
// not executed yet); empty string on platforms without a guided path.
function serverStartupHint(dir){
	if(!IS_LINUX)return '';
	const sqlFile=path.join(dir,'aihub-setup.sql');
	if(!fs.existsSync(sqlFile))return '';
	return `本地服务启动超时。若尚未初始化数据库，请在终端执行：\nsudo -u postgres psql -v ON_ERROR_STOP=1 -f "${sqlFile}"\n完成后从托盘菜单选择「重新连接」。`;
}

// ---- UI copy ---------------------------------------------------------------

function askModeLocalHint(){
	if(IS_WIN)return '需要 WSL、PostgreSQL 和管理员初始化。';
	if(IS_MAC)return '需要 Homebrew PostgreSQL。';
	return '需要本机 PostgreSQL 和数据库初始化。';
}
function localModeHelp(){
	if(IS_WIN)return '请确认已安装 WSL 与 PostgreSQL 14。';
	if(IS_MAC)return '请确认已通过 Homebrew 安装并启动 PostgreSQL（brew install postgresql@16）。';
	return '请确认已安装并启动本机 PostgreSQL（sudo apt install postgresql）。';
}

module.exports={
	PLATFORM,IS_WIN,IS_MAC,IS_LINUX,
	binNames,loginItemSupportedFor,appUserModelIdSupportedFor,codexSearchRoots,
	loginItemSupported:()=>loginItemSupportedFor(PLATFORM),
	appUserModelIdSupported:()=>appUserModelIdSupportedFor(PLATFORM),
	findCodex,tcpReachable,ensurePostgres,ensureDatabase,serverStartupHint,
	askModeLocalHint,localModeHelp,
};
