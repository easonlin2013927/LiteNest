package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanRelative(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"", "", true},
		{"photos/2026", "photos/2026", true},
		{"/photos//summer", "photos/summer", true},
		{"../secret", "", false},
		{"photos/../../secret", "", false},
		{`bad\..\secret`, "", false},
	}
	for _, test := range tests {
		got, err := cleanRelative(test.input)
		if test.ok && (err != nil || got != test.want) {
			t.Errorf("cleanRelative(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
		if !test.ok && err == nil {
			t.Errorf("cleanRelative(%q) unexpectedly succeeded", test.input)
		}
	}
}

func TestThemesAndTrashLifecycle(t *testing.T) {
 app,err:=newApp(t.TempDir());if err!=nil{t.Fatal(err)}
 if !validTheme("sage")||!validTheme("plum")||!validTheme("berry")||validTheme("unknown"){t.Fatal("theme allowlist is incorrect")}
 admin:=&User{Username:"admin",Admin:true}
 global:=httptest.NewRecorder()
 app.updateGlobalSettings(global,httptest.NewRequest(http.MethodPatch,"/api/settings/global",bytes.NewBufferString(`{"default_permissions":{"read":true,"write":true,"delete":true},"default_quota_bytes":0,"default_glass_level":70,"default_theme":"midnight","login_headline":"我的私人雲","login_description":"安全存放檔案"}`)),admin)
 if global.Code!=200{t.Fatalf("global theme update failed: %d %s",global.Code,global.Body.String())}
 personal:=httptest.NewRecorder()
 app.updateMySettings(personal,httptest.NewRequest(http.MethodPatch,"/api/settings/me",bytes.NewBufferString(`{"use_global_theme":false,"theme":"peach","skip_delete_confirm":true}`)),admin)
 if personal.Code!=200{t.Fatalf("personal theme update failed: %d %s",personal.Code,personal.Body.String())}
 app.store.mu.RLock();stored:=app.store.data.Users["admin"];theme:=app.store.data.Settings.DefaultTheme;personalTheme:=*stored.Theme;skip:=stored.SkipDeleteConfirm;app.store.mu.RUnlock()
 if theme!="midnight"||personalTheme!="peach"||!skip{t.Fatalf("theme settings not persisted: %s, %s, %v",theme,personalTheme,skip)}
 appearance:=httptest.NewRecorder();app.publicAppearance(appearance,httptest.NewRequest(http.MethodGet,"/api/appearance",nil))
 if !bytes.Contains(appearance.Body.Bytes(),[]byte("我的私人雲")){t.Fatalf("public login copy was not updated: %s",appearance.Body.String())}
 root:=app.userRoot("admin")
 original:=filepath.Join(root,"report.pdf")
 if err=os.WriteFile(original,[]byte("report"),0600);err!=nil{t.Fatal(err)}
 deleted:=httptest.NewRecorder()
 app.deleteFile(deleted,httptest.NewRequest(http.MethodDelete,"/api/files?path=report.pdf",nil),admin)
 if deleted.Code!=204{t.Fatalf("move to trash failed: %d %s",deleted.Code,deleted.Body.String())}
 if _,err=os.Stat(original);!errors.Is(err,os.ErrNotExist){t.Fatalf("original still exists: %v",err)}
 used,err:=app.userUsage("admin");if err!=nil||used!=6{t.Fatalf("trash should count toward quota: %d, %v",used,err)}
 listed:=httptest.NewRecorder();app.listTrash(listed,httptest.NewRequest(http.MethodGet,"/api/trash",nil),admin)
 var items []trashEntry;if err=json.Unmarshal(listed.Body.Bytes(),&items);err!=nil||len(items)!=1||items[0].OriginalPath!="report.pdf"{t.Fatalf("trash list: %+v, %v",items,err)}
 payload:=[]byte(`{"ids":["`+items[0].ID+`"]}`)
 restored:=httptest.NewRecorder();app.restoreTrash(restored,httptest.NewRequest(http.MethodPost,"/api/trash/restore",bytes.NewReader(payload)),admin)
 var result trashResult;if err=json.Unmarshal(restored.Body.Bytes(),&result);err!=nil||len(result.Done)!=1{t.Fatalf("restore failed: %+v, %v",result,err)}
 if content,err:=os.ReadFile(original);err!=nil||string(content)!="report"{t.Fatalf("restored file: %q, %v",content,err)}
 app.deleteFile(httptest.NewRecorder(),httptest.NewRequest(http.MethodDelete,"/api/files?path=report.pdf",nil),admin)
 listed=httptest.NewRecorder();app.listTrash(listed,httptest.NewRequest(http.MethodGet,"/api/trash",nil),admin)
 if err=json.Unmarshal(listed.Body.Bytes(),&items);err!=nil||len(items)!=1{t.Fatalf("second trash list: %+v, %v",items,err)}
 payload=[]byte(`{"ids":["`+items[0].ID+`"]}`)
 purged:=httptest.NewRecorder();app.purgeTrash(purged,httptest.NewRequest(http.MethodDelete,"/api/trash",bytes.NewReader(payload)),admin)
 if err=json.Unmarshal(purged.Body.Bytes(),&result);err!=nil||len(result.Done)!=1{t.Fatalf("purge failed: %+v, %v",result,err)}
 used,err=app.userUsage("admin");if err!=nil||used!=0{t.Fatalf("purge should free quota: %d, %v",used,err)}
}

func TestTrashRestoreConflictAndIsolation(t *testing.T){
 app,err:=newApp(t.TempDir());if err!=nil{t.Fatal(err)}
 if err=os.WriteFile(filepath.Join(app.userRoot("admin"),"same.txt"),[]byte("old"),0600);err!=nil{t.Fatal(err)}
 admin:=&User{Username:"admin",Admin:true}
 app.deleteFile(httptest.NewRecorder(),httptest.NewRequest(http.MethodDelete,"/api/files?path=same.txt",nil),admin)
 listed:=httptest.NewRecorder();app.listTrash(listed,httptest.NewRequest(http.MethodGet,"/api/trash",nil),admin)
 var items []trashEntry;if err=json.Unmarshal(listed.Body.Bytes(),&items);err!=nil||len(items)!=1{t.Fatalf("trash list: %+v, %v",items,err)}
 payload:=[]byte(`{"ids":["`+items[0].ID+`"]}`)
 if err=os.WriteFile(filepath.Join(app.userRoot("admin"),"same.txt"),[]byte("new"),0600);err!=nil{t.Fatal(err)}
 conflict:=httptest.NewRecorder();app.restoreTrash(conflict,httptest.NewRequest(http.MethodPost,"/api/trash/restore",bytes.NewReader(payload)),admin)
 var result trashResult;if err=json.Unmarshal(conflict.Body.Bytes(),&result);err!=nil||len(result.Failed)!=1{t.Fatalf("conflict should not overwrite: %+v, %v",result,err)}
 if content,_:=os.ReadFile(filepath.Join(app.userRoot("admin"),"same.txt"));string(content)!="new"{t.Fatal("restore overwrote existing file")}
 bob:=&User{Username:"bob",Permissions:Permissions{Read:true,Write:true,Delete:true}}
 isolated:=httptest.NewRecorder();app.restoreTrash(isolated,httptest.NewRequest(http.MethodPost,"/api/trash/restore",bytes.NewReader(payload)),bob)
 if err=json.Unmarshal(isolated.Body.Bytes(),&result);err!=nil||len(result.Failed)!=1{t.Fatalf("cross-user restore was not rejected: %+v, %v",result,err)}
}

func TestDelegatedAccessAndPasswordReset(t *testing.T){
 app,err:=newApp(t.TempDir());if err!=nil{t.Fatal(err)}
 defaults:=app.store.data.Settings.DefaultPermissions
 if !defaults.Read||!defaults.Write||!defaults.Delete||defaults.ReadOthers||defaults.ResetPasswords{t.Fatalf("unexpected default permissions: %+v",defaults)}
 hash,salt,err:=hashPassword("old-password");if err!=nil{t.Fatal(err)}
 bob:=&User{Username:"bob",PasswordHash:hash,Salt:salt,Permissions:Permissions{Read:true,Write:true,Delete:true}}
 helper:=&User{Username:"helper",Permissions:Permissions{ReadOthers:true,ResetPasswords:true}}
 app.store.mu.Lock();app.store.data.Users["bob"]=bob;app.store.data.Users["helper"]=helper;app.store.mu.Unlock()
 if err=os.MkdirAll(app.userRoot("bob"),0750);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(filepath.Join(app.userRoot("bob"),"private.txt"),[]byte("secret"),0600);err!=nil{t.Fatal(err)}
 list:=httptest.NewRecorder();app.files(list,httptest.NewRequest(http.MethodGet,"/api/files?owner=bob",nil),helper)
 if list.Code!=200||!bytes.Contains(list.Body.Bytes(),[]byte("private.txt")){t.Fatalf("delegated list failed: %d %s",list.Code,list.Body.String())}
 preview:=httptest.NewRecorder();app.previewFile(preview,httptest.NewRequest(http.MethodGet,"/api/preview?owner=bob&path=private.txt",nil),helper)
 if preview.Code!=200||preview.Body.String()!="secret"{t.Fatalf("delegated preview failed: %d %s",preview.Code,preview.Body.String())}
 download:=httptest.NewRecorder();app.download(download,httptest.NewRequest(http.MethodGet,"/api/download?owner=bob&path=private.txt",nil),helper)
 if download.Code!=200||download.Body.String()!="secret"{t.Fatalf("delegated download failed: %d %s",download.Code,download.Body.String())}
 forbidden:=httptest.NewRecorder();app.files(forbidden,httptest.NewRequest(http.MethodGet,"/api/files?owner=admin",nil),helper)
 if forbidden.Code!=403{t.Fatalf("delegated user read administrator files: %d",forbidden.Code)}
 ordinary:=httptest.NewRecorder();app.files(ordinary,httptest.NewRequest(http.MethodGet,"/api/files?owner=bob",nil),&User{Username:"alice",Permissions:Permissions{Read:true}})
 if ordinary.Code!=403{t.Fatalf("ordinary user read another account: %d",ordinary.Code)}
 app.sessionM.Lock();app.sessions["bob-session"]=session{Username:"bob",Expires:time.Now().Add(time.Hour)};app.sessionM.Unlock()
 reset:=httptest.NewRecorder();resetRequest:=httptest.NewRequest(http.MethodPost,"/api/users/bob/reset-password",bytes.NewBufferString(`{"new_password":"new-password"}`));resetRequest.SetPathValue("username","bob");app.resetOtherPassword(reset,resetRequest,helper)
 if reset.Code!=204||!verifyPassword("new-password",bob){t.Fatalf("delegated password reset failed: %d",reset.Code)}
 app.sessionM.RLock();_,exists:=app.sessions["bob-session"];app.sessionM.RUnlock();if exists{t.Fatal("password reset left old session active")}
 blocked:=httptest.NewRecorder();blockedRequest:=httptest.NewRequest(http.MethodPost,"/api/users/admin/reset-password",bytes.NewBufferString(`{"new_password":"new-password"}`));blockedRequest.SetPathValue("username","admin");app.resetOtherPassword(blocked,blockedRequest,helper)
 if blocked.Code!=403{t.Fatalf("delegated user reset administrator password: %d",blocked.Code)}
}

func TestCopyUploadQuota(t *testing.T) {
	var output bytes.Buffer
	written, err := copyUpload(&output, bytes.NewBufferString("12345"), 4)
	if !errors.Is(err, errQuota) || written != 5 {
		t.Fatalf("expected quota error after 5 bytes, got %d, %v", written, err)
	}
	output.Reset()
	written, err = copyUpload(&output, bytes.NewBufferString("12345"), -1)
	if err != nil || written != 5 || output.String() != "12345" {
		t.Fatalf("unlimited copy failed: %d, %v", written, err)
	}
}

func TestStoreCreatesDefaultAdmin(t *testing.T) {
	dir := t.TempDir()
	store, err := loadStore(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	admin := store.data.Users["admin"]
	if admin == nil || !admin.Admin || !verifyPassword("admin", admin) {
		t.Fatal("default administrator was not created correctly")
	}
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("config permissions are too broad: %v", info.Mode().Perm())
	}
}

func TestAuditActivityIsFilteredByUser(t *testing.T) {
 app := &App{dataDir:t.TempDir()}
 app.audit("activity","alice","上傳檔案","photos/a.jpg")
 app.audit("activity","bob","下載檔案","private.txt")
 own,err := app.readLogs("activity","alice",false)
 if err!=nil { t.Fatal(err) }
 if len(own)!=1 || own[0].Actor!="alice" || own[0].Detail!="photos/a.jpg" {
  t.Fatalf("activity log leaked another user's entries: %+v",own)
 }
 all,err := app.readLogs("activity","",true)
 if err!=nil || len(all)!=2 { t.Fatalf("expected both entries for admin: %+v, %v",all,err) }
}

func TestPreviewFileIsInlineAndSafe(t *testing.T) {
 app:=&App{dataDir:t.TempDir()}
 root:=app.userRoot("alice")
 if err:=os.MkdirAll(root,0750);err!=nil { t.Fatal(err) }
 if err:=os.WriteFile(filepath.Join(root,"example.js"),[]byte("alert(1)"),0600);err!=nil { t.Fatal(err) }
 user:=&User{Username:"alice",Permissions:Permissions{Read:true}}
 request:=httptest.NewRequest(http.MethodGet,"/api/preview?path=example.js",nil)
 response:=httptest.NewRecorder()
 app.previewFile(response,request,user)
 if response.Code!=200 || response.Header().Get("Content-Type")!="text/plain; charset=utf-8" || response.Body.String()!="alert(1)" { t.Fatalf("unsafe preview: %d, %v, %s",response.Code,response.Header(),response.Body.String()) }
 if got:=response.Header().Get("Content-Disposition");!strings.HasPrefix(got,"inline") { t.Fatalf("preview should be inline: %q",got) }
 denied:=httptest.NewRecorder()
 app.previewFile(denied,request,&User{Username:"alice"})
 if denied.Code!=403 { t.Fatalf("read permission was not enforced: %d",denied.Code) }
}

func TestPreviewTypesIncludeOfficeAndSafeFallback(t *testing.T) {
 cases:=map[string]string{
  "photo.png":"image/png",
  "letter.docx":"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  "slides.pptx":"application/vnd.openxmlformats-officedocument.presentationml.presentation",
  "legacy.doc":"application/octet-stream",
  "mystery.dat":"application/octet-stream",
 }
 for name,want:=range cases { if got:=previewType(name);got!=want { t.Errorf("previewType(%q) = %q, want %q",name,got,want) } }
}
