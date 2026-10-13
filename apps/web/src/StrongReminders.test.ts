import {expect,it} from 'vitest';
import {alertState,eligibleStrong,eligibleGeneric,reminderCandidates,canInterrupt} from './strongReminderPolicy';
import {remainingLabel} from './quotaFormat';
const a={id:'a',notified_at:new Date(0).toISOString(),dismissed:false,snoozed_until:null,advice:{kind:'slow',title:'',body:'',priority:2}};
it('only raises actionable quota or expiry reminders',()=>{
 expect(eligibleStrong(a,1000)).toBe(true);
 expect(eligibleStrong({...a,dismissed:true},1000)).toBe(false);
 expect(eligibleStrong({...a,snoozed_until:new Date(2000).toISOString()},1000)).toBe(false);
 expect(eligibleStrong({...a,advice:{...a.advice,kind:'news'}},1000)).toBe(false);
 expect(eligibleStrong({...a,advice:{...a.advice,kind:'use'}},1000)).toBe(false);
 expect(eligibleStrong({...a,read_at:new Date(0).toISOString()},1000)).toBe(false);
 expect(eligibleStrong({...a,notified_at:null},1000)).toBe(false);
 expect(eligibleStrong({...a,advice:{...a.advice,interrupt:false}},1000)).toBe(false);
 expect(eligibleStrong({...a,snoozed_until:new Date(500).toISOString()},1000)).toBe(true);
});
it('spaces equal-priority popups but allows escalation and preserves a small remaining quota',()=>{
 const last={at:1000,priority:3};
 expect(canInterrupt(last,3,2000)).toBe(false);
 expect(canInterrupt(last,4,2000)).toBe(true);
 expect(canInterrupt(last,2,601000)).toBe(true);
 expect(canInterrupt({at:NaN,priority:3},2,2000)).toBe(true);
 expect(remainingLabel(0.04)).toBe('<0.1');
 expect(remainingLabel(0.3)).toBe('0.3');
 expect(remainingLabel(0)).toBe('0');
});
it('ranks quota exhaustion above expiry and does not let old notifications bypass the advisor',()=>{
 const n={id:'legacy',title:'',body:'',created_at:'',status:'unread',severity:'warning',provider_slug:'codex',dedupe_key:'quota.low:bucket'};
 expect(eligibleGeneric(n)).toBe(false);
 expect(eligibleGeneric({...n,provider_slug:'cursor',active:false})).toBe(false);
 expect(eligibleGeneric({...n,provider_slug:'cursor',dedupe_key:'quota.reset_soon_unused:bucket'})).toBe(false);
 const credit={...a,id:'credit',advice:{...a.advice,kind:'credit',priority:3}};
 const exhausted={...a,id:'exhausted',advice:{...a.advice,priority:4}};
 expect(reminderCandidates([a,credit,exhausted],[n],1000).map(c=>c.id)).toEqual(['exhausted','credit','a']);
 expect(alertState({...a,read_at:new Date(0).toISOString()},1000)).toBe('read');
});
