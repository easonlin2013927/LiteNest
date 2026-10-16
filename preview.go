package main

import (
 "errors"
 "mime"
 "net/http"
 "os"
 "path/filepath"
 "strings"
)

// Text and executable-looking files are always sent as plain text. The UI
// escapes their contents; the browser must never execute them as HTML or JS.
func previewType(name string) string {
 switch strings.ToLower(filepath.Ext(name)) {
 case ".jpg", ".jpeg": return "image/jpeg"
 case ".png": return "image/png"
 case ".gif": return "image/gif"
 case ".webp": return "image/webp"
 case ".bmp": return "image/bmp"
 case ".avif": return "image/avif"
 case ".pdf": return "application/pdf"
 case ".mp3": return "audio/mpeg"
 case ".m4a": return "audio/mp4"
 case ".wav": return "audio/wav"
 case ".ogg": return "audio/ogg"
 case ".flac": return "audio/flac"
 case ".mp4", ".m4v": return "video/mp4"
 case ".webm": return "video/webm"
 case ".mov": return "video/quicktime"
 case ".xlsx": return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
 case ".ods": return "application/vnd.oasis.opendocument.spreadsheet"
 case ".md", ".markdown", ".csv", ".tsv", ".py", ".cmd", ".bat", ".ps1", ".js", ".mjs", ".cjs", ".json", ".css", ".html", ".htm", ".xml", ".svg", ".go", ".sh", ".bash", ".sql", ".yml", ".yaml", ".toml", ".ini", ".conf", ".log", ".txt", ".ts", ".tsx", ".jsx", ".rs", ".c", ".h", ".cpp", ".java", ".rb", ".php": return "text/plain; charset=utf-8"
 default: return ""
 }
}

func (a *App) previewFile(w http.ResponseWriter, r *http.Request, u *User) {
 if !u.Admin && !u.Permissions.Read { writeError(w,403,"沒有讀取權限"); return }
 root:=a.userRoot(u.Username)
 full,rel,err:=safePath(root,r.URL.Query().Get("path"))
 if err!=nil || rel=="" { writeError(w,400,"路徑無效"); return }
 info,err:=os.Lstat(full)
 if err!=nil || !info.Mode().IsRegular() { writeError(w,404,"檔案不存在"); return }
 // A symlinked parent must not expose data outside this user's storage root.
 rootReal,err:=filepath.EvalSymlinks(root);if err!=nil { writeError(w,500,"無法讀取儲存空間"); return }
 fileReal,err:=filepath.EvalSymlinks(full);if err!=nil { writeError(w,404,"檔案不存在"); return }
 within,err:=filepath.Rel(rootReal,fileReal)
 if err!=nil || within==".." || strings.HasPrefix(within,".."+string(filepath.Separator)) { writeError(w,403,"檔案不在使用者空間內"); return }
 contentType:=previewType(info.Name())
 if contentType=="" { writeError(w,415,"此格式不支援站內預覽"); return }
 file,err:=os.Open(fileReal);if err!=nil { if errors.Is(err,os.ErrNotExist){writeError(w,404,"檔案不存在")}else{writeError(w,500,"無法開啟檔案")};return };defer file.Close()
 w.Header().Set("Content-Type",contentType)
 w.Header().Set("Content-Disposition",mime.FormatMediaType("inline",map[string]string{"filename":info.Name()}))
 w.Header().Set("Cache-Control","private, no-store")
 if contentType=="application/pdf" { w.Header().Set("X-Frame-Options","SAMEORIGIN");w.Header().Set("Content-Security-Policy","frame-ancestors 'self'") }
 a.audit("activity",u.Username,"預覽檔案",rel)
 http.ServeContent(w,r,info.Name(),info.ModTime(),file)
}
