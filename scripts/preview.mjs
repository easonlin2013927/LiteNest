import http from 'node:http';
import fs from 'node:fs/promises';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
const web = path.join(root, 'web');
const port = Number(process.argv[2] || process.env.PORT || 4173);
const folders = new Set();
const files = new Map();
const trash = new Map();
let nextTrashID = 1;
const users = [{ username:'admin', password:'admin', admin:true, permissions:{read:true,write:true,delete:true}, quota_bytes:0, glass_level:null, theme:null, skip_delete_confirm:false, used_bytes:0, created_at:new Date().toISOString() }];
let settings = {default_permissions:{read:true,write:true,delete:false},default_quota_bytes:0,default_glass_level:70,default_theme:'sage'};
const globalLog = [];
const activityLog = [];
function record(list,actor,action,detail='') { list.unshift({time:new Date().toISOString(),actor,action,detail}); }
function publicUser(user) { const {password,...safe}=user; return {...safe,used_bytes:[...files.entries()].filter(([name])=>name.startsWith(`${user.username}/`)).reduce((sum,[,file])=>sum+file.size,0)+[...trash.values()].filter(item=>item.username===user.username).reduce((sum,item)=>sum+item.size,0)}; }
function itemKey(username,relative) { return `${username}/${relative}`; }

function json(response, status, value) {
  response.writeHead(status, {'Content-Type':'application/json; charset=utf-8'});
  response.end(value == null ? '' : JSON.stringify(value));
}
async function body(request) {
  const chunks=[]; for await (const chunk of request) chunks.push(chunk);
  try { return JSON.parse(Buffer.concat(chunks).toString() || '{}'); } catch { return {}; }
}
function currentUser(request) { const name=/litenest_preview=([^;]+)/.exec(request.headers.cookie||'')?.[1]; return users.find(user=>user.username===name); }

const server=http.createServer(async (request,response)=>{
  const url=new URL(request.url,'http://localhost');
  if(url.pathname==='/api/login'&&request.method==='POST'){
    const input=await body(request);
    const account=users.find(user=>user.username===input.username&&user.password===input.password);
    if(!account) return json(response,401,{error:'帳號或密碼錯誤'});
    response.setHeader('Set-Cookie',`litenest_preview=${account.username}; Path=/; HttpOnly; SameSite=Strict`);
    record(activityLog,account.username,'登入');
    return json(response,200,publicUser(account));
  }
  if(url.pathname==='/api/health') return json(response,200,{status:'ok',service:'litenest-preview'});
  const actor=currentUser(request);
  if(url.pathname.startsWith('/api/')&&!actor) return json(response,401,{error:'請先登入'});
  if(url.pathname==='/api/me') return json(response,200,{...publicUser(actor),default_glass_level:settings.default_glass_level,default_theme:settings.default_theme,must_change_password:actor.username==='admin'&&actor.password==='admin'});
  if(url.pathname==='/api/logout'){record(activityLog,actor.username,'登出');response.setHeader('Set-Cookie','litenest_preview=; Path=/; Max-Age=0');return json(response,204);}
  if(url.pathname==='/api/password'){const input=await body(request);if(input.current_password!==actor.password)return json(response,400,{error:'目前密碼錯誤'});actor.password=input.new_password;record(activityLog,actor.username,'變更密碼');return json(response,204);}
  if(url.pathname==='/api/settings/global'&&request.method==='GET') return actor.admin?json(response,200,settings):json(response,403,{error:'需要管理者權限'});
  if(url.pathname==='/api/settings/global'&&request.method==='PATCH'){if(!actor.admin)return json(response,403,{error:'需要管理者權限'});settings=await body(request);record(globalLog,actor.username,'更新全域預設設定');return json(response,200,settings);}
  if(url.pathname==='/api/settings/me'&&request.method==='PATCH'){const input=await body(request);if('use_global' in input)actor.glass_level=input.use_global?null:input.glass_level;if('use_global_theme' in input)actor.theme=input.use_global_theme?null:input.theme;if('skip_delete_confirm' in input)actor.skip_delete_confirm=input.skip_delete_confirm;record(activityLog,actor.username,'變更個人顯示設定');return json(response,200,{...publicUser(actor),default_glass_level:settings.default_glass_level,default_theme:settings.default_theme});}
  if(url.pathname==='/api/logs/global') return actor.admin?json(response,200,globalLog):json(response,403,{error:'需要管理者權限'});
  if(url.pathname==='/api/logs/activity') return json(response,200,actor.admin&&url.searchParams.get('scope')==='all'?activityLog:activityLog.filter(entry=>entry.actor===actor.username));
  if(url.pathname==='/api/files'&&request.method==='GET'){
    const current=url.searchParams.get('path')||'';
    const ownFolders=[...folders].filter(item=>item.startsWith(`${actor.username}/`)).map(item=>item.slice(actor.username.length+1));
    const items=ownFolders.filter(item=>path.posix.dirname(item)===(current||'.')).map(item=>({name:path.posix.basename(item),path:item,is_dir:true,size:0,modified:new Date().toISOString()}));
    for(const [key,file] of files) { if(!key.startsWith(`${actor.username}/`))continue; const relative=key.slice(actor.username.length+1); if(path.posix.dirname(relative)===(current||'.'))items.push({name:path.posix.basename(relative),path:relative,is_dir:false,size:file.size,modified:file.modified}); }
    items.sort((a,b)=>Number(b.is_dir)-Number(a.is_dir)||a.name.localeCompare(b.name,'zh-TW'));
    return json(response,200,{path:current,items});
  }
  if(url.pathname==='/api/folders'&&request.method==='POST'){const input=await body(request);const key=itemKey(actor.username,input.path);if(folders.has(key)||files.has(key))return json(response,409,{error:'同名項目已存在'});folders.add(key);record(activityLog,actor.username,'建立資料夾',input.path);return json(response,201,{path:input.path});}
  if(url.pathname==='/api/upload'&&request.method==='POST'){
    const current=url.searchParams.get('path')||'';
    if(current&&!folders.has(itemKey(actor.username,current)))return json(response,400,{error:'目的資料夾不存在'});
    let form;try{form=await new Request('http://localhost/api/upload',{method:'POST',headers:request.headers,body:request,duplex:'half'}).formData();}catch{return json(response,400,{error:'上傳格式無效'});}
    const uploaded=[];
    for(const file of form.getAll('files')){
      if(typeof file==='string'||!file.name)continue;
      const relative=path.posix.join(current,path.posix.basename(file.name));
      const key=itemKey(actor.username,relative);
      if(folders.has(key))return json(response,409,{error:'同名資料夾已存在'});
      files.set(key,{data:Buffer.from(await file.arrayBuffer()),size:file.size,modified:new Date().toISOString()});
      uploaded.push(file.name);record(activityLog,actor.username,'上傳檔案',relative);
    }
    if(!uploaded.length)return json(response,400,{error:'沒有選擇檔案'});
    return json(response,201,{uploaded});
  }
  if(url.pathname==='/api/download'&&request.method==='GET'){
    const relative=url.searchParams.get('path')||'';const file=files.get(itemKey(actor.username,relative));
    if(!file)return json(response,404,{error:'檔案不存在'});
    response.writeHead(200,{'Content-Type':'application/octet-stream','Content-Disposition':`attachment; filename="${path.posix.basename(relative).replaceAll('"','')}"`});record(activityLog,actor.username,'下載檔案',relative);return response.end(file.data);
  }
  if(url.pathname==='/api/preview'&&request.method==='GET'){
    const relative=url.searchParams.get('path')||'';const file=files.get(itemKey(actor.username,relative));
    if(!file)return json(response,404,{error:'檔案不存在'});
    const ext=path.posix.extname(relative).toLowerCase();
    const types={'.jpg':'image/jpeg','.jpeg':'image/jpeg','.png':'image/png','.gif':'image/gif','.webp':'image/webp','.bmp':'image/bmp','.avif':'image/avif','.pdf':'application/pdf','.mp3':'audio/mpeg','.m4a':'audio/mp4','.wav':'audio/wav','.ogg':'audio/ogg','.flac':'audio/flac','.mp4':'video/mp4','.m4v':'video/mp4','.webm':'video/webm','.mov':'video/quicktime','.xlsx':'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet','.ods':'application/vnd.oasis.opendocument.spreadsheet'};
    const textExtensions=new Set(['.md','.markdown','.csv','.tsv','.py','.cmd','.bat','.ps1','.js','.mjs','.cjs','.json','.css','.html','.htm','.xml','.svg','.go','.sh','.bash','.sql','.yml','.yaml','.toml','.ini','.conf','.log','.txt','.ts','.tsx','.jsx','.rs','.c','.h','.cpp','.java','.rb','.php']);
    const officeTypes={'.docx':'application/vnd.openxmlformats-officedocument.wordprocessingml.document','.pptx':'application/vnd.openxmlformats-officedocument.presentationml.presentation','.odt':'application/vnd.oasis.opendocument.text','.odp':'application/vnd.oasis.opendocument.presentation'};
    const type=types[ext]||officeTypes[ext]||(textExtensions.has(ext)?'text/plain; charset=utf-8':'application/octet-stream');
    if(!type)return json(response,415,{error:'此格式不支援站內預覽'});
    record(activityLog,actor.username,'預覽檔案',relative);
    response.writeHead(200,{'Content-Type':type,'Content-Length':file.size,'Cache-Control':'private, no-store','X-Content-Type-Options':'nosniff'});return response.end(file.data);
  }
  if(url.pathname==='/api/trash'&&request.method==='GET'){
    if(!actor.admin&&!actor.permissions.read&&!actor.permissions.delete)return json(response,403,{error:'沒有讀取權限'});
    const items=[...trash.values()].filter(item=>item.username===actor.username).map(({folders:oldFolders,files:oldFiles,username,...item})=>item);
    return json(response,200,items.reverse());
  }
  if(url.pathname==='/api/trash/restore'&&request.method==='POST'){
    if(!actor.admin&&!actor.permissions.write)return json(response,403,{error:'沒有寫入權限'});
    const {ids}=await body(request);if(!Array.isArray(ids)||!ids.length)return json(response,400,{error:'請先選擇項目'});
    const done=[],failed=[];
    const items=ids.map(id=>trash.get(id)).filter(item=>item?.username===actor.username).sort((a,b)=>a.original_path.split('/').length-b.original_path.split('/').length);
    for(const item of items){const key=itemKey(actor.username,item.original_path),parent=path.posix.dirname(item.original_path);if(folders.has(key)||files.has(key)){failed.push({id:item.id,error:'原位置已有同名項目'});continue}if(parent!=='.'&&!folders.has(itemKey(actor.username,parent))){failed.push({id:item.id,error:'原始資料夾不存在'});continue}for(const folder of item.folders)folders.add(itemKey(actor.username,folder));for(const [name,file] of item.files)files.set(itemKey(actor.username,name),file);trash.delete(item.id);done.push(item.id);record(activityLog,actor.username,'從垃圾桶還原',item.original_path)}
    for(const id of ids)if(!done.includes(id)&&!failed.some(item=>item.id===id))failed.push({id,error:'項目不存在'});
    return json(response,200,{done,failed});
  }
  if(url.pathname==='/api/trash'&&request.method==='DELETE'){
    if(!actor.admin&&!actor.permissions.delete)return json(response,403,{error:'沒有刪除權限'});
    const {ids}=await body(request);if(!Array.isArray(ids)||!ids.length)return json(response,400,{error:'請先選擇項目'});
    const done=[],failed=[];for(const id of ids){const item=trash.get(id);if(!item||item.username!==actor.username){failed.push({id,error:'項目不存在'});continue}trash.delete(id);done.push(id);record(activityLog,actor.username,'永久刪除',item.original_path)}return json(response,200,{done,failed});
  }
  if(url.pathname==='/api/files'&&request.method==='DELETE'){
    if(!actor.admin&&!actor.permissions.delete)return json(response,403,{error:'沒有刪除權限'});
    const item=url.searchParams.get('path')||'';if(!item)return json(response,400,{error:'路徑無效'});
    const key=itemKey(actor.username,item);if(!folders.has(key)&&!files.has(key))return json(response,404,{error:'項目不存在'});
    const oldFolders=[...folders].filter(name=>name===key||name.startsWith(`${key}/`)).map(name=>name.slice(actor.username.length+1));
    const oldFiles=[...files.entries()].filter(([name])=>name===key||name.startsWith(`${key}/`)).map(([name,file])=>[name.slice(actor.username.length+1),file]);
    for(const folder of oldFolders)folders.delete(itemKey(actor.username,folder));for(const [name] of oldFiles)files.delete(itemKey(actor.username,name));
    const id=(nextTrashID++).toString(16).padStart(32,'0');trash.set(id,{id,username:actor.username,name:path.posix.basename(item),original_path:item,is_dir:oldFolders.includes(item),size:oldFiles.reduce((sum,[,file])=>sum+file.size,0),deleted_at:new Date().toISOString(),folders:oldFolders,files:oldFiles});
    record(activityLog,actor.username,'移到垃圾桶',item);return json(response,204);
  }
  if(url.pathname==='/api/users'&&request.method==='GET') return actor.admin?json(response,200,users.map(publicUser)):json(response,403,{error:'需要管理者權限'});
  if(url.pathname==='/api/users'&&request.method==='POST'){if(!actor.admin)return json(response,403,{error:'需要管理者權限'});const input=await body(request);const username=String(input.username||'').trim().toLowerCase();if(users.some(user=>user.username.toLowerCase()===username))return json(response,409,{error:'使用者已存在'});if(!/^[a-z0-9_-]{3,32}$/.test(username))return json(response,400,{error:'帳號需為 3–32 個小寫英數字元、連字號或底線'});const user={...input,username,permissions:input.permissions??settings.default_permissions,quota_bytes:input.quota_bytes??settings.default_quota_bytes,glass_level:input.glass_level??null,theme:null,skip_delete_confirm:false,used_bytes:0,created_at:new Date().toISOString()};users.push(user);record(globalLog,actor.username,'建立使用者',user.username);return json(response,201,publicUser(user));}
  if(url.pathname.startsWith('/api/users/')&&request.method==='PATCH'){if(!actor.admin)return json(response,403,{error:'需要管理者權限'});const input=await body(request);const name=decodeURIComponent(url.pathname.split('/').pop());const user=users.find(item=>item.username===name);Object.assign(user,input);if(input.use_global_glass)user.glass_level=null;record(globalLog,actor.username,'更新使用者',name);return json(response,200,publicUser(user));}
  if(url.pathname.startsWith('/api/users/')&&request.method==='DELETE'){if(!actor.admin)return json(response,403,{error:'需要管理者權限'});const name=decodeURIComponent(url.pathname.split('/').pop());const index=users.findIndex(item=>item.username===name);if(index>0)users.splice(index,1);record(globalLog,actor.username,'刪除使用者',name);return json(response,204);}

  const requested=url.pathname==='/'?'index.html':url.pathname.slice(1);
  const file=path.resolve(web,requested);
  if(!file.startsWith(web)) return response.writeHead(403).end();
  try{
    const data=await fs.readFile(file);
    const type=file.endsWith('.css')?'text/css':file.endsWith('.js')?'text/javascript':'text/html';
    response.writeHead(200,{'Content-Type':type+'; charset=utf-8'});response.end(data);
  }catch{response.writeHead(404).end('Not found');}
});

server.listen(port,'127.0.0.1',()=>{
  console.log('LiteNest preview is running');
  console.log(`URL: http://127.0.0.1:${port}`);
  console.log('Login: admin / admin');
  console.log('Close this window to stop the preview.');
});
