package main

import (
 "crypto/hmac"
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 "embed"
 "encoding/base64"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "io"
 "io/fs"
 "log"
 "net/http"
 "os"
 "path"
 "path/filepath"
 "sort"
 "strings"
 "sync"
 "time"
)

const (
 defaultListen = ":8080"
 sessionCookie = "litenest_session"
 sessionTTL = 24 * time.Hour
 passwordRounds = 210000
 maxUploadBytes = int64(8 << 30)
)

//go:embed web/*
var webAssets embed.FS

type Permissions struct { Read bool `json:"read"`; Write bool `json:"write"`; Delete bool `json:"delete"` }
type User struct {
 Username string `json:"username"`
 PasswordHash string `json:"password_hash"`
 Salt string `json:"salt"`
 Admin bool `json:"admin"`
 Permissions Permissions `json:"permissions"`
 QuotaBytes int64 `json:"quota_bytes"`
 GlassLevel *int `json:"glass_level,omitempty"`
 Theme *string `json:"theme,omitempty"`
 SkipDeleteConfirm bool `json:"skip_delete_confirm,omitempty"`
 CreatedAt time.Time `json:"created_at"`
}
type GlobalSettings struct { DefaultPermissions Permissions `json:"default_permissions"`; DefaultQuotaBytes int64 `json:"default_quota_bytes"`; DefaultGlassLevel int `json:"default_glass_level"`; DefaultTheme string `json:"default_theme"` }
type configFile struct { Version int `json:"version"`; Users map[string]*User `json:"users"`; Settings GlobalSettings `json:"settings"` }
type Store struct { mu sync.RWMutex; path string; data configFile }
type session struct { Username string; Expires time.Time }
type App struct { store *Store; dataDir string; sessions map[string]session; sessionM sync.RWMutex; auditM sync.Mutex; started time.Time }
type apiError struct { Error string `json:"error"` }
type fileInfo struct { Name string `json:"name"`; Path string `json:"path"`; IsDir bool `json:"is_dir"`; Size int64 `json:"size"`; Modified time.Time `json:"modified"` }

func main() {
 listen := flag.String("listen", envOr("LITENEST_LISTEN", defaultListen), "HTTP listen address")
 dataDir := flag.String("data", envOr("LITENEST_DATA", "./data"), "persistent data directory")
 flag.Parse()
 app, err := newApp(*dataDir); if err != nil { log.Fatal(err) }
 server := &http.Server{Addr:*listen, Handler:app.routes(), ReadHeaderTimeout:10*time.Second, IdleTimeout:90*time.Second}
 log.Printf("LiteNest listening on %s (data: %s)", *listen, *dataDir)
 log.Fatal(server.ListenAndServe())
}

func envOr(key, fallback string) string { if v:=strings.TrimSpace(os.Getenv(key)); v!="" { return v }; return fallback }

func newApp(dataDir string) (*App,error) {
 abs,err:=filepath.Abs(dataDir); if err!=nil{return nil,err}
 for _,d:=range []string{abs,filepath.Join(abs,"users"),filepath.Join(abs,"trash","users")} { if err=os.MkdirAll(d,0750);err!=nil{return nil,err} }
 store,err:=loadStore(filepath.Join(abs,"config.json"));if err!=nil{return nil,err}
 for name:=range store.data.Users { if err=os.MkdirAll(filepath.Join(abs,"users",name),0750);err!=nil{return nil,err} }
 return &App{store:store,dataDir:abs,sessions:map[string]session{},started:time.Now()},nil
}

func loadStore(filename string)(*Store,error){
 s:=&Store{path:filename,data:configFile{Version:2,Users:map[string]*User{},Settings:defaultGlobalSettings()}}
 raw,err:=os.ReadFile(filename)
 if err==nil { if err=json.Unmarshal(raw,&s.data);err!=nil{return nil,fmt.Errorf("read config: %w",err)}; if s.data.Users==nil{s.data.Users=map[string]*User{}}; changed:=false; if s.data.Version<2{s.data.Settings=defaultGlobalSettings();s.data.Version=2;changed=true}; if !validTheme(s.data.Settings.DefaultTheme){s.data.Settings.DefaultTheme="sage";changed=true};if changed{if err=s.saveLocked();err!=nil{return nil,err}}; return s,nil }
 if !errors.Is(err,os.ErrNotExist){return nil,err}
 hash,salt,err:=hashPassword("admin");if err!=nil{return nil,err}
 s.data.Users["admin"]=&User{Username:"admin",PasswordHash:hash,Salt:salt,Admin:true,Permissions:Permissions{true,true,true},CreatedAt:time.Now().UTC()}
 if err=s.saveLocked();err!=nil{return nil,err}; log.Print("created default account admin/admin; change it after first login");return s,nil
}
func(s *Store)saveLocked()error{raw,err:=json.MarshalIndent(s.data,"","  ");if err!=nil{return err};tmp:=s.path+".tmp";if err=os.WriteFile(tmp,raw,0600);err!=nil{return err};return os.Rename(tmp,s.path)}

func hashPassword(password string)(string,string,error){salt:=make([]byte,16);if _,err:=rand.Read(salt);err!=nil{return "","",err};digest:=deriveKey([]byte(password),salt);return base64.RawStdEncoding.EncodeToString(digest),base64.RawStdEncoding.EncodeToString(salt),nil}
func verifyPassword(password string,user *User)bool{salt,e1:=base64.RawStdEncoding.DecodeString(user.Salt);want,e2:=base64.RawStdEncoding.DecodeString(user.PasswordHash);if e1!=nil||e2!=nil||len(want)!=sha256.Size{return false};return subtle.ConstantTimeCompare(deriveKey([]byte(password),salt),want)==1}
func deriveKey(password,salt []byte)[]byte{block:=make([]byte,len(salt)+4);copy(block,salt);block[len(block)-1]=1;mac:=hmac.New(sha256.New,password);_,_=mac.Write(block);u:=mac.Sum(nil);out:=append([]byte(nil),u...);for i:=1;i<passwordRounds;i++{mac.Reset();_,_=mac.Write(u);u=mac.Sum(u[:0]);for j:=range out{out[j]^=u[j]}};return out}

func(a *App)routes()http.Handler{
 mux:=http.NewServeMux()
 mux.HandleFunc("POST /api/login",a.login);mux.HandleFunc("POST /api/logout",a.withAuth(a.logout));mux.HandleFunc("GET /api/me",a.withAuth(a.me));mux.HandleFunc("POST /api/password",a.withAuth(a.changePassword))
 mux.HandleFunc("GET /api/files",a.withAuth(a.files));mux.HandleFunc("POST /api/folders",a.withAuth(a.createFolder));mux.HandleFunc("POST /api/upload",a.withAuth(a.upload));mux.HandleFunc("GET /api/download",a.withAuth(a.download));mux.HandleFunc("GET /api/preview",a.withAuth(a.previewFile));mux.HandleFunc("GET /api/office-preview",a.withAuth(a.officePreview));mux.HandleFunc("DELETE /api/files",a.withAuth(a.deleteFile))
 mux.HandleFunc("GET /api/trash",a.withAuth(a.listTrash));mux.HandleFunc("POST /api/trash/restore",a.withAuth(a.restoreTrash));mux.HandleFunc("DELETE /api/trash",a.withAuth(a.purgeTrash))
 mux.HandleFunc("GET /api/users",a.withAdmin(a.listUsers));mux.HandleFunc("POST /api/users",a.withAdmin(a.createUser));mux.HandleFunc("PATCH /api/users/{username}",a.withAdmin(a.updateUser));mux.HandleFunc("DELETE /api/users/{username}",a.withAdmin(a.deleteUser));mux.HandleFunc("GET /api/health",a.health)
 mux.HandleFunc("GET /api/settings/global",a.withAdmin(a.getGlobalSettings));mux.HandleFunc("PATCH /api/settings/global",a.withAdmin(a.updateGlobalSettings));mux.HandleFunc("PATCH /api/settings/me",a.withAuth(a.updateMySettings))
 mux.HandleFunc("GET /api/logs/global",a.withAdmin(a.globalLogs));mux.HandleFunc("GET /api/logs/activity",a.withAuth(a.activityLogs))
 static,_:=fs.Sub(webAssets,"web");mux.Handle("/",http.FileServer(http.FS(static)));return securityHeaders(mux)
}
func securityHeaders(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("X-Frame-Options","DENY");w.Header().Set("Referrer-Policy","same-origin");w.Header().Set("Content-Security-Policy","default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; frame-src 'self' blob:; object-src 'none'; frame-ancestors 'none'");if r.Method!="GET"&&r.Method!="HEAD"&&r.Method!="OPTIONS"{if origin:=r.Header.Get("Origin");origin!=""&&!strings.HasSuffix(origin,"://"+r.Host){writeError(w,403,"跨站請求已拒絕");return}};next.ServeHTTP(w,r)})}

func(a *App)login(w http.ResponseWriter,r *http.Request){var in struct{Username string `json:"username"`;Password string `json:"password"`};if !decodeJSON(w,r,&in){return};a.store.mu.RLock();user:=a.store.data.Users[strings.ToLower(strings.TrimSpace(in.Username))];valid:=user!=nil&&verifyPassword(in.Password,user);a.store.mu.RUnlock();if !valid{time.Sleep(250*time.Millisecond);writeError(w,401,"帳號或密碼錯誤");return};raw:=make([]byte,32);if _,err:=rand.Read(raw);err!=nil{writeError(w,500,"無法建立工作階段");return};token:=base64.RawURLEncoding.EncodeToString(raw);a.sessionM.Lock();a.sessions[token]=session{user.Username,time.Now().Add(sessionTTL)};for k,v:=range a.sessions{if time.Now().After(v.Expires){delete(a.sessions,k)}};a.sessionM.Unlock();http.SetCookie(w,&http.Cookie{Name:sessionCookie,Value:token,Path:"/",HttpOnly:true,SameSite:http.SameSiteStrictMode,MaxAge:int(sessionTTL.Seconds())});a.audit("activity",user.Username,"登入","");writeJSON(w,200,publicUser(user))}
func(a *App)logout(w http.ResponseWriter,r *http.Request,u *User){if c,err:=r.Cookie(sessionCookie);err==nil{a.sessionM.Lock();delete(a.sessions,c.Value);a.sessionM.Unlock()};http.SetCookie(w,&http.Cookie{Name:sessionCookie,Path:"/",MaxAge:-1,HttpOnly:true,SameSite:http.SameSiteStrictMode});a.audit("activity",u.Username,"登出","");w.WriteHeader(204)}
func(a *App)currentUser(r *http.Request)*User{c,err:=r.Cookie(sessionCookie);if err!=nil{return nil};a.sessionM.RLock();s,ok:=a.sessions[c.Value];a.sessionM.RUnlock();if !ok||time.Now().After(s.Expires){return nil};a.store.mu.RLock();defer a.store.mu.RUnlock();u:=a.store.data.Users[s.Username];if u==nil{return nil};copy:=*u;return &copy}
func(a *App)withAuth(next func(http.ResponseWriter,*http.Request,*User))http.HandlerFunc{return func(w http.ResponseWriter,r *http.Request){u:=a.currentUser(r);if u==nil{writeError(w,401,"請先登入");return};next(w,r,u)}}
func(a *App)withAdmin(next func(http.ResponseWriter,*http.Request,*User))http.HandlerFunc{return a.withAuth(func(w http.ResponseWriter,r *http.Request,u *User){if !u.Admin{writeError(w,403,"需要管理者權限");return};next(w,r,u)})}
func publicUser(u *User)map[string]any{return map[string]any{"username":u.Username,"admin":u.Admin,"permissions":u.Permissions,"quota_bytes":u.QuotaBytes,"created_at":u.CreatedAt,"glass_level":u.GlassLevel,"theme":u.Theme,"skip_delete_confirm":u.SkipDeleteConfirm}}
func(a *App)me(w http.ResponseWriter,_ *http.Request,u *User){used,_:=a.userUsage(u.Username);out:=publicUser(u);out["used_bytes"]=used;out["must_change_password"]=u.Username=="admin"&&verifyPassword("admin",u);a.store.mu.RLock();out["default_glass_level"]=a.store.data.Settings.DefaultGlassLevel;out["default_theme"]=a.store.data.Settings.DefaultTheme;a.store.mu.RUnlock();writeJSON(w,200,out)}

func(a *App)changePassword(w http.ResponseWriter,r *http.Request,u *User){var in struct{Current string `json:"current_password"`;New string `json:"new_password"`};if !decodeJSON(w,r,&in){return};if len(in.New)<8{writeError(w,400,"新密碼至少需要 8 個字元");return};a.store.mu.Lock();defer a.store.mu.Unlock();stored:=a.store.data.Users[u.Username];if stored==nil||!verifyPassword(in.Current,stored){writeError(w,400,"目前密碼錯誤");return};hash,salt,err:=hashPassword(in.New);if err!=nil{writeError(w,500,"無法更新密碼");return};stored.PasswordHash,stored.Salt=hash,salt;if err=a.store.saveLocked();err!=nil{writeError(w,500,"無法儲存設定");return};a.audit("activity",u.Username,"變更密碼","");w.WriteHeader(204)}
func(a *App)userRoot(name string)string{return filepath.Join(a.dataDir,"users",name)}
func cleanRelative(raw string)(string,error){raw=strings.ReplaceAll(strings.TrimSpace(raw),"\\","/");for _,part:=range strings.Split(raw,"/"){if part==".."||strings.ContainsRune(part,0){return "",errors.New("invalid path")}};clean:=strings.TrimPrefix(path.Clean("/"+raw),"/");if clean=="."{return "",nil};return clean,nil}
func safePath(root,raw string)(string,string,error){rel,err:=cleanRelative(raw);if err!=nil{return "","",err};full:=filepath.Join(root,filepath.FromSlash(rel));check,err:=filepath.Rel(root,full);if err!=nil||check==".."||strings.HasPrefix(check,".."+string(filepath.Separator)){return "","",errors.New("unsafe path")};return full,rel,nil}

func(a *App)files(w http.ResponseWriter,r *http.Request,u *User){if !u.Admin&&!u.Permissions.Read{writeError(w,403,"沒有讀取權限");return};full,rel,err:=safePath(a.userRoot(u.Username),r.URL.Query().Get("path"));if err!=nil{writeError(w,400,"路徑無效");return};entries,err:=os.ReadDir(full);if err!=nil{if errors.Is(err,os.ErrNotExist){writeError(w,404,"資料夾不存在")}else{writeError(w,500,"無法讀取資料夾")};return};items:=make([]fileInfo,0,len(entries));for _,entry:=range entries{info,err:=entry.Info();if err!=nil||info.Mode()&os.ModeSymlink!=0{continue};p:=entry.Name();if rel!=""{p=path.Join(rel,p)};items=append(items,fileInfo{entry.Name(),p,entry.IsDir(),info.Size(),info.ModTime()})};sort.Slice(items,func(i,j int)bool{if items[i].IsDir!=items[j].IsDir{return items[i].IsDir};return strings.ToLower(items[i].Name)<strings.ToLower(items[j].Name)});writeJSON(w,200,map[string]any{"path":rel,"items":items})}
func(a *App)createFolder(w http.ResponseWriter,r *http.Request,u *User){if !u.Admin&&!u.Permissions.Write{writeError(w,403,"沒有寫入權限");return};var in struct{Path string `json:"path"`};if !decodeJSON(w,r,&in){return};full,rel,err:=safePath(a.userRoot(u.Username),in.Path);if err!=nil||rel==""{writeError(w,400,"資料夾名稱無效");return};if err=os.Mkdir(full,0750);err!=nil{if errors.Is(err,os.ErrExist){writeError(w,409,"同名項目已存在")}else{writeError(w,400,"無法建立資料夾")};return};a.audit("activity",u.Username,"建立資料夾",rel);writeJSON(w,201,map[string]string{"path":rel})}

var errQuota=errors.New("quota exceeded")
func copyUpload(dst io.Writer,src io.Reader,remaining int64)(int64,error){if remaining<0{remaining=maxUploadBytes};limited:=&io.LimitedReader{R:src,N:remaining+1};n,err:=io.Copy(dst,limited);if err==nil&&n>remaining{err=errQuota};return n,err}

func(a *App)upload(w http.ResponseWriter,r *http.Request,u *User){
 if !u.Admin&&!u.Permissions.Write{writeError(w,403,"沒有寫入權限");return}
 r.Body=http.MaxBytesReader(w,r.Body,maxUploadBytes);reader,err:=r.MultipartReader();if err!=nil{writeError(w,400,"上傳格式無效");return}
 parent,_,err:=safePath(a.userRoot(u.Username),r.URL.Query().Get("path"));if err!=nil{writeError(w,400,"路徑無效");return};info,err:=os.Stat(parent);if err!=nil||!info.IsDir(){writeError(w,400,"目的資料夾不存在");return}
 used,_:=a.userUsage(u.Username);uploaded:=[]string{}
 for{part,err:=reader.NextPart();if errors.Is(err,io.EOF){break};if err!=nil{writeError(w,400,"讀取上傳內容失敗");return};if part.FileName()==""{_ = part.Close();continue};name:=filepath.Base(strings.ReplaceAll(part.FileName(),"\\","/"));if name=="."||name==""||strings.ContainsRune(name,0){_ = part.Close();writeError(w,400,"檔名無效");return};target:=filepath.Join(parent,name);oldSize:=int64(0);if old,statErr:=os.Stat(target);statErr==nil{if old.IsDir(){_ = part.Close();writeError(w,409,"同名資料夾已存在");return};oldSize=old.Size()};tmp,err:=os.CreateTemp(parent,".upload-*");if err!=nil{_ = part.Close();writeError(w,500,"無法建立檔案");return};remaining:=int64(-1);if u.QuotaBytes>0{remaining=u.QuotaBytes-used+oldSize};written,copyErr:=copyUpload(tmp,part,remaining);closeErr:=tmp.Close();_ = part.Close();if copyErr!=nil||closeErr!=nil{_ = os.Remove(tmp.Name());if errors.Is(copyErr,errQuota){writeError(w,413,"已超過儲存空間配額")}else{writeError(w,500,"寫入檔案失敗")};return};if err=os.Rename(tmp.Name(),target);err!=nil{_ = os.Remove(tmp.Name());writeError(w,500,"儲存檔案失敗");return};used+=written-oldSize;uploaded=append(uploaded,name)
 }
 if len(uploaded)==0{writeError(w,400,"沒有選擇檔案");return};for _,name:=range uploaded{a.audit("activity",u.Username,"上傳檔案",path.Join(r.URL.Query().Get("path"),name))};writeJSON(w,201,map[string]any{"uploaded":uploaded})
}

func(a *App)download(w http.ResponseWriter,r *http.Request,u *User){if !u.Admin&&!u.Permissions.Read{writeError(w,403,"沒有讀取權限");return};full,_,err:=safePath(a.userRoot(u.Username),r.URL.Query().Get("path"));if err!=nil{writeError(w,400,"路徑無效");return};info,err:=os.Stat(full);if err!=nil||info.IsDir()||info.Mode()&os.ModeSymlink!=0{writeError(w,404,"檔案不存在");return};w.Header().Set("Content-Disposition",fmt.Sprintf("attachment; filename=%q",info.Name()));a.audit("activity",u.Username,"下載檔案",r.URL.Query().Get("path"));http.ServeFile(w,r,full)}

func(a *App)listUsers(w http.ResponseWriter,_ *http.Request,_ *User){a.store.mu.RLock();users:=make([]map[string]any,0,len(a.store.data.Users));for _,u:=range a.store.data.Users{item:=publicUser(u);used,_:=a.userUsage(u.Username);item["used_bytes"]=used;users=append(users,item)};a.store.mu.RUnlock();sort.Slice(users,func(i,j int)bool{return users[i]["username"].(string)<users[j]["username"].(string)});writeJSON(w,200,users)}
func validUsername(v string)bool{if len(v)<3||len(v)>32{return false};for _,c:=range v{if !(c>='a'&&c<='z'||c>='0'&&c<='9'||c=='-'||c=='_'){return false}};return true}
func(a *App)createUser(w http.ResponseWriter,r *http.Request,actor *User){
 var in struct{Username string `json:"username"`;Password string `json:"password"`;Admin bool `json:"admin"`;Permissions *Permissions `json:"permissions"`;QuotaBytes *int64 `json:"quota_bytes"`;GlassLevel *int `json:"glass_level"`}
 if !decodeJSON(w,r,&in){return}
 in.Username=strings.ToLower(strings.TrimSpace(in.Username))
 if !validUsername(in.Username)||len(in.Password)<8{writeError(w,400,"帳號需為 3–32 個小寫英數字元；密碼至少 8 個字元");return}
 if in.QuotaBytes!=nil&&*in.QuotaBytes<0{writeError(w,400,"配額不能小於零");return}
 if in.GlassLevel!=nil&&!validGlass(*in.GlassLevel){writeError(w,400,"玻璃效果需介於 0–100");return}
 hash,salt,err:=hashPassword(in.Password);if err!=nil{writeError(w,500,"無法建立帳號");return}
 a.store.mu.Lock()
 if _,ok:=a.store.data.Users[in.Username];ok{a.store.mu.Unlock();writeError(w,409,"使用者已存在");return}
 defaults:=a.store.data.Settings
 created:=&User{Username:in.Username,PasswordHash:hash,Salt:salt,Admin:in.Admin,Permissions:defaults.DefaultPermissions,QuotaBytes:defaults.DefaultQuotaBytes,GlassLevel:in.GlassLevel,CreatedAt:time.Now().UTC()}
 if in.Permissions!=nil{created.Permissions=*in.Permissions}
 if in.QuotaBytes!=nil{created.QuotaBytes=*in.QuotaBytes}
 if created.Admin{created.Permissions=Permissions{true,true,true}}
 a.store.data.Users[in.Username]=created
 if err=a.store.saveLocked();err!=nil{delete(a.store.data.Users,in.Username);a.store.mu.Unlock();writeError(w,500,"無法儲存設定");return}
 a.store.mu.Unlock()
 if err=os.MkdirAll(a.userRoot(in.Username),0750);err!=nil{writeError(w,500,"帳號已建立，但無法建立儲存空間");return}
 a.audit("global",actor.Username,"建立使用者",in.Username)
 writeJSON(w,201,publicUser(created))
}
func(a *App)updateUser(w http.ResponseWriter,r *http.Request,actor *User){
 username:=strings.ToLower(r.PathValue("username"))
 var in struct{Password string `json:"password"`;Admin *bool `json:"admin"`;Permissions *Permissions `json:"permissions"`;QuotaBytes *int64 `json:"quota_bytes"`;GlassLevel *int `json:"glass_level"`;UseGlobalGlass bool `json:"use_global_glass"`}
 if !decodeJSON(w,r,&in){return}
 if in.QuotaBytes!=nil&&*in.QuotaBytes<0{writeError(w,400,"配額不能小於零");return}
 if in.GlassLevel!=nil&&!validGlass(*in.GlassLevel){writeError(w,400,"玻璃效果需介於 0–100");return}
 if in.Password!=""&&len(in.Password)<8{writeError(w,400,"密碼至少需要 8 個字元");return}
 if in.Admin!=nil&&username==actor.Username&&!*in.Admin{writeError(w,400,"不能移除自己的管理者權限");return}
 a.store.mu.Lock()
 target:=a.store.data.Users[username]
 if target==nil{a.store.mu.Unlock();writeError(w,404,"找不到使用者");return}
 original:=*target
 if in.Password!=""{hash,salt,err:=hashPassword(in.Password);if err!=nil{a.store.mu.Unlock();writeError(w,500,"無法更新密碼");return};target.PasswordHash,target.Salt=hash,salt}
 if in.Admin!=nil{target.Admin=*in.Admin}
 if in.Permissions!=nil{target.Permissions=*in.Permissions}
 if target.Admin{target.Permissions=Permissions{true,true,true}}
 if in.QuotaBytes!=nil{target.QuotaBytes=*in.QuotaBytes}
 if in.UseGlobalGlass{target.GlassLevel=nil}else if in.GlassLevel!=nil{level:=*in.GlassLevel;target.GlassLevel=&level}
 if err:=a.store.saveLocked();err!=nil{*target=original;a.store.mu.Unlock();writeError(w,500,"無法儲存設定");return}
 result:=publicUser(target);a.store.mu.Unlock()
 a.audit("global",actor.Username,"更新使用者",username)
 writeJSON(w,200,result)
}
func(a *App)deleteUser(w http.ResponseWriter,r *http.Request,actor *User){username:=strings.ToLower(r.PathValue("username"));if username==actor.Username{writeError(w,400,"不能刪除目前登入的帳號");return};a.store.mu.Lock();target:=a.store.data.Users[username];if target==nil{a.store.mu.Unlock();writeError(w,404,"找不到使用者");return};delete(a.store.data.Users,username);if err:=a.store.saveLocked();err!=nil{a.store.data.Users[username]=target;a.store.mu.Unlock();writeError(w,500,"無法儲存設定");return};a.store.mu.Unlock();source:=a.userRoot(username);destination:=filepath.Join(a.dataDir,"trash","users",username+"-"+time.Now().Format("20060102-150405"));_ = os.Rename(source,destination);a.audit("global",actor.Username,"刪除使用者",username);w.WriteHeader(204)}

func(a *App)health(w http.ResponseWriter,_ *http.Request){writeJSON(w,200,map[string]any{"status":"ok","service":"litenest","uptime_seconds":int64(time.Since(a.started).Seconds())})}
func directorySize(root string)(int64,error){var total int64;err:=filepath.WalkDir(root,func(_ string,e fs.DirEntry,err error)error{if err!=nil{return err};if e.Type().IsRegular(){info,err:=e.Info();if err!=nil{return err};total+=info.Size()};return nil});return total,err}
func decodeJSON(w http.ResponseWriter,r *http.Request,v any)bool{r.Body=http.MaxBytesReader(w,r.Body,1<<20);d:=json.NewDecoder(r.Body);d.DisallowUnknownFields();if err:=d.Decode(v);err!=nil{writeError(w,400,"請求內容無效");return false};return true}
func writeJSON(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,status int,message string){writeJSON(w,status,apiError{message})}
