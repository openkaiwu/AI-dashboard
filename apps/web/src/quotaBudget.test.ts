import {expect,it} from 'vitest';
import {budget} from './quotaBudget';
it('reserves quota and caps short-window allocation at available quota',()=>{
 expect(budget(60,48,10,false)).toEqual({available:50,nextDay:25});
 expect(budget(60,2,10,false)).toEqual({available:50,nextDay:50});
 expect(budget(5,48,10,false)).toEqual({available:0,nextDay:0});
});
it('does not plan against stale, unknown or expired windows',()=>{
 expect(budget(60,48,10,true)).toBeNull();
 expect(budget(60,null,10,false)).toBeNull();
 expect(budget(60,0,10,false)).toBeNull();
});
