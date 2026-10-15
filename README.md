# LiteNest

一個為樹莓派與小型家用伺服器設計的極輕量私有雲。單一執行檔、零外部執行依賴，提供像雲端硬碟一樣直覺的檔案操作與使用者權限管理。

## 目前功能

- 現代化 Liquid Glass 響應式介面，可在手機、平板與電腦使用
- 帳號登入與安全的 HttpOnly 工作階段
- 檔案上傳、下載、搜尋、建立資料夾與刪除
- 拖放與多檔上傳
- 每位使用者擁有隔離的個人空間
- 管理者可建立、編輯、刪除使用者
- 可分別授予讀取、寫入、刪除與管理者權限
- 可為使用者設定儲存配額
- 帳號刪除後，檔案保留於伺服器回收資料夾
- ARM64、ARMv7 與 AMD64 預編譯版本
- systemd 自動啟動與基本服務沙箱保護

## 一行安裝

支援使用 systemd 的 Debian、Ubuntu 與 Raspberry Pi OS：

```sh
curl -fsSL https://raw.githubusercontent.com/easonlin2013927/litenest/main/install.sh | sudo sh
```

安裝完成後，開啟 `http://樹莓派IP:8080`。

預設管理者帳號與密碼都是 `admin`。第一次登入後，請立即按右上角「變更密碼」。

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

目前版本聚焦可靠的 NAS 核心。檔案分享連結、縮圖、WebDAV、版本歷史與相簿等功能可在後續版本逐步加入。

## 授權

[MIT](LICENSE)
