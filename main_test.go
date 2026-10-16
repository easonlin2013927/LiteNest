package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
