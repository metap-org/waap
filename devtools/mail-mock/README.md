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
| `:8025/` | UI: danh sách thư, lọc theo người nhận, xem nội dung (link bấm được) hoặc bản gốc, xoá hết. Tự làm mới mỗi 2 giây |
| `:8025/api/*` | API (bên dưới) |

## API

| Lệnh | Kết quả |
|---|---|
| `GET /api/messages` | mọi thư, mới nhất trước; `?to=<email>` lọc theo người nhận (không phân biệt hoa thường) |
| `GET /api/messages/latest?to=<email>` | thư mới nhất, `404` khi chưa có |
| `GET /api/messages/{id}` | một thư |
| `DELETE /api/messages` | xoá hết |

Mỗi thư: `id`, `receivedAt`, `from`/`to` (địa chỉ thật trong phong bì SMTP), `subject` (đã giải mã
encoded-word), `text` (phần text/plain, đã giải mã quoted-printable/base64), `html` khi có, `raw`.

Ví dụ lấy link đặt lại mật khẩu trong một script:

```sh
curl -s 'http://localhost:8025/api/messages/latest?to=bob@acme.io' | jq -r .data.text | grep -o 'http[^ ]*token=[^ ]*'
```

## Biến môi trường

| Biến | Mặc định | |
|---|---|---|
| `MAIL_MOCK_SMTP_ADDR` | `:1025` | |
| `MAIL_MOCK_HTTP_ADDR` | `:8025` | |
| `MAIL_MOCK_CAPACITY` | `500` | số thư giữ lại, bỏ thư cũ nhất trước |

## Chạy

Cả hai file compose của `m-deploy` đều chạy nó và trỏ `SMTP_HOST` của zones-service (service cấp
token, nơi gửi thư) vào đây. Chạy riêng: `go run .` trong thư mục này. Test: `go test ./...`.
