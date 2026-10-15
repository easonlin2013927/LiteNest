const $ = (selector) => document.querySelector(selector);
const state = { me: null, path: '', items: [], users: [] };
let toastTimer;

async function api(url, options = {}) {
  const settings = { credentials: 'same-origin', ...options };
  if (settings.body && !(settings.body instanceof FormData)) {
    settings.headers = { 'Content-Type': 'application/json', ...(settings.headers || {}) };
    settings.body = JSON.stringify(settings.body);
  }
  const response = await fetch(url, settings);
  if (response.status === 401 && url !== '/api/login') { showLogin(); throw new Error('請重新登入'); }
  if (!response.ok) {
    let message = '操作失敗';
    try { message = (await response.json()).error || message; } catch (_) {}
    throw new Error(message);
  }
  if (response.status === 204) return null;
  return response.json();
}

function showToast(message) {
  const toast = $('#toast'); toast.textContent = message; toast.classList.add('show');
  clearTimeout(toastTimer); toastTimer = setTimeout(() => toast.classList.remove('show'), 2600);
}
function showLogin() { $('#loginView').classList.remove('hidden'); $('#appView').classList.add('hidden'); }
function showApp() { $('#loginView').classList.add('hidden'); $('#appView').classList.remove('hidden'); }
function formatBytes(value) {
  if (!value) return '0 B'; const units = ['B','KB','MB','GB','TB']; const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / Math.pow(1024,index)).toFixed(index ? 1 : 0)} ${units[index]}`;
}
function formatDate(value) { return new Intl.DateTimeFormat('zh-TW',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(value)); }
function escapeHtml(value) { return String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }

async function bootstrap() {
  try { state.me = await api('/api/me'); showApp(); applyProfile(); await loadFiles(''); }
  catch (_) { showLogin(); }
}
function applyProfile() {
  const me = state.me; $('#profileName').textContent = me.username; $('#profileRole').textContent = me.admin ? '管理者' : '使用者'; $('#avatar').textContent = me.username[0].toUpperCase();
  $('#usersNav').classList.toggle('hidden', !me.admin); $('#warningBanner').classList.toggle('hidden', !me.must_change_password);
  const quota = me.quota_bytes; $('#storageText').textContent = quota ? `${formatBytes(me.used_bytes)} / ${formatBytes(quota)}` : `${formatBytes(me.used_bytes)} / 不限`;
  $('#storageBar').style.width = quota ? `${Math.min(100, me.used_bytes / quota * 100)}%` : '8%';
  const canWrite = me.admin || me.permissions.write; $('#newFolderButton').classList.toggle('hidden', !canWrite); $('#uploadLabel').classList.toggle('hidden', !canWrite);
}

$('#loginForm').addEventListener('submit', async event => {
  event.preventDefault(); $('#loginError').textContent = '';
  try { state.me = await api('/api/login',{method:'POST',body:{username:$('#loginUsername').value,password:$('#loginPassword').value}}); state.me = await api('/api/me'); showApp(); applyProfile(); await loadFiles(''); }
  catch (error) { $('#loginError').textContent = error.message; }
});
$('#logoutButton').addEventListener('click', async () => { try { await api('/api/logout',{method:'POST'}); } finally { state.me=null; showLogin(); } });

async function loadFiles(newPath = state.path) {
  try { const result = await api(`/api/files?path=${encodeURIComponent(newPath)}`); state.path = result.path; state.items = result.items || []; renderBreadcrumbs(); renderFiles(); }
  catch (error) { showToast(error.message); }
}
function renderBreadcrumbs() {
  const parts = state.path ? state.path.split('/') : []; let current = '';
  const nodes = [`<button data-path="">我的檔案</button>`];
  parts.forEach(part => { current = current ? `${current}/${part}` : part; nodes.push('<i>›</i>',`<button data-path="${escapeHtml(current)}">${escapeHtml(part)}</button>`); });
  $('#breadcrumbs').innerHTML = nodes.join(''); $('#breadcrumbs').querySelectorAll('button').forEach(button => button.addEventListener('click', () => loadFiles(button.dataset.path)));
}
function renderFiles() {
  const query = $('#searchInput').value.trim().toLowerCase(); const items = state.items.filter(item => item.name.toLowerCase().includes(query));
  $('#emptyState').classList.toggle('hidden', items.length > 0); $('#fileList').innerHTML = items.map(item => {
    const icon = item.is_dir ? '▰' : fileIcon(item.name); const size = item.is_dir ? '資料夾' : formatBytes(item.size);
    const open = item.is_dir ? `data-open="${escapeHtml(item.path)}"` : `data-download="${escapeHtml(item.path)}"`;
    const download = item.is_dir ? '' : `<a href="/api/download?path=${encodeURIComponent(item.path)}">下載</a>`;
    const remove = state.me.admin || state.me.permissions.delete ? `<button class="danger delete-item" data-delete="${escapeHtml(item.path)}">刪除</button>` : '';
    return `<div class="file-row"><div class="file-name"><span class="file-icon">${icon}</span><button ${open}>${escapeHtml(item.name)}</button></div><span class="file-meta">${size}</span><span class="file-meta">${formatDate(item.modified)}</span><div class="row-actions"><button class="more" aria-label="更多操作">⋯</button><div class="action-menu hidden">${download}${remove}</div></div></div>`;
  }).join('');
  $('#fileList').querySelectorAll('[data-open]').forEach(el => el.addEventListener('click', () => loadFiles(el.dataset.open)));
  $('#fileList').querySelectorAll('[data-download]').forEach(el => el.addEventListener('click', () => { location.href = `/api/download?path=${encodeURIComponent(el.dataset.download)}`; }));
  $('#fileList').querySelectorAll('.more').forEach(button => button.addEventListener('click', event => { event.stopPropagation(); document.querySelectorAll('.action-menu').forEach(menu => { if (menu !== button.nextElementSibling) menu.classList.add('hidden'); }); button.nextElementSibling.classList.toggle('hidden'); }));
  $('#fileList').querySelectorAll('.delete-item').forEach(button => button.addEventListener('click', () => deleteItem(button.dataset.delete)));
}
function fileIcon(name) { const ext = name.split('.').pop().toLowerCase(); if (['jpg','jpeg','png','gif','webp','svg'].includes(ext)) return '◫'; if (['mp4','mov','mkv','avi'].includes(ext)) return '▷'; if (['zip','rar','7z','tar','gz'].includes(ext)) return '▤'; return '▧'; }
$('#searchInput').addEventListener('input', renderFiles);
document.addEventListener('click', () => document.querySelectorAll('.action-menu').forEach(menu => menu.classList.add('hidden')));
async function deleteItem(itemPath) { if (!confirm(`確定要刪除「${itemPath.split('/').pop()}」？此操作無法復原。`)) return; try { await api(`/api/files?path=${encodeURIComponent(itemPath)}`,{method:'DELETE'}); showToast('已刪除'); await refreshFilesAndUsage(); } catch(error) { showToast(error.message); } }

$('#newFolderButton').addEventListener('click', () => { $('#folderName').value='';$('#folderError').textContent='';$('#folderDialog').showModal();setTimeout(()=>$('#folderName').focus(),50); });
$('#folderForm').addEventListener('submit', async event => { event.preventDefault(); const name=$('#folderName').value.trim(); if(!name)return; try { await api('/api/folders',{method:'POST',body:{path:state.path?`${state.path}/${name}`:name}}); $('#folderDialog').close();showToast('資料夾已建立');await loadFiles(); } catch(error) { $('#folderError').textContent=error.message; } });

$('#fileInput').addEventListener('change', event => uploadFiles(event.target.files));
const dropZone=$('#dropZone'); ['dragenter','dragover'].forEach(type=>dropZone.addEventListener(type,event=>{event.preventDefault();dropZone.classList.add('dragging')})); ['dragleave','drop'].forEach(type=>dropZone.addEventListener(type,event=>{event.preventDefault();dropZone.classList.remove('dragging')})); dropZone.addEventListener('drop',event=>uploadFiles(event.dataTransfer.files));
async function uploadFiles(files) { if(!files.length)return; const data=new FormData(); [...files].forEach(file=>data.append('files',file)); showToast(`正在上傳 ${files.length} 個檔案…`); try { await api(`/api/upload?path=${encodeURIComponent(state.path)}`,{method:'POST',body:data});$('#fileInput').value='';showToast('上傳完成');await refreshFilesAndUsage(); } catch(error) { showToast(error.message); } }
async function refreshFilesAndUsage(){await loadFiles();state.me=await api('/api/me');applyProfile()}

function openPassword(){ $('#currentPassword').value='';$('#newPassword').value='';$('#passwordError').textContent='';$('#passwordDialog').showModal(); }
$('#changePasswordButton').addEventListener('click',openPassword);$('#warningAction').addEventListener('click',openPassword);
$('#passwordForm').addEventListener('submit',async event=>{event.preventDefault();try{await api('/api/password',{method:'POST',body:{current_password:$('#currentPassword').value,new_password:$('#newPassword').value}});$('#passwordDialog').close();state.me.must_change_password=false;applyProfile();showToast('密碼已更新');}catch(error){$('#passwordError').textContent=error.message;}});

document.querySelectorAll('.nav-item').forEach(button=>button.addEventListener('click',async()=>{document.querySelectorAll('.nav-item').forEach(x=>x.classList.remove('active'));button.classList.add('active');const users=button.dataset.view==='users';$('#filesView').classList.toggle('hidden',users);$('#usersView').classList.toggle('hidden',!users);$('#pageTitle').textContent=users?'系統設定':'我的檔案';$('.top-actions').classList.toggle('hidden',users);$('.sidebar').classList.remove('open');if(users)await loadUsers();}));
$('#menuButton').addEventListener('click',()=>$('.sidebar').classList.toggle('open'));

async function loadUsers(){try{state.users=await api('/api/users');renderUsers();}catch(error){showToast(error.message)}}
function permissionBadges(user){if(user.admin)return '<span class="badge admin">管理者</span>';const out=[];if(user.permissions.read)out.push('<span class="badge">讀取</span>');if(user.permissions.write)out.push('<span class="badge">寫入</span>');if(user.permissions.delete)out.push('<span class="badge">刪除</span>');return out.join('')||'<span class="file-meta">無權限</span>'}
function renderUsers(){$('#userList').innerHTML=state.users.map(user=>{const quota=user.quota_bytes?formatBytes(user.quota_bytes):'不限';const canDelete=user.username!==state.me.username?`<button class="delete-user" data-delete-user="${escapeHtml(user.username)}" title="刪除">×</button>`:'';return `<div class="user-row"><div class="user-identity"><span class="avatar">${user.username[0].toUpperCase()}</span><span><strong>${escapeHtml(user.username)}</strong><small>${user.admin?'系統管理者':'一般使用者'}</small></span></div><div class="badges">${permissionBadges(user)}</div><div class="file-meta">${formatBytes(user.used_bytes)} / ${quota}</div><div class="user-actions"><button data-edit-user="${escapeHtml(user.username)}" title="編輯">✎</button>${canDelete}</div></div>`}).join('');document.querySelectorAll('[data-edit-user]').forEach(button=>button.addEventListener('click',()=>openUserDialog(state.users.find(user=>user.username===button.dataset.editUser))));document.querySelectorAll('[data-delete-user]').forEach(button=>button.addEventListener('click',()=>deleteUser(button.dataset.deleteUser)));}
$('#addUserButton').addEventListener('click',()=>openUserDialog());
function openUserDialog(user=null){$('#userForm').reset();$('#userError').textContent='';$('#editingUsername').value=user?.username||'';$('#userDialogTitle').textContent=user?'編輯使用者':'新增使用者';$('#userUsername').disabled=!!user;$('#userUsername').value=user?.username||'';$('#userPassword').required=!user;$('#userQuota').value=user?.quota_bytes?(user.quota_bytes/1073741824).toFixed(1):0;$('#userAdmin').checked=user?.admin||false;$('#permRead').checked=user?.permissions.read??true;$('#permWrite').checked=user?.permissions.write??true;$('#permDelete').checked=user?.permissions.delete??false;$('#userDialog').showModal();}
$('#userAdmin').addEventListener('change',event=>{document.querySelectorAll('#userForm fieldset input').forEach(input=>{input.disabled=event.target.checked;if(event.target.checked)input.checked=true})});
$('#userForm').addEventListener('submit',async event=>{event.preventDefault();const editing=$('#editingUsername').value;const body={password:$('#userPassword').value,admin:$('#userAdmin').checked,permissions:{read:$('#permRead').checked,write:$('#permWrite').checked,delete:$('#permDelete').checked},quota_bytes:Math.round(Number($('#userQuota').value)*1073741824)};if(!editing)body.username=$('#userUsername').value.trim();try{await api(editing?`/api/users/${encodeURIComponent(editing)}`:'/api/users',{method:editing?'PATCH':'POST',body});$('#userDialog').close();showToast(editing?'使用者已更新':'使用者已建立');await loadUsers();}catch(error){$('#userError').textContent=error.message;}});
async function deleteUser(username){if(!confirm(`確定刪除使用者「${username}」？檔案會移到伺服器的回收資料夾。`))return;try{await api(`/api/users/${encodeURIComponent(username)}`,{method:'DELETE'});showToast('使用者已刪除，檔案已保留於回收資料夾');await loadUsers();}catch(error){showToast(error.message)}}

bootstrap();
