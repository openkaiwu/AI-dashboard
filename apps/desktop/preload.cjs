const {contextBridge,ipcRenderer}=require('electron');
contextBridge.exposeInMainWorld('aihubDesktop',Object.freeze({
 alert:(value)=>ipcRenderer.invoke('quota-alert',value),
 localRequest:(profileId,path,method,body)=>ipcRenderer.invoke('local-data-request',profileId,path,method,body),
 localStatus:(profileId,patch)=>ipcRenderer.invoke('local-data-status',profileId,patch),
 syncLocalData:(profileId)=>ipcRenderer.invoke('local-data-sync',profileId),
 openCodex:()=>ipcRenderer.invoke('open-codex'),
 sessionGet:(id)=>ipcRenderer.sendSync('session-get',id),
 installationID:(previous)=>ipcRenderer.sendSync('installation-id',previous),
 sessionSet:(id,value)=>ipcRenderer.sendSync('session-set',id,value),
 configureServer:(url)=>ipcRenderer.invoke('configure-server',url),
 extensionPairCode:()=>ipcRenderer.invoke('extension-pair-code')
}));
