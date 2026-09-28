import { beforeEach, describe, expect, it, vi } from "vitest";
import { IDBFactory } from "fake-indexeddb";
const mocks=vi.hoisted(()=>({key:"a",request:vi.fn()}));
vi.mock("../src/session",()=>({scope:()=>mocks.key}));
vi.mock("../src/api",()=>({request:mocks.request,ApiError:class ApiError extends Error{constructor(public status:number,public code:string,message:string){super(message);}}}));
beforeEach(()=>{
 vi.resetModules();mocks.request.mockReset();mocks.key=crypto.randomUUID();
 vi.stubGlobal("indexedDB",new IDBFactory());
 vi.stubGlobal("navigator",{locks:{request:async(_name:string,fn:()=>unknown)=>fn()}});
 vi.stubGlobal("dispatchEvent",()=>true);
});
const event=(op:any,version=1)=>({entity_id:op.entity_id,version,op:op.op,payload:op.payload});
describe("IndexedDB recovery",()=>{
 it("persists offline queue across module restart and retries same operation after lost response",async()=>{
  let sync=await import("../src/sync");await sync.enqueue("draft","body","note");
  const original=(await sync.loadState()).pending[0];
  mocks.request.mockRejectedValueOnce(new Error("offline"));
  await expect(sync.synchronize()).rejects.toThrow("offline");
  vi.resetModules();sync=await import("../src/sync");
  expect((await sync.loadState()).pending[0]).toEqual(original);
  mocks.request.mockRejectedValueOnce(new Error("response lost"));
  await expect(sync.synchronize()).rejects.toThrow("response lost");
  mocks.request.mockResolvedValueOnce({status:"applied",event:event(original)}).mockResolvedValueOnce({events:[event(original)],cursor:"e:1",has_more:false});
  await sync.synchronize();expect((await sync.loadState()).pending).toHaveLength(0);
  const posts=mocks.request.mock.calls.filter(c=>c[1]?.method==="POST");
  expect(posts.map(c=>JSON.parse(c[1].body).operation_id)).toEqual([original.operation_id,original.operation_id,original.operation_id]);
 });
 it("preserves conflict drafts and requires a fresh operation ID to resolve",async()=>{
  const sync=await import("../src/sync");await sync.enqueue("mine","local","note");const op=(await sync.loadState()).pending[0];
  mocks.request.mockResolvedValueOnce({status:"conflict",current:{...event(op,3),payload:{title:"remote",body:"new"}}}).mockResolvedValueOnce({events:[],cursor:"e:3",has_more:false});
  await sync.synchronize();expect((await sync.loadState()).conflicts[0].operation.payload.title).toBe("mine");
  await sync.resolveConflict(op.operation_id,true);const queued=(await sync.loadState()).pending[0];
  expect(queued.base_version).toBe(3);expect(queued.operation_id).not.toBe(op.operation_id);
 });
 it("rebuilds invalid cursor, applies tombstones, and scopes cache to account/server",async()=>{
  const sync=await import("../src/sync");const{ApiError}=await import("../src/api");
  mocks.request.mockRejectedValueOnce(new ApiError(409,"cursor_reset","reset"))
   .mockResolvedValueOnce({events:[{entity_id:"n",version:2,op:"delete",payload:{title:"",body:""}}],cursor:"new:2",has_more:false});
  await sync.synchronize();expect((await sync.loadState()).cursor).toBe("new:2");
  expect(sync.visibleNotes(await sync.loadState())).toHaveLength(0);
  mocks.key="other-account";expect((await sync.loadState()).notes).toEqual({});
 });
 it("keeps acknowledged first page when the next pull fails and stops on revoked credentials",async()=>{
  const sync=await import("../src/sync");
  mocks.request.mockResolvedValueOnce({events:[],cursor:"e:10",has_more:true}).mockRejectedValueOnce(new Error("interrupted"));
  await expect(sync.synchronize()).rejects.toThrow("interrupted");expect((await sync.loadState()).cursor).toBe("e:10");
  await sync.enqueue("draft","keep");mocks.request.mockRejectedValueOnce(new Error("revoked"));
  await expect(sync.synchronize()).rejects.toThrow("revoked");expect((await sync.loadState()).pending).toHaveLength(1);
 });
});
