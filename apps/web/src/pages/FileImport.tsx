import {useState} from "react";
import {enqueueManual} from "../manualQueue";

type Row={observed_at?:string;remaining_value?:number|null;limit_value?:number|null;reset_at?:string;expires_at?:string;note?:string};
const columns=['observed_at','remaining_value','limit_value','reset_at','expires_at','note'];
function csvLines(input:string):string[][]{
 const rows:string[][]=[];let row:string[]=[],field='',quoted=false;
 for(let i=0;i<input.length;i++){const c=input[i];if(c==='"'){if(quoted&&input[i+1]==='"'){field+='"';i++;}else quoted=!quoted;}else if(c===','&&!quoted){row.push(field);field='';}else if((c==='\n'||c==='\r')&&!quoted){if(c==='\r'&&input[i+1]==='\n')i++;row.push(field);if(row.some(v=>v.trim()))rows.push(row);row=[];field='';}else field+=c;}
 if(quoted)throw Error('CSV 引号未闭合');row.push(field);if(row.some(v=>v.trim()))rows.push(row);return rows;
}
function parse(text:string,name:string):Row[]{
 let records:unknown[];
 if(name.toLowerCase().endsWith('.json')){const value=JSON.parse(text);records=Array.isArray(value)?value:[value];}
 else if(name.toLowerCase().endsWith('.csv')){const [header,...lines]=csvLines(text);if(!header)throw Error('文件为空');const keys=header.map(x=>x.trim());if(keys.some(x=>!columns.includes(x)))throw Error('含有未支持的字段');records=lines.map(fields=>Object.fromEntries(keys.map((k,i)=>[k,fields[i]??''])));}
 else throw Error('仅支持 CSV 或 JSON 文件');
 if(!records.length||records.length>500)throw Error('一次导入需要 1–500 条记录');
 return records.map((value,i)=>{
  if(!value||typeof value!=='object'||Array.isArray(value))throw Error(`第 ${i+1} 条格式无效`);
  const raw=value as Record<string,unknown>;if(Object.keys(raw).some(k=>!columns.includes(k)))throw Error(`第 ${i+1} 条含未支持字段`);
  const row:Row={};for(const k of columns){const v=raw[k];if(v==null||v==='')continue;
   if(k==='remaining_value'||k==='limit_value'){const n=Number(v);if(!Number.isFinite(n)||n<0)throw Error(`第 ${i+1} 条 ${k} 需要非负数字`);(row as Record<string,unknown>)[k]=n;}
   else if(k==='note'){if(typeof v!=='string'||v.length>300)throw Error(`第 ${i+1} 条备注过长`);row.note=v;}
   else {if(typeof v!=='string'||!Number.isFinite(Date.parse(v)))throw Error(`第 ${i+1} 条 ${k} 时间无效`);(row as Record<string,unknown>)[k]=new Date(v).toISOString();}
  }
  if(row.remaining_value==null&&row.limit_value==null)throw Error(`第 ${i+1} 条缺少额度字段`);
  return row;
 });
}
async function identifier(bucketId:string,row:Row){const bytes=new TextEncoder().encode(bucketId+JSON.stringify(row));const hash=await crypto.subtle.digest('SHA-256',bytes);return 'file-'+Array.from(new Uint8Array(hash),x=>x.toString(16).padStart(2,'0')).join('');}
export default function FileImport({bucketId,onComplete}:{bucketId:string;onComplete:()=>Promise<void>}){
 const[rows,setRows]=useState<Row[]>([]),[message,setMessage]=useState(''),[busy,setBusy]=useState(false);
 async function select(file?:File){if(!file)return;setMessage('');try{if(file.size>256000)throw Error('文件超过 256 KB');setRows(parse(await file.text(),file.name));}catch(e){setRows([]);setMessage(e instanceof Error?e.message:'文件无法读取');}}
 async function submit(){setBusy(true);try{for(const row of rows)await enqueueManual(bucketId,{...row,source_type:'file_import',operation_id:await identifier(bucketId,row)});setMessage(`已处理 ${rows.length} 条；离线记录会在恢复连接后同步。`);setRows([]);await onComplete();}catch(e){setMessage(e instanceof Error?e.message:'导入失败')}finally{setBusy(false)}}
 return <section className="panel"><h2>文件导入</h2><p className="muted">仅解析额度字段；请先检查预览。相同记录再次导入不会生成重复快照。</p><input type="file" accept=".csv,.json" onChange={e=>void select(e.target.files?.[0])}/>{rows.length>0&&<><p>预览 {rows.length} 条（显示前 5 条）</p><table className="table"><thead><tr><th>时间</th><th>剩余</th><th>总额度</th><th>备注</th></tr></thead><tbody>{rows.slice(0,5).map((r,i)=><tr key={i}><td>{r.observed_at||'导入时'}</td><td>{r.remaining_value??'—'}</td><td>{r.limit_value??'—'}</td><td>{r.note||'—'}</td></tr>)}</tbody></table><button className="btn" disabled={busy} onClick={()=>void submit()}>{busy?'正在导入…':'确认导入'}</button></>}{message&&<p role="status">{message}</p>}</section>;
}
