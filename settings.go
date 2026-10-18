package main

import (
 "bufio"
 "encoding/json"
 "errors"
 "log"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "time"
)

const defaultLoginHeadline = "把重要檔案，\n安放在自己的家。"
const defaultLoginDescription = "為小型伺服器與樹莓派打造的輕量私有雲。"
func defaultGlobalSettings() GlobalSettings { return GlobalSettings{DefaultPermissions:Permissions{Read:true,Write:true,Delete:true},DefaultGlassLevel:70,DefaultTheme:"sage",LoginHeadline:defaultLoginHeadline,LoginDescription:defaultLoginDescription} }
func validGlass(level int) bool { return level>=0 && level<=100 }
func validTheme(name string) bool { switch name {case "sage","ocean","peach","midnight","forest","plum","lilac","citrus","stone","ember","graphite","berry":return true};return false }

func(a *App)publicAppearance(w http.ResponseWriter,_ *http.Request){a.store.mu.RLock();settings:=a.store.data.Settings;a.store.mu.RUnlock();writeJSON(w,200,map[string]string{"login_headline":settings.LoginHeadline,"login_description":settings.LoginDescription,"default_theme":settings.DefaultTheme})}

func(a *App)getGlobalSettings(w http.ResponseWriter,_ *http.Request,_ *User){
 a.store.mu.RLock();settings:=a.store.data.Settings;a.store.mu.RUnlock();writeJSON(w,200,settings)
}
func(a *App)updateGlobalSettings(w http.ResponseWriter,r *http.Request,actor *User){
 var settings GlobalSettings
 if !decodeJSON(w,r,&settings){return}
 if !validGlass(settings.DefaultGlassLevel)||settings.DefaultQuotaBytes<0||!validTheme(settings.DefaultTheme) {writeError(w,400,"玻璃效果、配額或介面風格無效");return}
 settings.LoginHeadline=strings.TrimSpace(settings.LoginHeadline);settings.LoginDescription=strings.TrimSpace(settings.LoginDescription)
 if len([]rune(settings.LoginHeadline))<2||len([]rune(settings.LoginHeadline))>70||len([]rune(settings.LoginDescription))<2||len([]rune(settings.LoginDescription))>140{writeError(w,400,"登入頁文字長度無效");return}
 a.store.mu.Lock();old:=a.store.data.Settings;a.store.data.Settings=settings
 if err:=a.store.saveLocked();err!=nil{a.store.data.Settings=old;a.store.mu.Unlock();writeError(w,500,"無法儲存設定");return}
 a.store.mu.Unlock();a.audit("global",actor.Username,"更新全域預設設定","")
 writeJSON(w,200,settings)
}
func(a *App)updateMySettings(w http.ResponseWriter,r *http.Request,actor *User){
 var input struct{ GlassLevel *int `json:"glass_level"`; UseGlobal *bool `json:"use_global"`; Theme *string `json:"theme"`; UseGlobalTheme *bool `json:"use_global_theme"`; SkipDeleteConfirm *bool `json:"skip_delete_confirm"` }
 if !decodeJSON(w,r,&input){return}
 if input.UseGlobal!=nil&&!*input.UseGlobal&&(input.GlassLevel==nil||!validGlass(*input.GlassLevel)){writeError(w,400,"玻璃效果需介於 0–100");return}
 if input.GlassLevel!=nil&&!validGlass(*input.GlassLevel){writeError(w,400,"玻璃效果需介於 0–100");return}
 if input.UseGlobalTheme!=nil&&!*input.UseGlobalTheme&&(input.Theme==nil||!validTheme(*input.Theme)){writeError(w,400,"介面風格無效");return}
 if input.Theme!=nil&&!validTheme(*input.Theme){writeError(w,400,"介面風格無效");return}
 if input.UseGlobal==nil&&input.GlassLevel==nil&&input.UseGlobalTheme==nil&&input.Theme==nil&&input.SkipDeleteConfirm==nil{writeError(w,400,"沒有可更新的設定");return}
 a.store.mu.Lock();stored:=a.store.data.Users[actor.Username];if stored==nil{a.store.mu.Unlock();writeError(w,404,"找不到使用者");return}
 old:=*stored
 if input.UseGlobal!=nil {if *input.UseGlobal{stored.GlassLevel=nil}else{level:=*input.GlassLevel;stored.GlassLevel=&level}}else if input.GlassLevel!=nil{level:=*input.GlassLevel;stored.GlassLevel=&level}
 if input.UseGlobalTheme!=nil{if *input.UseGlobalTheme{stored.Theme=nil}else{theme:=*input.Theme;stored.Theme=&theme}}else if input.Theme!=nil{theme:=*input.Theme;stored.Theme=&theme}
 if input.SkipDeleteConfirm!=nil{stored.SkipDeleteConfirm=*input.SkipDeleteConfirm}
 if err:=a.store.saveLocked();err!=nil{*stored=old;a.store.mu.Unlock();writeError(w,500,"無法儲存設定");return}
 result:=publicUser(stored);result["default_glass_level"]=a.store.data.Settings.DefaultGlassLevel;result["default_theme"]=a.store.data.Settings.DefaultTheme;a.store.mu.Unlock()
 a.audit("activity",actor.Username,"變更個人顯示設定","")
 writeJSON(w,200,result)
}

type auditEntry struct{
 Time time.Time `json:"time"`
 Actor string `json:"actor"`
 Action string `json:"action"`
 Detail string `json:"detail,omitempty"`
}
func(a *App)audit(kind,actor,action,detail string){
 if kind!="global"&&kind!="activity"{return}
 entry:=auditEntry{Time:time.Now().UTC(),Actor:actor,Action:action,Detail:detail}
 raw,err:=json.Marshal(entry);if err!=nil{return}
 a.auditM.Lock();defer a.auditM.Unlock()
 folder:=filepath.Join(a.dataDir,"audit")
 if err=os.MkdirAll(folder,0750);err!=nil{log.Printf("audit directory: %v",err);return}
 file,err:=os.OpenFile(filepath.Join(folder,kind+".jsonl"),os.O_APPEND|os.O_CREATE|os.O_WRONLY,0600)
 if err!=nil{log.Printf("audit open: %v",err);return}
 defer file.Close()
 raw=append(raw,byte(10))
 if _,err=file.Write(raw);err!=nil{log.Printf("audit write: %v",err)}
}
func(a *App)readLogs(kind,actor string,all bool)([]auditEntry,error){
 a.auditM.Lock();defer a.auditM.Unlock()
 file,err:=os.Open(filepath.Join(a.dataDir,"audit",kind+".jsonl"))
 if errors.Is(err,os.ErrNotExist){return []auditEntry{},nil};if err!=nil{return nil,err}
 defer file.Close()
 entries:=make([]auditEntry,0,100)
 scanner:=bufio.NewScanner(file);scanner.Buffer(make([]byte,4096),1<<20)
 for scanner.Scan(){var entry auditEntry;if json.Unmarshal(scanner.Bytes(),&entry)!=nil{continue};if !all&&entry.Actor!=actor{continue};entries=append(entries,entry);if len(entries)>200{entries=entries[1:]}}
 if err=scanner.Err();err!=nil{return nil,err}
 for i,j:=0,len(entries)-1;i<j;i,j=i+1,j-1{entries[i],entries[j]=entries[j],entries[i]}
 return entries,nil
}
func(a *App)globalLogs(w http.ResponseWriter,_ *http.Request,_ *User){entries,err:=a.readLogs("global","",true);if err!=nil{writeError(w,500,"無法讀取日誌");return};writeJSON(w,200,entries)}
func(a *App)activityLogs(w http.ResponseWriter,r *http.Request,actor *User){all:=actor.Admin&&r.URL.Query().Get("scope")=="all";entries,err:=a.readLogs("activity",actor.Username,all);if err!=nil{writeError(w,500,"無法讀取日誌");return};writeJSON(w,200,entries)}
