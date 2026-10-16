/* Lightweight, dependency-free, read-only file previews. */
const LiteNestPreview = (() => {
  const dialog = document.querySelector('#previewDialog');
  const body = document.querySelector('#previewBody');
  const title = document.querySelector('#previewTitle');
  const meta = document.querySelector('#previewMeta');
  const download = document.querySelector('#previewDownload');
  const images = new Set(['jpg','jpeg','png','gif','webp','bmp','avif']);
  const audio = new Set(['mp3','m4a','wav','ogg','flac']);
  const video = new Set(['mp4','webm','mov','m4v']);
  const code = new Set(['py','cmd','bat','ps1','js','mjs','cjs','json','css','html','htm','xml','svg','go','sh','bash','sql','yml','yaml','toml','ini','conf','log','txt','ts','tsx','jsx','rs','c','h','cpp','java','rb','php']);
  let loadId = 0;

  function kind(name) {
    const ext = name.includes('.') ? name.split('.').pop().toLowerCase() : '';
    if (images.has(ext)) return 'image';
    if (audio.has(ext)) return 'audio';
    if (video.has(ext)) return 'video';
    if (ext === 'pdf') return 'pdf';
    if (ext === 'md' || ext === 'markdown') return 'markdown';
    if (['csv','tsv','xlsx','ods'].includes(ext)) return 'sheet';
    if (code.has(ext)) return 'code';
    return 'unknown';
  }
  function escape(value) { return String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
  function message(text) { const p=document.createElement('p');p.className='preview-message';p.textContent=text;body.replaceChildren(p); }
  function inlineMarkdown(text) {
    return escape(text)
      .replace(/`([^`]+)`/g,'<code>$1</code>')
      .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g,(_,label,url)=>`<a href="${url}" target="_blank" rel="noopener noreferrer">${label}</a>`)
      .replace(/\*\*([^*]+)\*\*/g,'<strong>$1</strong>')
      .replace(/\*([^*]+)\*/g,'<em>$1</em>');
  }
  function renderMarkdown(text) {
    const article=document.createElement('article');article.className='markdown-view';
    const lines=text.replace(/\r\n?/g,'\n').split('\n');
    const parts=[];let list='';let fenced=false;let fence=[];let paragraph=[];
    const closeList=()=>{if(list){parts.push(`</${list}>`);list='';}};
    const closeParagraph=()=>{if(paragraph.length){parts.push(`<p>${inlineMarkdown(paragraph.join(' '))}</p>`);paragraph=[];}};
    const cells=line=>line.trim().replace(/^\|/,'').replace(/\|$/,'').split('|').map(cell=>cell.trim());
    for(let i=0;i<lines.length;i++){
      const line=lines[i];
      if(/^\s*```/.test(line)){closeParagraph();closeList();if(fenced){parts.push(`<pre><code>${escape(fence.join('\n'))}</code></pre>`);fence=[];}fenced=!fenced;continue;}
      if(fenced){fence.push(line);continue;}
      if(line.includes('|')&&i+1<lines.length&&/^\s*\|?\s*:?-{3,}:?(\s*\|\s*:?-{3,}:?)*\s*\|?\s*$/.test(lines[i+1])){
        closeParagraph();closeList();const headings=cells(line);let table='<div class="markdown-table-wrap"><table><thead><tr>'+headings.map(cell=>`<th>${inlineMarkdown(cell)}</th>`).join('')+'</tr></thead><tbody>';i++;
        while(i+1<lines.length&&lines[i+1].includes('|')&&lines[i+1].trim()){i++;table+='<tr>'+cells(lines[i]).map(cell=>`<td>${inlineMarkdown(cell)}</td>`).join('')+'</tr>';}
        parts.push(table+'</tbody></table></div>');continue;
      }
      const heading=/^(#{1,6})\s+(.+)$/.exec(line);
      if(heading){closeParagraph();closeList();const level=heading[1].length;parts.push(`<h${level}>${inlineMarkdown(heading[2])}</h${level}>`);continue;}
      if(/^\s*---+\s*$/.test(line)){closeParagraph();closeList();parts.push('<hr>');continue;}
      const bullet=/^\s*[-*+]\s+(.+)$/.exec(line);const number=/^\s*\d+[.)]\s+(.+)$/.exec(line);
      if(bullet||number){closeParagraph();const next=bullet?'ul':'ol';if(list!==next){closeList();parts.push(`<${next}>`);list=next;}parts.push(`<li>${inlineMarkdown((bullet||number)[1])}</li>`);continue;}
      closeList();if(/^>\s?/.test(line)){closeParagraph();parts.push(`<blockquote>${inlineMarkdown(line.replace(/^>\s?/,''))}</blockquote>`);continue;}
      if(line.trim())paragraph.push(line.trim());else closeParagraph();
    }
    closeParagraph();closeList();if(fenced)parts.push(`<pre><code>${escape(fence.join('\n'))}</code></pre>`);
    article.innerHTML=parts.join('');body.replaceChildren(article);
  }
  function parseDelimited(text,delimiter) {
    const rows=[];let row=[],field='',quoted=false;
    for(let i=0;i<text.length&&rows.length<200;i++){
      const c=text[i];
      if(c==='"'){if(quoted&&text[i+1]==='"'){field+='"';i++;}else quoted=!quoted;}
      else if(c===delimiter&&!quoted){row.push(field.slice(0,2000));field='';}
      else if((c==='\n'||c==='\r')&&!quoted){if(c==='\r'&&text[i+1]==='\n')i++;row.push(field.slice(0,2000));rows.push(row.slice(0,50));row=[];field='';}
      else field+=c;
    }
    if(field||row.length){row.push(field.slice(0,2000));rows.push(row.slice(0,50));}
    return rows;
  }
  function renderSheets(sheets) {
    if(!sheets.length){message('這份試算表沒有可顯示的工作表。');return;}
    const wrap=document.createElement('div');wrap.className='sheet-view';
    const tabs=document.createElement('div');tabs.className='sheet-tabs';
    const tableWrap=document.createElement('div');tableWrap.className='sheet-scroll';
    function show(index){
      [...tabs.children].forEach((tab,i)=>tab.classList.toggle('active',i===index));
      const table=document.createElement('table');
      for(const [rowIndex,row] of sheets[index].rows.entries()){
        const tr=document.createElement('tr');
        for(const value of row.slice(0,50)){const cell=document.createElement(rowIndex===0?'th':'td');cell.textContent=String(value??'');tr.append(cell);}
        table.append(tr);
      }
      tableWrap.replaceChildren(table);
    }
    sheets.forEach((sheet,index)=>{const tab=document.createElement('button');tab.type='button';tab.textContent=sheet.name||`工作表 ${index+1}`;tab.addEventListener('click',()=>show(index));tabs.append(tab);});
    wrap.append(tabs,tableWrap);body.replaceChildren(wrap);show(0);
  }
  function zipEntries(buffer) {
    const bytes=new Uint8Array(buffer),view=new DataView(buffer);let end=-1;
    for(let i=bytes.length-22;i>=Math.max(0,bytes.length-65557);i--){if(view.getUint32(i,true)===0x06054b50){end=i;break;}}
    if(end<0)throw new Error('試算表 ZIP 格式無效');
    const result=new Map();let pos=view.getUint32(end+16,true),count=view.getUint16(end+10,true);
    for(let i=0;i<count;i++){
      if(pos+46>bytes.length||view.getUint32(pos,true)!==0x02014b50)throw new Error('試算表 ZIP 目錄無效');
      const method=view.getUint16(pos+10,true),compressed=view.getUint32(pos+20,true),size=view.getUint32(pos+24,true);
      const nameLen=view.getUint16(pos+28,true),extraLen=view.getUint16(pos+30,true),commentLen=view.getUint16(pos+32,true),offset=view.getUint32(pos+42,true);
      const name=new TextDecoder().decode(bytes.subarray(pos+46,pos+46+nameLen));
      if(name==='xl/sharedStrings.xml'||name==='xl/workbook.xml'||name==='content.xml'||/^xl\/worksheets\/sheet\d+\.xml$/.test(name))result.set(name,{method,compressed,size,offset});
      pos+=46+nameLen+extraLen+commentLen;
    }
    return {bytes,view,entries:result};
  }
  async function readZipText(zip,name) {
    const entry=zip.entries.get(name);if(!entry)return '';
    if(entry.compressed>8e6||entry.size>16e6)throw new Error('工作表太大，請下載後開啟');
    const {view,bytes}=zip,at=entry.offset;
    if(at+30>bytes.length||view.getUint32(at,true)!==0x04034b50)throw new Error('工作表內容無效');
    const start=at+30+view.getUint16(at+26,true)+view.getUint16(at+28,true);
    if(start+entry.compressed>bytes.length)throw new Error('工作表內容不完整');
    const chunk=bytes.slice(start,start+entry.compressed);
    if(entry.method===0)return new TextDecoder().decode(chunk);
    if(entry.method!==8||typeof DecompressionStream==='undefined')throw new Error('瀏覽器無法解壓縮這份試算表');
    const stream=new Blob([chunk]).stream().pipeThrough(new DecompressionStream('deflate-raw'));
    const reader=stream.getReader(),decoder=new TextDecoder();let output='',total=0;
    for(;;){const {done,value}=await reader.read();if(done)break;total+=value.byteLength;if(total>16e6){await reader.cancel();throw new Error('工作表解壓後太大');}output+=decoder.decode(value,{stream:true});}
    return output+decoder.decode();
  }
  function xml(text) {const doc=new DOMParser().parseFromString(text,'application/xml');if(doc.getElementsByTagName('parsererror').length)throw new Error('工作表 XML 無法讀取');return doc;}
  function columnIndex(ref) {let n=0;for(const c of (ref.match(/^[A-Z]+/)||[''])[0])n=n*26+c.charCodeAt(0)-64;return n-1;}
  async function xlsxSheets(buffer) {
    const zip=zipEntries(buffer),shared=[];
    if(zip.entries.has('xl/sharedStrings.xml')){for(const si of xml(await readZipText(zip,'xl/sharedStrings.xml')).getElementsByTagNameNS('*','si'))shared.push(si.textContent||'');}
    const names=zip.entries.has('xl/workbook.xml')?[...xml(await readZipText(zip,'xl/workbook.xml')).getElementsByTagNameNS('*','sheet')].map(s=>s.getAttribute('name')):[];
    const paths=[...zip.entries.keys()].filter(n=>/^xl\/worksheets\/sheet\d+\.xml$/.test(n)).sort((a,b)=>Number(a.match(/\d+/g).at(-1))-Number(b.match(/\d+/g).at(-1))).slice(0,5);
    const sheets=[];
    for(const [index,name] of paths.entries()){
      const doc=xml(await readZipText(zip,name)),rows=[];
      for(const row of [...doc.getElementsByTagNameNS('*','row')].slice(0,200)){
        const cells=[];
        for(const cell of row.getElementsByTagNameNS('*','c')){
          const col=columnIndex(cell.getAttribute('r')||'');if(col<0||col>=50)continue;
          const type=cell.getAttribute('t'),v=cell.getElementsByTagNameNS('*','v')[0]?.textContent||'';
          cells[col]=type==='s'?(shared[Number(v)]??''):type==='inlineStr'?(cell.getElementsByTagNameNS('*','is')[0]?.textContent||''):type==='b'?(v==='1'?'TRUE':'FALSE'):v;
        }
        rows.push(Array.from({length:Math.min(50,cells.length)},(_,i)=>cells[i]??''));
      }
      sheets.push({name:names[index]||`工作表 ${index+1}`,rows});
    }
    return sheets;
  }
  async function odsSheets(buffer) {
    const zip=zipEntries(buffer),doc=xml(await readZipText(zip,'content.xml')),sheets=[];
    for(const table of [...doc.getElementsByTagNameNS('*','table')].slice(0,5)){
      if(table.localName!=='table')continue;
      const rows=[];
      for(const row of [...table.getElementsByTagNameNS('*','table-row')].slice(0,200)){
        const cells=[];
        for(const cell of row.children){if(!['table-cell','covered-table-cell'].includes(cell.localName))continue;const count=Math.min(50-cells.length,Number(cell.getAttribute('table:number-columns-repeated')||1));for(let i=0;i<count;i++)cells.push((cell.textContent||'').slice(0,2000));if(cells.length>=50)break;}
        rows.push(cells);
      }
      sheets.push({name:table.getAttribute('table:name')||`工作表 ${sheets.length+1}`,rows});
    }
    return sheets;
  }
  async function loadText(url,maxBytes=2e6) {
    const response=await fetch(url,{credentials:'same-origin'});if(!response.ok)throw new Error('無法讀取檔案');
    if(Number(response.headers.get('Content-Length'))>maxBytes)throw new Error('檔案太大，請下載後開啟');
    const buffer=await response.arrayBuffer();if(buffer.byteLength>maxBytes)throw new Error('檔案太大，請下載後開啟');
    return new TextDecoder().decode(buffer);
  }
  async function open(item) {
    const id=++loadId,ext=item.name.split('.').pop().toLowerCase(),type=kind(item.name);
    const url=`/api/preview?path=${encodeURIComponent(item.path)}`;
    title.textContent=item.name;meta.textContent=`${type==='unknown'?'無法站內預覽':'唯讀預覽'} · ${item.size?`${(item.size/1024).toFixed(1)} KB`:'空白檔案'}`;
    download.href=`/api/download?path=${encodeURIComponent(item.path)}`;download.setAttribute('download',item.name);
    message('正在開啟預覽…');dialog.showModal();
    try {
      if(type==='image'||type==='audio'||type==='video'||type==='pdf'){
        const element=document.createElement(({image:'img',audio:'audio',video:'video',pdf:'iframe'})[type]);
        element.className=`preview-${type}`;element.src=url;
        if(type==='audio'||type==='video')element.controls=true;
        if(type==='pdf')element.title=item.name;
        if(type==='image')element.alt=item.name;
        if(id===loadId)body.replaceChildren(element);return;
      }
      if(type==='markdown'){const text=await loadText(url);if(id===loadId)renderMarkdown(text);return;}
      if(type==='code'){
        const text=await loadText(url);if(id!==loadId)return;
        const pre=document.createElement('pre');pre.className='code-view';
        if(ext==='json'){try{pre.textContent=JSON.stringify(JSON.parse(text),null,2);}catch{pre.textContent=text;}}else pre.textContent=text;
        body.replaceChildren(pre);return;
      }
      if(type==='sheet'){
        let sheets;
        if(ext==='csv'||ext==='tsv'){sheets=[{name:'工作表 1',rows:parseDelimited(await loadText(url,4e6),ext==='csv'?',':'\t')}];}
        else{const response=await fetch(url,{credentials:'same-origin'});if(!response.ok)throw new Error('無法讀取試算表');if(Number(response.headers.get('Content-Length'))>12e6)throw new Error('試算表太大，請下載後開啟');const buffer=await response.arrayBuffer();if(buffer.byteLength>12e6)throw new Error('試算表太大，請下載後開啟');sheets=ext==='xlsx'?await xlsxSheets(buffer):await odsSheets(buffer);}
        if(id===loadId)renderSheets(sheets);return;
      }
      message('目前沒有適合這種格式的站內檢視器，請使用右上角「下載」開啟。');
    } catch(error) {if(id===loadId)message(error.message||'預覽失敗，請下載後開啟。');}
  }
  dialog.addEventListener('close',()=>{loadId++;body.replaceChildren();});
  return {open};
})();
