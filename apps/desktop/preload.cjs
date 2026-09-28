const {contextBridge,ipcRenderer}=require('electron');
contextBridge.exposeInMainWorld('aihubDesktop',Object.freeze({
 alert:(value)=>ipcRenderer.invoke('quota-alert',value),
 openCodex:()=>ipcRenderer.invoke('open-codex')
}));
