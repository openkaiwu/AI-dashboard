export function budget(remaining:number,hours:number|null,reserve:number,stale:boolean){
 if(stale||hours==null||!Number.isFinite(hours)||hours<=0||!Number.isFinite(remaining)||!Number.isFinite(reserve))return null;
 const available=Math.max(0,Math.min(100,remaining)-Math.max(0,reserve));
 return {available,nextDay:available*Math.min(1,24/hours)};
}
