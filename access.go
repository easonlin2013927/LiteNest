package main

import (
 "net/http"
 "path/filepath"
 "sort"
 "strings"
)

// Cross-account access is read-only and never includes administrator accounts
// for delegated users. Every read endpoint calls this before touching a path.
func (a *App) readableRoot(w http.ResponseWriter, r *http.Request, actor *User) (string, string, bool) {
 owner := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("owner")))
 if owner == "" { owner = actor.Username }
 if !validUsername(owner) { writeError(w,400,"使用者無效"); return "","",false }
 if owner == actor.Username {
  if !actor.Admin && !actor.Permissions.Read { writeError(w,403,"沒有讀取權限"); return "","",false }
 } else {
  if !actor.Admin && !actor.Permissions.ReadOthers { writeError(w,403,"沒有讀取其他使用者檔案的權限"); return "","",false }
  a.store.mu.RLock();target:=a.store.data.Users[owner];a.store.mu.RUnlock()
  if target==nil { writeError(w,404,"使用者不存在"); return "","",false }
  if target.Admin && !actor.Admin { writeError(w,403,"只有管理者可讀取管理者檔案"); return "","",false }
 }
 return a.userRoot(owner),owner,true
}

func withinStorage(root, full string) bool {
 rootReal,err:=filepath.EvalSymlinks(root);if err!=nil{return false}
 fileReal,err:=filepath.EvalSymlinks(full);if err!=nil{return false}
 relative,err:=filepath.Rel(rootReal,fileReal)
 return err==nil&&relative!=".."&&!strings.HasPrefix(relative,".."+string(filepath.Separator))
}

func (a *App) accessibleAccounts(w http.ResponseWriter,_ *http.Request,actor *User) {
 if !actor.Admin&&!actor.Permissions.ReadOthers&&!actor.Permissions.ResetPasswords {writeError(w,403,"沒有跨帳號權限");return}
 a.store.mu.RLock();names:=make([]string,0,len(a.store.data.Users))
 for name,target:=range a.store.data.Users {if name!=actor.Username&&(actor.Admin||!target.Admin){names=append(names,name)}}
 a.store.mu.RUnlock()
 sort.Strings(names)
 writeJSON(w,200,names)
}

func (a *App) resetOtherPassword(w http.ResponseWriter,r *http.Request,actor *User) {
 if !actor.Admin&&!actor.Permissions.ResetPasswords {writeError(w,403,"沒有重設其他使用者密碼的權限");return}
 username:=strings.ToLower(r.PathValue("username"))
 if !validUsername(username)||username==actor.Username {writeError(w,400,"請選擇其他使用者");return}
 var input struct {NewPassword string `json:"new_password"`}
 if !decodeJSON(w,r,&input){return}
 if len([]rune(input.NewPassword))<8 {writeError(w,400,"新密碼至少需要 8 個字元");return}
 hash,salt,err:=hashPassword(input.NewPassword);if err!=nil{writeError(w,500,"無法更新密碼");return}
 a.store.mu.Lock();target:=a.store.data.Users[username]
 if target==nil {a.store.mu.Unlock();writeError(w,404,"使用者不存在");return}
 if target.Admin&&!actor.Admin {a.store.mu.Unlock();writeError(w,403,"只有管理者可重設管理者密碼");return}
 oldHash,oldSalt:=target.PasswordHash,target.Salt;target.PasswordHash,target.Salt=hash,salt
 if err=a.store.saveLocked();err!=nil{target.PasswordHash,target.Salt=oldHash,oldSalt;a.store.mu.Unlock();writeError(w,500,"無法儲存密碼");return}
 a.store.mu.Unlock()
 a.sessionM.Lock();for token,session:=range a.sessions {if session.Username==username {delete(a.sessions,token)}};a.sessionM.Unlock()
 a.audit("global",actor.Username,"重設使用者密碼",username)
 a.audit("activity",actor.Username,"重設使用者密碼",username)
 w.WriteHeader(http.StatusNoContent)
}
