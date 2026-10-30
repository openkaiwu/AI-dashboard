
type Popup = { title: string; body: string; provider?: string };
declare global { interface Window { aihubDesktop?: {
  info?: () => {version:string;bundledUI:boolean;platform:string};
  alert: (value: Popup) => Promise<string>; openCodex: () => Promise<void>;
  timeZoneStatus?: () => Promise<import('./pages/TimeZone').TimeZoneState>;
  timeZoneApply?: (target:'cn'|'la') => Promise<{success:boolean;message:string;state:import('./pages/TimeZone').TimeZoneState}>;
  openDashboard?: () => Promise<void>; sessionGet: (id: string) => import('./session').Session | null;
  sessionSet: (id: string, value: import('./session').Session | null) => boolean;
  installationID?: (previous: string | null) => string; extensionPairCode: () => Promise<string>;
  configureServer: (url: string) => Promise<string>;
  localRequest?: (profileId:string,path:string,method:string,body?:unknown)=>Promise<{handled:boolean;value:unknown}>;
  localStatus?: (profileId:string,patch?:{auto_sync:boolean})=>Promise<import('./LocalDataSettings').StorageStatus>;
  syncLocalData?: (profileId:string)=>Promise<import('./LocalDataSettings').StorageStatus>;
  bridgeStatus?: () => Promise<import('./pages/Desktop').BridgeStatus>;
  bridgeControl?: (action: 'pause' | 'resume') => Promise<import('./pages/Desktop').BridgeStatus>;
  scanRoots?: () => Promise<{ roots: string[] }>;
  scanPick?: () => Promise<{ roots: string[] }>;
  scanRemove?: (root: string) => Promise<{ roots: string[]; removed: boolean }>;
  scanRun?: () => Promise<{ entries: import('./pages/Desktop').ScanEntry[] }>;
  diagnostics?: () => Promise<import('./pages/Desktop').Diagnostics>;
} } }

export default function StrongReminders() { return null; }
