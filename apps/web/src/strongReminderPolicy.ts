export type Alert={id:string;notified_at:string;dismissed:boolean;snoozed_until:string|null;advice:{kind:string;title:string;body:string;priority:number}};
export function eligibleStrong(a:Alert,now:number){return !a.dismissed&&(!a.snoozed_until||Date.parse(a.snoozed_until)<=now)&&['slow','use','credit'].includes(a.advice.kind);}
