# LiteNest

一個為樹莓派與小型家用伺服器設計的極輕量私有雲。單一執行檔、零外部執行依賴，提供像雲端硬碟一樣直覺的檔案操作與使用者權限管理。

## 目前功能

- 現代化 Liquid Glass 響應式介面，可在手機、平板與電腦使用
- 帳號登入與安全的 HttpOnly 工作階段
- 檔案上傳、下載、搜尋、建立資料夾與刪除
- 拖放與多檔上傳
- 點選檔案可站內唯讀預覽：圖片、PDF、影音、Markdown、CSV／TSV／XLSX／ODS 及常見程式碼與設定檔
- 每位使用者擁有隔離的個人空間
- 管理者可建立、編輯、刪除使用者
- 可分別授予讀取、寫入、刪除與管理者權限
- 可為使用者設定儲存配額
- 管理者全域設定：新使用者預設權限、配額與 Liquid Glass 程度
- 個人設定：每位使用者可覆寫自己的 Liquid Glass 程度並變更密碼
- 全域設定日誌與個人操作日誌
- 帳號刪除後，檔案保留於伺服器回收資料夾
- ARM64、ARMv7 與 AMD64 預編譯版本
- systemd 自動啟動與基本服務沙箱保護

## 一行安裝

支援使用 systemd 的 Debian、Ubuntu 與 Raspberry Pi OS：

```sh
curl -fsSL https://raw.githubusercontent.com/easonlin2013927/litenest/main/install.sh | sudo sh
```

安裝完成後，開啟 `http://樹莓派IP:8080`。

預設管理者帳號與密碼都是 `admin`。第一次登入後，請立即在「個人設定」中變更密碼。

> 安裝器會下載最新 GitHub Release，因此儲存庫必須至少發布過一個版本標籤。

## 手動執行

需要 Go 1.22 或更新版本：

```sh
go build -trimpath -ldflags="-s -w" -o litenest .
./litenest -listen :8080 -data ./data
```

也可使用環境變數：

| 變數 | 預設值 | 說明 |
| --- | --- | --- |
| `LITENEST_LISTEN` | `:8080` | HTTP 監聽位址 |
| `LITENEST_DATA` | `./data` | 設定與使用者檔案位置 |

## 資料結構

```text
data/
├── config.json          # 帳號、密碼雜湊、權限與配額
├── users/<username>/    # 每位使用者的個人檔案
├── audit/               # 全域設定與操作日誌
└── trash/users/         # 已刪除帳號的保留檔案
```

密碼以 PBKDF2-HMAC-SHA256（隨機 salt、210,000 次迭代）儲存。LiteNest 不會保存明文密碼。

## 反向代理與 HTTPS

區域網路測試可直接使用 8080 連接埠。若要從網際網路連線，請務必放在 Caddy、Nginx 或其他提供 HTTPS 的反向代理後方；不要直接把 8080 暴露到公網。

Caddy 範例：

```caddyfile
nas.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

## 管理服務

```sh
sudo systemctl status litenest
sudo journalctl -u litenest -f
sudo systemctl restart litenest
```

完整移除前請先備份 `/var/lib/litenest`；所有使用者檔案都在該目錄內。

## 開發

```sh
gofmt -w .
go test ./...
go vet ./...
go run . -data ./data
```

若本機尚未安裝 Go，可先啟動 UI 測試伺服器：

```sh
node scripts/preview.mjs
```

這是記憶體中的 UI 預覽；測試用檔案與帳號會在伺服器重新啟動後清空。站內檢視器全螢幕顯示圖片、音訊、影片、PDF、Markdown、程式碼、CSV/TSV、XLSX/ODS，以及 DOCX/ODT/PPTX/ODP 的文字內容。Office 文件為輕量唯讀檢視，不保證完整還原排版、圖片或動畫；上傳的腳本與 HTML 不會執行。

舊版 `.doc`、`.ppt`、`.xls` 可選擇在 NAS 主機安裝 LibreOffice，以便伺服器在預覽時轉為 PDF；這不是 LiteNest 的預設依賴，未安裝時會改顯示唯讀位元組檢視。其他未知格式也可檢查檔案開頭，並可下載原檔，但無法保證對世界上每種格式提供完整的專用編輯器。測試伺服器不執行 LibreOffice 轉換。

目前版本聚焦可靠的 NAS 核心。檔案分享連結、縮圖、WebDAV、版本歷史與相簿等功能可在後續版本逐步加入。

## 授權

[MIT](LICENSE)
