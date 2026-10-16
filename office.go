package main

import (
 "context"
 "fmt"
 "mime"
 "net/http"
 "net/url"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "time"
)

// Legacy Office formats need a converter. It is optional so the base NAS
// remains small enough for a Raspberry Pi.
func (a *App) officePreview(w http.ResponseWriter, r *http.Request, u *User) {
 if !u.Admin && !u.Permissions.Read { writeError(w,403,"沒有讀取權限"); return }
 root:=a.userRoot(u.Username)
 full,rel,err:=safePath(root,r.URL.Query().Get("path"))
 if err!=nil || rel=="" { writeError(w,400,"無效的路徑"); return }
 ext:=strings.ToLower(filepath.Ext(full))
 if ext!=".doc" && ext!=".ppt" && ext!=".xls" { writeError(w,415,"不支援的轉換格式"); return }
 info,err:=os.Lstat(full)
 if err!=nil || !info.Mode().IsRegular() { writeError(w,404,"檔案不存在"); return }
 if info.Size()>25<<20 { writeError(w,413,"文件過大，請下載檢視"); return }
 rootReal,err:=filepath.EvalSymlinks(root)
 if err!=nil { writeError(w,500,"無法讀取儲存空間"); return }
 fileReal,err:=filepath.EvalSymlinks(full)
 if err!=nil { writeError(w,404,"檔案不存在"); return }
 within,err:=filepath.Rel(rootReal,fileReal)
 if err!=nil || within==".." || strings.HasPrefix(within,".."+string(filepath.Separator)) { writeError(w,403,"檔案不在使用者空間內"); return }
 binary,err:=exec.LookPath("libreoffice")
 if err!=nil { binary,err=exec.LookPath("soffice") }
 if err!=nil { writeError(w,503,"需另外安裝 LibreOffice 才能預覽舊版 Office 文件"); return }
 temp,err:=os.MkdirTemp("","litenest-office-*")
 if err!=nil { writeError(w,500,"無法準備文件預覽"); return }
 defer os.RemoveAll(temp)
 profile:=filepath.ToSlash(filepath.Join(temp,"profile"))
 if runtime.GOOS=="windows" { profile="/"+profile }
 profileURL:=(&url.URL{Scheme:"file",Path:profile}).String()
 ctx,cancel:=context.WithTimeout(r.Context(),35*time.Second)
 defer cancel()
 command:=exec.CommandContext(ctx,binary,"-env:UserInstallation="+profileURL,"--headless","--convert-to","pdf","--outdir",temp,fileReal)
 if output,err:=command.CombinedOutput();err!=nil { _=output;writeError(w,502,"文件轉換失敗或逾時");return }
 converted:=filepath.Join(temp,strings.TrimSuffix(filepath.Base(fileReal),filepath.Ext(fileReal))+".pdf")
 pdf,err:=os.Open(converted)
 if err!=nil { writeError(w,502,"轉換後的文件不存在"); return }
 defer pdf.Close()
 pdfInfo,err:=pdf.Stat()
 if err!=nil || !pdfInfo.Mode().IsRegular() || pdfInfo.Size()>50<<20 { writeError(w,502,"轉換後的文件無效或過大"); return }
 w.Header().Set("Content-Type","application/pdf")
 w.Header().Set("Content-Disposition",mime.FormatMediaType("inline",map[string]string{"filename":fmt.Sprintf("%s.pdf",strings.TrimSuffix(info.Name(),filepath.Ext(info.Name())))}))
 w.Header().Set("Cache-Control","private, no-store")
 w.Header().Set("X-Frame-Options","SAMEORIGIN")
 w.Header().Set("Content-Security-Policy","frame-ancestors 'self'")
 a.audit("activity",u.Username,"預覽舊版 Office 文件",rel)
 http.ServeContent(w,r,filepath.Base(converted),pdfInfo.ModTime(),pdf)
}
