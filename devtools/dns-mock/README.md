# dns-mock

DNS giả lập cho dev và staging. Nó đóng vai DNS của thế giới bên ngoài: nhà cung cấp DNS của khách
và resolver công cộng. Nhờ vậy mọi thành phần cần DNS "thật" chạy được mà không cần domain thật.
Không thuộc plane nào. Module Go riêng, không phụ thuộc `metap`. **Không bao giờ dùng làm resolver
production.**

## Cổng

| Cổng | Dùng cho |
|---|---|
| `:5353` UDP + TCP (compose map ra host `:15353`, vì 5353 trên host thường bị mDNS chiếm) | DNS thật: `dig @127.0.0.1 -p 15353 ...`, `net.Resolver`, bất cứ client nào |
| `:8053/resolve` | DoH JSON kiểu `dns.google/resolve` (định dạng mà zones-service đang đọc) |
| `:8053/dns-query` | DoH chuẩn RFC 8484 (GET `?dns=` hoặc POST) |
| `:8053/` | UI: thêm/xoá bản ghi, tiêm lỗi, tra thử, xem log truy vấn |
| `:8053/api/*` | Admin API (bên dưới) |

Cả 3 cổng truy vấn dùng chung một đường resolve, nên hỏi kiểu nào cũng nhận cùng một câu trả lời.

## Hành vi như DNS thật

- Tên không phân biệt hoa thường; dấu chấm cuối là tuỳ chọn.
- CNAME được dò tiếp (trả cả chuỗi, như resolver thật); vòng lặp CNAME trả `SERVFAIL`; CNAME trỏ
  vào tên không tồn tại trả chuỗi kèm `NXDOMAIN`; CNAME không được nằm chung tên với bản ghi khác.
- Wildcard `*.example.test` chỉ áp cho tên không tồn tại (RFC 4592).
- Tên có tồn tại nhưng không có loại được hỏi → `NOERROR` rỗng (NODATA), không phải `NXDOMAIN`; tên
  cha của một bản ghi (ví dụ `shop.test` khi chỉ có `_waf-verify.shop.test`) cũng tính là tồn tại.
- Mọi loại bản ghi `miekg/dns` hiểu (A, AAAA, CNAME, TXT, MX, NS, CAA, SRV, ...), giá trị viết theo
  cú pháp zone file. TXT nhập chuỗi thô; tự thêm ngoặc kép và tự tách thành nhiều chuỗi khi dài hơn 255 byte.
- `DNS_MOCK_UPSTREAM=1.1.1.1:53`: tên mock không sở hữu thì chuyển lên resolver thật. Mock khi đó
  thành một lớp phủ: chỉ ghi đè vài tên (ví dụ `_waf-verify.<domain thật>`), phần còn lại vẫn là
  DNS thật. Không đặt thì mọi tên lạ đều `NXDOMAIN`, giữ cho dev chạy ổn định, lặp lại được.

## Lỗi giả lập

Theo tên (hỗ trợ `*.`), có thể chỉ cho một loại bản ghi:

| `kind` | Kết quả |
|---|---|
| `servfail` / `refused` / `nxdomain` | trả đúng rcode đó |
| `timeout` | không trả lời (UDP/TCP); DoH giữ request tới khi client tự timeout |
| `delay` + `delayMs` | trả lời bình thường sau `delayMs` |

## Admin API

```bash
M=http://localhost:8053
# Thêm (replace=true: thay mọi bản cùng tên + loại, hợp cho TXT token)
curl -XPOST $M/api/records -d '{"name":"_waf-verify.shop.test","type":"TXT","value":"waf-verify-…","replace":true}'
curl -XPOST $M/api/records -d '{"name":"www.shop.test","type":"CNAME","value":"z1.edge.waf.local."}'
curl -XPOST $M/api/faults  -d '{"name":"*.flaky.test","kind":"timeout"}'
curl $M/api/records ; curl $M/api/faults ; curl $M/api/queries
curl -XDELETE "$M/api/records?name=www.shop.test&type=CNAME"   # hoặc /api/records/{id}
curl -XDELETE "$M/api/faults?name=*.flaky.test"
curl -XPOST $M/api/reset                                          # xoá sạch
```

Đặt `DNS_MOCK_ADMIN_TOKEN` thì mọi `/api/*` cần `Authorization: Bearer <token>` (UI có ô nhập
token). **Bắt buộc đặt trên staging dùng chung.** Cổng truy vấn DNS luôn mở, như một resolver.

## Lưu trạng thái

- `DNS_MOCK_STATE_FILE`: mỗi thay đổi được ghi ra file này, và được nạp lại khi khởi động.
- `DNS_MOCK_SEED_FILE`: chỉ nạp khi chưa có state file. Cùng định dạng `{"records":[...],"faults":[...]}`.

Compose của management-plane giữ state trong volume `dns-mock`, seed bằng `seed.dev.json`
(`*.edge.waf.local → 127.0.0.1`, khớp `EDGE_CNAME_TARGET`).

## Xác minh một zone trên portal (dev)

1. Tạo zone `www.shop.test` trên portal. Domain `shop.test` được tạo kèm `verificationToken`.
2. Thêm TXT `_waf-verify.shop.test` = token đó (qua UI `http://localhost:8053` hoặc `curl` ở trên),
   rồi bấm verify domain. zones-service tra qua `DOH_RESOLVER_URL=http://dns-mock:8053/resolve`.
3. Thêm CNAME `www.shop.test → z<anything>.edge.waf.local.` rồi bấm kiểm tra DNS: zone thành `routed`.
4. Muốn thử nhánh lỗi: thêm fault `servfail`/`timeout` cho `_waf-verify.shop.test` rồi verify lại.

Log truy vấn trên UI cho biết service đã thực sự hỏi gì và nhận gì.

## Chạy riêng

```bash
cd devtools/dns-mock
DNS_MOCK_SEED_FILE=seed.dev.json go run .
go test ./...
```
