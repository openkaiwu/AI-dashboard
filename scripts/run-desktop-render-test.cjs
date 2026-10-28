const {spawn} = require('node:child_process');
const path = require('node:path');
const env = {...process.env};
delete env.ELECTRON_RUN_AS_NODE;
if (process.argv[2]) {
  const resources = path.resolve(process.argv[2], 'resources');
  env.AIHUB_TEST_DESKTOP_ROOT = path.join(resources,'app.asar');
  env.AIHUB_TEST_UI_ROOT = path.join(resources,'ui');
}
const executable = require('../apps/desktop/node_modules/electron');
const child = spawn(executable, [path.join(__dirname,'test-desktop-render.cjs')], {env,stdio:'inherit',windowsHide:true});
child.on('error', error => { console.error(error); process.exitCode=1; });
child.on('exit', code => { process.exitCode=code ?? 1; });
