const status=document.getElementById('status');
document.getElementById('sync').addEventListener('click',async()=>{
 try{
  const pair=document.getElementById('pair').value.trim();if(pair.length<32)throw Error('请先从桌面应用复制配对码');
  const [tab]=await chrome.tabs.query({active:true,currentWindow:true});
  if(!tab?.id||!tab.url)throw Error('无法读取当前标签页');
  const url=new URL(tab.url),origin=url.origin+'/*';
  if(!['http:','https:'].includes(url.protocol))throw Error('仅支持 HTTP(S) 样本页');
  if(!await chrome.permissions.request({origins:[origin]}))throw Error('站点授权已取消');
  const [result]=await chrome.scripting.executeScript({target:{tabId:tab.id},func:()=>{
   const node=document.querySelector('[data-aihub-sample="cursor"]');
   if(!node)return null;
   const value=Number(node.getAttribute('data-used-percent'));
   return Number.isFinite(value)&&value>=0&&value<=100?{used_percent:value,observed_at:new Date().toISOString()}:null;
  }});
  if(!result?.result)throw Error('当前不是已支持的固定样本页，或字段无效');
  const response=await fetch('http://127.0.0.1:47831/sample/cursor',{method:'POST',headers:{'Content-Type':'application/json','X-AIHub-Pair':pair},body:JSON.stringify(result.result)});
  if(!response.ok)throw Error(response.status===401?'配对码错误或桌面 Bridge 未运行':`上报失败 (${response.status})`);
  const data=await response.json();status.textContent=data.applied?'样本已记录；不会计入真实额度。':'相同样本已存在。';
 }catch(e){status.textContent=e.message||String(e)}
});
