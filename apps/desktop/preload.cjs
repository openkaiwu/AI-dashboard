const {contextBridge,ipcRenderer}=require('electron');
contextBridge.exposeInMainWorld('aihubDesktop',Object.freeze({
 alert:(value)=>ipcRenderer.invoke('quota-alert',value),
 openCodex:()=>ipcRenderer.invoke('open-codex'),
 sessionGet:(id)=>ipcRenderer.sendSync('session-get',id),
 sessionSet:(id,value)=>ipcRenderer.sendSync('session-set',id,value),
 configureServer:(url)=>ipcRenderer.invoke('configure-server',url),
 extensionPairCode:()=>ipcRenderer.invoke('extension-pair-code')
}));
