# mail-mock

SMTP giả lập cho dev và staging. Nó đóng vai máy chủ mail của thế giới bên ngoài: nhận mọi thư,
không gửi đi đâu cả, và giữ thư trong bộ nhớ để đọc lại qua UI hoặc API. Dùng để đọc thư mời và
thư đặt lại mật khẩu (roadmap 113 P2) mà không cần hộp thư thật. Không thuộc plane nào. Module Go
riêng, chỉ dùng thư viện chuẩn, không phụ thuộc `metap`. **Không bao giờ dùng làm relay production.**

Tự viết thay vì dùng Mailpit vì môi trường dev không kéo được image từ Docker Hub, và phần cần dùng
(nhận SMTP, xem thư, đọc link qua API) chỉ vài trăm dòng.

## Cổng

| Cổng | Dùng cho |
|---|---|
| `:1025` | SMTP. Không TLS; nhận AUTH PLAIN/LOGIN với bất kỳ thông tin nào, nên client cấu hình cho relay có xác thực vẫn chạy được |
| `:1143` | IMAP (IMAP4rev1, không TLS): đọc thư như một mail client. Xem "Hộp thư" bên dưới |
| `:1110` | POP3 (không TLS) |
| `:8025/` | UI: danh sách thư (đã đọc/chưa đọc), lọc theo người nhận, tiêu đề, nội dung, các liên kết trong thư có nút sao chép và mở, xem nội dung/HTML/bản gốc/headers, hướng dẫn nối mail client, phím tắt `/` `j` `k`. Tự làm mới mỗi 2 giây |
| `:8025/api/*` | API (bên dưới) |

## API

| Lệnh | Kết quả |
|---|---|
| `GET /api/messages` | mọi thư, mới nhất trước; `?to=<email>` lọc theo người nhận (không phân biệt hoa thường) |
| `GET /api/messages/latest?to=<email>` | thư mới nhất, `404` khi chưa có |
| `GET /api/messages/{id}` | một thư |
| `GET /api/info` | cổng các giao thức đang nghe (rỗng = tắt) và số thư |
| `DELETE /api/messages` | xoá hết |

Mỗi thư: `id`, `receivedAt`, `from`/`to` (địa chỉ thật trong phong bì SMTP), `subject` (đã giải mã
encoded-word), `text` (phần text/plain, đã giải mã quoted-printable/base64), `html` khi có, `raw`.

Ví dụ lấy link đặt lại mật khẩu trong một script:

```sh
curl -s 'http://localhost:8025/api/messages/latest?to=bob@acme.io' | jq -r .data.text | grep -o 'http[^ ]*token=[^ ]*'
```

## Hộp thư: IMAP và POP3

Cùng các thư đó, đọc bằng giao thức của mail client, cho nghiệp vụ cần đọc thư như một ứng dụng thật
(tích hợp nhận thư, test đầu cuối). Không cần tạo gì trước:

- **Mỗi địa chỉ nhận là một hộp thư.** Đăng nhập bằng chính địa chỉ đó (`bob@acme.io`), mật khẩu nhập
  gì cũng được. Tên đăng nhập `*` đọc mọi thư.
- **Trạng thái của client thuộc hộp thư đó.** Đã đọc, cờ, và xoá (IMAP `EXPUNGE`, POP3 `DELE` + `QUIT`)
  chỉ ẩn thư khỏi hộp thư đó. Hộp thư khác, UI và API vẫn thấy đủ.
- **IMAP**: `CAPABILITY`, `LOGIN`, `AUTHENTICATE PLAIN`, `NAMESPACE`, `LIST`/`LSUB`, `STATUS`, `SELECT`/
  `EXAMINE`, `FETCH` (`FLAGS UID RFC822 RFC822.SIZE RFC822.HEADER RFC822.TEXT INTERNALDATE ENVELOPE
  BODY[...]` gồm `HEADER.FIELDS` và `<partial>`), `SEARCH` (`ALL SEEN UNSEEN DELETED FLAGGED FROM TO
  SUBJECT TEXT BODY UID`), `STORE`, `EXPUNGE`, `CLOSE`, `UNSELECT`, `IDLE`, và dạng `UID` của
  `FETCH`/`SEARCH`/`STORE`. Thư mới đến được báo (`EXISTS`) ở `NOOP` và trong `IDLE`. Chỉ có `INBOX`;
  `APPEND`, `COPY`, tạo thư mục bị từ chối. `BODYSTRUCTURE` chỉ là bản thay thế text/plain.
- **POP3**: `USER`/`PASS`, `STAT`, `LIST`, `UIDL`, `RETR`, `TOP`, `DELE`, `RSET`, `NOOP`, `CAPA`, `QUIT`.

```sh
# POP3 bằng tay
printf 'USER bob@acme.io\r\nPASS x\r\nSTAT\r\nQUIT\r\n' | nc localhost 1110
# IMAP bằng Python
python3 -c "import imaplib; m=imaplib.IMAP4('localhost',1143); m.login('bob@acme.io','x'); m.select(); print(m.search(None,'UNSEEN'))"
```

## Biến môi trường

| Biến | Mặc định | |
|---|---|---|
| `MAIL_MOCK_SMTP_ADDR` | `:1025` | |
| `MAIL_MOCK_HTTP_ADDR` | `:8025` | |
| `MAIL_MOCK_IMAP_ADDR` | `:1143` | `off` để tắt |
| `MAIL_MOCK_POP3_ADDR` | `:1110` | `off` để tắt |
| `MAIL_MOCK_CAPACITY` | `500` | số thư giữ lại, bỏ thư cũ nhất trước |

## Chạy

Cả hai file compose của `m-deploy` đều chạy nó và trỏ `SMTP_HOST` của zones-service (service cấp
token, nơi gửi thư) vào đây. Chạy riêng: `go run .` trong thư mục này. Test: `go test ./...`.
