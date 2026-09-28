import {beforeEach,it,expect,vi} from "vitest";
const mocks=vi.hoisted(()=>({session:null as any}));
vi.mock("../src/session",()=>({
 profile:()=>({id:"test",url:"https://test.invalid"}),
 getSession:()=>mocks.session,
 saveSession:(s:any)=>{mocks.session=s;}
}));
beforeEach(()=>{
 vi.resetModules();mocks.session={token:"old",refresh_token:"refresh",device_id:"device",user:{id:"user",email:"test@example.com"}};
 vi.stubGlobal("navigator",{locks:{request:async(_name:string,fn:()=>unknown)=>fn()}});
});
it("refreshes an expired access token once and retries the protected request",async()=>{
 const fetcher=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({error:"unauthenticated"}),{status:401}))
  .mockResolvedValueOnce(new Response(JSON.stringify({...mocks.session,token:"new",refresh_token:"next"})))
  .mockResolvedValueOnce(new Response(JSON.stringify({ok:true})));
 vi.stubGlobal("fetch",fetcher);const{request}=await import("../src/api");
 expect(await request("/api/v1/me")).toEqual({ok:true});expect(mocks.session.token).toBe("new");
 expect(fetcher.mock.calls[2][1].headers.get("Authorization")).toBe("Bearer new");
});
it("stops further network activity after revoked refresh while retaining offline identity",async()=>{
 const fetcher=vi.fn().mockResolvedValueOnce(new Response("{}",{status:401})).mockResolvedValueOnce(new Response("{}",{status:401}));
 vi.stubGlobal("fetch",fetcher);const{request}=await import("../src/api");
 await expect(request("/api/v1/sync/pull")).rejects.toMatchObject({code:"session_revoked"});
 expect(mocks.session.blocked).toBe(true);expect(mocks.session.user.id).toBe("user");
 await expect(request("/api/v1/sync/pull")).rejects.toMatchObject({code:"session_revoked"});
 expect(fetcher).toHaveBeenCalledTimes(2);
});
