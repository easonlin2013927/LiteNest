package main

import (
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io/fs"
 "net/http"
 "os"
 "path"
 "path/filepath"
 "sort"
 "strings"
 "time"
)

type trashEntry struct {
 ID string `json:"id"`
 Name string `json:"name"`
 OriginalPath string `json:"original_path"`
 IsDir bool `json:"is_dir"`
 Size int64 `json:"size"`
 DeletedAt time.Time `json:"deleted_at"`
}

type trashFailure struct { ID string `json:"id"`; Error string `json:"error"` }
type trashResult struct { Done []string `json:"done"`; Failed []trashFailure `json:"failed"` }

func (a *App) userTrashRoot(name string) string { return filepath.Join(a.dataDir,"trash","files",name) }

func validTrashID(id string) bool {
 if len(id)!=32 { return false }
 for _,c:=range id { if !strings.ContainsRune("0123456789abcdef",c) { return false } }
 return true
}

func secureExistingParent(root,full string) error {
 rootReal,err:=filepath.EvalSymlinks(root);if err!=nil{return err}
 parentReal,err:=filepath.EvalSymlinks(filepath.Dir(full));if err!=nil{return err}
 rel,err:=filepath.Rel(rootReal,parentReal)
 if err!=nil||rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator)){return errors.New("path escapes storage root")}
 return nil
}

func itemSize(full string,info fs.FileInfo)(int64,error){if info.IsDir(){return directorySize(full)};return info.Size(),nil}

func (a *App) userUsage(name string)(int64,error){
 live,err:=directorySize(a.userRoot(name));if err!=nil{return 0,err}
 root:=a.userTrashRoot(name);entries,err:=os.ReadDir(root)
 if errors.Is(err,os.ErrNotExist){return live,nil};if err!=nil{return 0,err}
 for _,entry:=range entries {
  if !validTrashID(entry.Name()){continue}
  info,err:=entry.Info();if err!=nil{return 0,err}
  if info.Mode()&os.ModeSymlink!=0{continue}
  size,err:=itemSize(filepath.Join(root,entry.Name()),info);if err!=nil{return 0,err}
  live+=size
 }
 return live,nil
}

func (a *App) deleteFile(w http.ResponseWriter,r *http.Request,u *User){
 if !u.Admin&&!u.Permissions.Delete{writeError(w,403,"沒有刪除權限");return}
 root:=a.userRoot(u.Username);full,rel,err:=safePath(root,r.URL.Query().Get("path"))
 if err!=nil||rel==""{writeError(w,400,"路徑無效");return}
 info,err:=os.Lstat(full)
 if errors.Is(err,os.ErrNotExist){writeError(w,404,"項目不存在");return};if err!=nil{writeError(w,500,"無法讀取項目");return}
 if info.Mode()&os.ModeSymlink!=0||secureExistingParent(root,full)!=nil{writeError(w,403,"不允許移動連結或儲存空間外的項目");return}
 size,err:=itemSize(full,info);if err!=nil{writeError(w,500,"無法計算項目大小");return}
 trashRoot:=a.userTrashRoot(u.Username)
 if err=os.MkdirAll(trashRoot,0750);err!=nil{writeError(w,500,"無法建立垃圾桶");return}
 random:=make([]byte,16);if _,err=rand.Read(random);err!=nil{writeError(w,500,"無法建立垃圾桶項目");return}
 id:=hex.EncodeToString(random)
 entry:=trashEntry{ID:id,Name:path.Base(rel),OriginalPath:rel,IsDir:info.IsDir(),Size:size,DeletedAt:time.Now().UTC()}
 metaPath:=filepath.Join(trashRoot,id+".json");payload:=filepath.Join(trashRoot,id)
 meta,err:=os.OpenFile(metaPath,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{writeError(w,500,"無法建立垃圾桶紀錄");return}
 writeErr:=json.NewEncoder(meta).Encode(entry);closeErr:=meta.Close()
 if writeErr!=nil||closeErr!=nil{_ = os.Remove(metaPath);writeError(w,500,"無法儲存垃圾桶紀錄");return}
 if err=os.Rename(full,payload);err!=nil{_ = os.Remove(metaPath);writeError(w,500,"無法移到垃圾桶");return}
 a.audit("activity",u.Username,"移到垃圾桶",rel)
 w.WriteHeader(204)
}

func (a *App) readTrashEntry(name,id string)(trashEntry,error){
 if !validTrashID(id){return trashEntry{},errors.New("invalid trash id")}
 root:=a.userTrashRoot(name)
 raw,err:=os.ReadFile(filepath.Join(root,id+".json"));if err!=nil{return trashEntry{},err}
 var entry trashEntry
 if err=json.Unmarshal(raw,&entry);err!=nil{return trashEntry{},err}
 if entry.ID!=id{return trashEntry{},errors.New("trash id mismatch")}
 if _,err=os.Lstat(filepath.Join(root,id));err!=nil{return trashEntry{},err}
 return entry,nil
}

func (a *App) listTrash(w http.ResponseWriter,_ *http.Request,u *User){
 if !u.Admin&&!u.Permissions.Read&&!u.Permissions.Delete{writeError(w,403,"沒有讀取權限");return}
 entries,err:=os.ReadDir(a.userTrashRoot(u.Username))
 if errors.Is(err,os.ErrNotExist){writeJSON(w,200,[]trashEntry{});return}
 if err!=nil{writeError(w,500,"無法讀取垃圾桶");return}
 items:=make([]trashEntry,0,len(entries))
 for _,entry:=range entries {id:=strings.TrimSuffix(entry.Name(),".json");if !strings.HasSuffix(entry.Name(),".json")||!validTrashID(id){continue};item,err:=a.readTrashEntry(u.Username,id);if err==nil{items=append(items,item)}}
 sort.Slice(items,func(i,j int)bool{return items[i].DeletedAt.After(items[j].DeletedAt)})
 writeJSON(w,200,items)
}

func readTrashIDs(w http.ResponseWriter,r *http.Request)([]string,bool){
 var input struct{IDs []string `json:"ids"`}
 if !decodeJSON(w,r,&input){return nil,false}
 if len(input.IDs)==0{writeError(w,400,"請先選擇項目");return nil,false}
 seen:=make(map[string]bool,len(input.IDs));ids:=make([]string,0,len(input.IDs))
 for _,id:=range input.IDs{if !validTrashID(id){writeError(w,400,"垃圾桶項目無效");return nil,false};if !seen[id]{seen[id]=true;ids=append(ids,id)}}
 return ids,true
}

func (a *App) restoreTrash(w http.ResponseWriter,r *http.Request,u *User){
 if !u.Admin&&!u.Permissions.Write{writeError(w,403,"沒有寫入權限");return}
 ids,ok:=readTrashIDs(w,r);if !ok{return}
 result:=trashResult{Done:[]string{},Failed:[]trashFailure{}}
 items:=make([]trashEntry,0,len(ids))
 for _,id:=range ids{entry,err:=a.readTrashEntry(u.Username,id);if err!=nil{result.Failed=append(result.Failed,trashFailure{id,"項目不存在"});continue};items=append(items,entry)}
 sort.Slice(items,func(i,j int)bool{return strings.Count(items[i].OriginalPath,"/")<strings.Count(items[j].OriginalPath,"/")})
 root:=a.userRoot(u.Username);trashRoot:=a.userTrashRoot(u.Username)
 for _,entry:=range items{
  dest,rel,err:=safePath(root,entry.OriginalPath)
  if err!=nil||rel==""||secureExistingParent(root,dest)!=nil{result.Failed=append(result.Failed,trashFailure{entry.ID,"原始資料夾不存在或無效"});continue}
  if _,err=os.Lstat(dest);err==nil{result.Failed=append(result.Failed,trashFailure{entry.ID,"原位置已有同名項目"});continue}else if !errors.Is(err,os.ErrNotExist){result.Failed=append(result.Failed,trashFailure{entry.ID,"無法檢查原位置"});continue}
  if err=os.Rename(filepath.Join(trashRoot,entry.ID),dest);err!=nil{result.Failed=append(result.Failed,trashFailure{entry.ID,"還原失敗"});continue}
  _ = os.Remove(filepath.Join(trashRoot,entry.ID+".json"))
  result.Done=append(result.Done,entry.ID);a.audit("activity",u.Username,"從垃圾桶還原",rel)
 }
 writeJSON(w,200,result)
}

func (a *App) purgeTrash(w http.ResponseWriter,r *http.Request,u *User){
 if !u.Admin&&!u.Permissions.Delete{writeError(w,403,"沒有刪除權限");return}
 ids,ok:=readTrashIDs(w,r);if !ok{return}
 result:=trashResult{Done:[]string{},Failed:[]trashFailure{}}
 root:=a.userTrashRoot(u.Username)
 for _,id:=range ids{
  entry,err:=a.readTrashEntry(u.Username,id);if err!=nil{result.Failed=append(result.Failed,trashFailure{id,"項目不存在"});continue}
  if err=os.RemoveAll(filepath.Join(root,id));err!=nil{result.Failed=append(result.Failed,trashFailure{id,"永久刪除失敗"});continue}
  _ = os.Remove(filepath.Join(root,id+".json"))
  result.Done=append(result.Done,id);a.audit("activity",u.Username,"永久刪除",entry.OriginalPath)
 }
 writeJSON(w,200,result)
}
