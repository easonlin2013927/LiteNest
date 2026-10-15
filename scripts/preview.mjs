import http from 'node:http';
import fs from 'node:fs/promises';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
const web = path.join(root, 'web');
const port = Number(process.env.PORT || 4173);
const folders = new Set();
const users = [{ username:'admin', admin:true, permissions:{read:true,write:true,delete:true}, quota_bytes:0, used_bytes:0, created_at:new Date().toISOString() }];

function json(response, status, value) {
  response.writeHead(status, {'Content-Type':'application/json; charset=utf-8'});
  response.end(value == null ? '' : JSON.stringify(value));
}
async function body(request) {
  const chunks=[]; for await (const chunk of request) chunks.push(chunk);
  try { return JSON.parse(Buffer.concat(chunks).toString() || '{}'); } catch { return {}; }
}
function authenticated(request) { return request.headers.cookie?.includes('litenest_preview=1'); }

const server=http.createServer(async (request,response)=>{
  const url=new URL(request.url,'http://localhost');
  if(url.pathname==='/api/login'&&request.method==='POST'){
    const input=await body(request);
    if(input.username!=='admin'||input.password!=='admin') return json(response,401,{error:'帳號或密碼錯誤'});
    response.setHeader('Set-Cookie','litenest_preview=1; Path=/; HttpOnly; SameSite=Strict');
    return json(response,200,users[0]);
  }
  if(url.pathname.startsWith('/api/')&&!authenticated(request)) return json(response,401,{error:'請先登入'});
  if(url.pathname==='/api/me') return json(response,200,{...users[0],must_change_password:true});
  if(url.pathname==='/api/logout'){response.setHeader('Set-Cookie','litenest_preview=; Path=/; Max-Age=0');return json(response,204);}
  if(url.pathname==='/api/password') return json(response,204);
  if(url.pathname==='/api/health') return json(response,200,{status:'ok',service:'litenest-preview'});
  if(url.pathname==='/api/files'&&request.method==='GET'){
    const current=url.searchParams.get('path')||'';
    const items=[...folders].filter(item=>path.posix.dirname(item)===(current||'.')).map(item=>({name:path.posix.basename(item),path:item,is_dir:true,size:0,modified:new Date().toISOString()}));
    return json(response,200,{path:current,items});
  }
  if(url.pathname==='/api/folders'&&request.method==='POST'){const input=await body(request);folders.add(input.path);return json(response,201,{path:input.path});}
  if(url.pathname==='/api/upload'&&request.method==='POST') return json(response,201,{uploaded:['preview-file']});
  if(url.pathname==='/api/files'&&request.method==='DELETE'){folders.delete(url.searchParams.get('path'));return json(response,204);}
  if(url.pathname==='/api/users'&&request.method==='GET') return json(response,200,users);
  if(url.pathname==='/api/users'&&request.method==='POST'){const input=await body(request);const user={...input,used_bytes:0,created_at:new Date().toISOString()};delete user.password;users.push(user);return json(response,201,user);}
  if(url.pathname.startsWith('/api/users/')&&request.method==='PATCH'){const input=await body(request);const name=decodeURIComponent(url.pathname.split('/').pop());const user=users.find(item=>item.username===name);Object.assign(user,input);delete user.password;return json(response,200,user);}
  if(url.pathname.startsWith('/api/users/')&&request.method==='DELETE'){const name=decodeURIComponent(url.pathname.split('/').pop());const index=users.findIndex(item=>item.username===name);if(index>0)users.splice(index,1);return json(response,204);}

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
