import {expect,it} from 'vitest';
import {eligibleStrong} from './strongReminderPolicy';
const a={id:'a',notified_at:'',dismissed:false,snoozed_until:null,advice:{kind:'slow',title:'',body:'',priority:2}};
it('only raises actionable quota or expiry reminders',()=>{
 expect(eligibleStrong(a,1000)).toBe(true);
 expect(eligibleStrong({...a,dismissed:true},1000)).toBe(false);
 expect(eligibleStrong({...a,snoozed_until:new Date(2000).toISOString()},1000)).toBe(false);
 expect(eligibleStrong({...a,advice:{...a.advice,kind:'news'}},1000)).toBe(false);
 expect(eligibleStrong({...a,snoozed_until:new Date(500).toISOString()},1000)).toBe(true);
});
