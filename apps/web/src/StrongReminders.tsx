
type Popup = { title: string; body: string; provider?: string };
declare global { interface Window { aihubDesktop?: {
  alert: (value: Popup) => Promise<string>; openCodex: () => Promise<void>;
  openDashboard?: () => Promise<void>; sessionGet: (id: string) => import('./session').Session | null;
  sessionSet: (id: string, value: import('./session').Session | null) => boolean;
  installationID?: (previous: string | null) => string; extensionPairCode: () => Promise<string>;
  configureServer: (url: string) => Promise<string>;
  localRequest?: (profileId:string,path:string,method:string,body?:unknown)=>Promise<{handled:boolean;value:unknown}>;
  localStatus?: (profileId:string,patch?:{auto_sync:boolean})=>Promise<import('./LocalDataSettings').StorageStatus>;
  syncLocalData?: (profileId:string)=>Promise<import('./LocalDataSettings').StorageStatus>;
} } }

export default function StrongReminders() { return null; }
