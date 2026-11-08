# 12. Glossary

Thuật ngữ, viết tắt và quy ước mã dùng trong tài liệu, chia theo nhóm. Thuật ngữ kỹ thuật giữ tiếng Anh
theo chính sách tài liệu ([2.4](../02-constraints/00-index.md#24-convention)).

- [12.1 Quy ước chung](#121-quy-ước-chung)
- [12.2 Kiến trúc tổng thể](#122-kiến-trúc-tổng-thể)
- [12.3 Edge: tầng L7 và tầng WAF](#123-edge-tầng-l7-và-tầng-waf)
- [12.4 Rule và WAF](#124-rule-và-waf)
- [12.5 Bot management và API security](#125-bot-management-và-api-security)
- [12.6 CDN và DNS](#126-cdn-và-dns)
- [12.7 Cấu hình, phân phối, rollout](#127-cấu-hình-phân-phối-rollout)
- [12.8 Analytics, cảnh báo, giám sát](#128-analytics-cảnh-báo-giám-sát)
- [12.9 Thương mại và gói giá](#129-thương-mại-và-gói-giá)
- [12.10 Bảo mật và tuân thủ](#1210-bảo-mật-và-tuân-thủ)
- [12.11 Công nghệ và sản phẩm](#1211-công-nghệ-và-sản-phẩm)
- [12.12 Viết tắt](#1212-viết-tắt)
- [12.13 Quy ước mã trong tài liệu](#1213-quy-ước-mã-trong-tài-liệu)

## 12.1 Quy ước chung

| Thuật ngữ | Nghĩa |
|---|---|
| **NCC** | Nhà cung cấp: công ty xây và vận hành sản phẩm WAAP này (bên bán). Đối lập với "khách" |
| **Khách** | Tổ chức mua WAAP để bảo vệ website, API của mình |
| **End-user** | Người dùng (hoặc bot, kẻ tấn công) truy cập website của khách qua edge |
| **Org / account** | Cấp tổ chức của khách trong Management: org chứa nhiều account, account chứa nhiều zone, phân quyền theo cấp ([5.8.12](../05-building-blocks/08-components/12-management.md)) |
| **Tenant** | Ranh giới cô lập dữ liệu và tài nguyên của một khách; mọi entity, log, quota đều mang tenant. Trong database Management, tenant của metap là **org** ([ADR-0046](../09-adr/0046-management-database-tenancy-and-storage.md)) |
| **Zone** | Một site/domain khách bảo vệ, đơn vị cấu hình chính |
| **Hostname** | Tên miền cụ thể trong một zone (ví dụ `shop.example.com`); một zone có nhiều hostname |
| **Bản đồ phân rã chức năng** | Functional Decomposition Diagram (FDD): cây chức năng ba cấp nhóm → chức năng → chức năng lá, nói hệ thống làm gì trước khi nói làm bằng gì ([requirements §1](../../requirements/01-functional-decomposition.md)) |
| **Chức năng lá** | Cấp thấp nhất của bản đồ phân rã, đủ nhỏ để viết requirement và kiểm thử; mỗi lá có đúng một plane chủ |
| **Plane chủ** | Plane giữ quyết định và dữ liệu gốc của một chức năng; plane khác chỉ gọi tới hoặc nhận kết quả |
| **Cutover** | Thời điểm khách đổi DNS để traffic thật đi qua edge ([requirements §3.1, OB8](../../requirements/03-business-analysis/01-onboarding.md#ob8-đổi-dns-và-theo-dõi)) |
| **Uỷ quyền `_acme-challenge`** | Bản ghi CNAME khách tạo để vừa chứng minh sở hữu domain, vừa cho NCC xin và gia hạn cert bằng DNS-01 trước khi đổi DNS ([requirements §3.1, OB3](../../requirements/03-business-analysis/01-onboarding.md#ob3-xác-minh-sở-hữu)) |
| **Khoá origin** | Origin chỉ nhận traffic từ edge (firewall theo dải IP của edge, header bí mật hoặc mTLS) để không ai đi vòng qua WAF ([requirements §3.1, OB11](../../requirements/03-business-analysis/01-onboarding.md#ob11-khoá-origin)) |
| **Preset bảo vệ** | Bộ cấu hình ban đầu theo loại ứng dụng do NCC soạn, áp vào bản nháp policy khi onboard ([requirements §3.1, OB6](../../requirements/03-business-analysis/01-onboarding.md#ob6-preset-bảo-vệ-ban-đầu)) |
| **SOC tự hành** | Hệ thống tự làm phần lớn vòng phát hiện, phân loại, điều tra, hành động, kiểm chứng; con người duyệt thay đổi lâu dài ([requirements §4.3](../../requirements/04-product-direction/03-autonomous-soc.md)) |
| **Tác tử AI (AI agent)** | Phần mềm AI duyệt web, thao tác thay người dùng; khác crawler AI ở chỗ có người dùng thật đứng sau ([requirements §3.9](../../requirements/03-business-analysis/09-bot-management.md)) |
| **Khử nhận dạng** | Biến đổi dữ liệu để không còn nhận ra người hay tenant cụ thể: cắt hoặc băm IP, bỏ ID trong path, bỏ body, gộp theo ngưỡng k ([requirements §4.4](../../requirements/04-product-direction/04-data-strategy.md)) |
| **Ngưỡng k (k-anonymity)** | Một ô số liệu chỉ được dùng khi gộp từ ít nhất k tenant khác nhau |
| **Vùng nghiên cứu** | Kho dữ liệu tách khỏi sản xuất, chỉ chứa dữ liệu đã khử nhận dạng (TD3, TD4), nơi data analyst làm việc |
| **Tiêu chí chấp nhận** | Điều kiện kiểm được để coi một requirement đã đáp ứng ([requirements §2](../../requirements/02-requirements.md)) |
| **Origin** | Máy chủ thật của khách đứng sau edge |
| **MVP / Mở rộng / Nâng cao** | Ba mức của mỗi nghiệp vụ: tối thiểu để ký hợp đồng doanh nghiệp đầu tiên / ngang đối thủ / tạo khác biệt ([1.2](../01-introduction/00-index.md#12-tổng-quan-yêu-cầu)) |
| **arc42** | Khung tài liệu kiến trúc 12 mục mà tài liệu này theo |
| **C4** | Ký hiệu diagram 4 mức (Context, Container, Component, Code); dùng ở mục 3 và 5 |
| **4+1 View Model** | Danh sách kiểm 5 góc nhìn kiến trúc (logical, development, process, physical, scenarios) |
| **ADR** | Architecture Decision Record: ghi một quyết định, bối cảnh, phương án và hệ quả ([9](../09-adr/00-index.md)) |
| **Đề xuất, chờ chọn / Chấp nhận** | Trạng thái của ADR: chưa chốt, cần người quyết / đã chốt |
| **Spike** | Thử nghiệm ngắn có tiêu chí đo để chọn giữa các phương án (ví dụ 3 tuần cho [ADR-0031](../09-adr/0031-edge-l7-and-waf-engine-options.md)) |
| **metap / metap-go** | Nền tảng metadata-driven của org (entity CRUD, workflow, RBAC/ABAC, audit, outbox); chỉ dùng cho Management plane ([ADR-0009](../09-adr/0009-metap-for-management-plane-only.md)) |
| **Repo demo** | Code hiện có trong repo `metap-demo-waf`; không phải điểm xuất phát bắt buộc ([11.2](../11-risks/00-index.md#112-chênh-lệch-với-code-demo-hiện-tại)) |

## 12.2 Kiến trúc tổng thể

| Thuật ngữ | Nghĩa |
|---|---|
| **WAAP** | Web Application & API Protection: WAF, DDoS L7, bot, API security gộp thành một sản phẩm |
| **Plane** | Một phần hệ thống có chu kỳ deploy và yêu cầu riêng; có 4 plane ([ADR-0001](../09-adr/0001-four-planes.md)) |
| **Management plane** | Plane nghiệp vụ: portal, API, tenant, policy, case, billing. Thư mục `management-plane/` của repo demo (trước 2026-11-08 tên là `data-plane/`) |
| **Control plane** | Plane biến ý định thành config edge chạy được và phân phối nó |
| **Edge plane** | Đường đi của traffic thật; theo nghĩa chuẩn ngành là *data plane* |
| **Analytics plane** | Plane nhận, lưu, truy vấn, phân tích log; detection, cảnh báo, export, metering |
| **Data plane** | Thuật ngữ ngành cho đường đi của traffic, tức Edge plane ở đây (không liên quan thư mục `data-plane/` cũ của repo, nay là `management-plane/`) |
| **PoP** | Point of Presence: một điểm đặt edge node |
| **SaaS / Hybrid / On-prem** | Ba mô hình triển khai: mọi plane ở NCC / Management, Control ở NCC còn edge (và có thể Analytics) ở hạ tầng khách / mọi plane ở hạ tầng khách ([7.1](../07-deployment/00-index.md#71-ba-mô-hình-triển-khai)) |
| **Core region** | Region đặt Management và Control, active/standby qua hai availability zone |
| **Outbox** | Ghi sự kiện cùng transaction với dữ liệu nghiệp vụ, phát sau; không mất sự kiện |
| **Desired state** | Trạng thái Control muốn mỗi node đạt (version snapshot); node tự kéo về và báo ACK/NACK, kiểu xDS |
| **Hợp đồng giữa plane** | Định dạng dữ liệu có version giữa hai plane, bên nhận chấp nhận N và N-1 ([5.5](../05-building-blocks/05-contracts.md)) |

## 12.3 Edge: tầng L7 và tầng WAF

| Thuật ngữ | Nghĩa |
|---|---|
| **L7** | Tầng 1 của edge, đứng trước WAF (trước đây gọi là *edge proxy*): kết nối, TLS, pre-filter, bot, challenge, API auth, rate limit, cache, proxy tới origin. Phân biệt với *tấn công L7* / *DDoS L7* (tấn công ở tầng ứng dụng): khi viết về tầng thì dùng "tầng L7" hoặc "L7" như tên thành phần |
| **WAF inspection** | Tầng 2 của edge: dịch vụ kiểm tra nội dung (CRS, rule cần body), trả verdict; tài nguyên chính là CPU |
| **Pre-filter** | Các bước lọc rẻ ở L7 trước khi gọi WAF: access list, bot, challenge, rate limit, custom rule phần pre |
| **Inspect gate** | Bước ở L7 quyết định request có cần gửi WAF không (tài nguyên tĩnh, đã bị chặn, cache hit thì không) |
| **Inspection API** | Hợp đồng gRPC stream giữa L7 và WAF inspection ([5.5](../05-building-blocks/05-contracts.md#inspection-api-l7--waf)) |
| **Verdict** | Kết luận WAF trả về cho một request: hành động, rule khớp, điểm anomaly, trường khớp (đã che) |
| **Deadline** | Thời gian tối đa L7 chờ WAF cho một request; quá hạn thì áp fail mode |
| **Fail mode / fail-open / fail-closed** | Cách xử lý khi một bước kiểm tra lỗi hoặc quá hạn: cho qua / chặn; chọn theo zone |
| **Circuit breaker** | Cơ chế ở L7 ngừng gọi WAF khi pool WAF lỗi hàng loạt, áp fail mode ngay thay vì chờ deadline |
| **Admission control / fair share** | WAF xếp hàng request theo tenant với trọng số theo gói; quá tải thì trả `Overloaded` thay vì chờ |
| **Governor** | Giới hạn tài nguyên theo tenant ở edge (CPU kiểm body, kích thước body, băng thông log) |
| **Noisy neighbor** | Tenant dùng tài nguyên chung quá mức làm chậm tenant khác |
| **Blast radius** | Phạm vi bị ảnh hưởng khi có sự cố hay cấu hình lỗi (một cell, một PoP, toàn fleet) |
| **Body processor** | Bộ phân tích body của engine WAF theo content type (form, JSON, XML, multipart) và decoder gắn thêm (GraphQL, protobuf) |
| **Profile CRS** | Tổ hợp (ruleset release, paranoia, ngưỡng) dùng chung một instance engine cho nhiều zone |
| **Profile lite** | Profile WAF của zone không trả phí: tập con rule CVE nghiêm trọng, khoảng 1/4 chi phí CPU của CRS PL1 |
| **`waf_profile_fallback`** | Cờ trên request được kiểm bằng profile chuẩn trong lúc instance riêng của zone đang được dựng |
| **Policy agent** | Module trong binary của cả hai tầng: nhận snapshot, kiểm chữ ký, dựng runtime, báo ACK/NACK ([5.8.4](../05-building-blocks/08-components/04-edge-agent-and-state.md)) |
| **State sync** | Module ở L7 đồng bộ counter rate limit giữa các node (chỉ hot key) |
| **Hot key** | Khoá rate limit vượt 10% ngưỡng trong cửa sổ hiện tại; chỉ những khoá này được đồng bộ qua Valkey |
| **Trie** | Cây tra cứu IP/CIDR dùng cho access list |
| **Routing index** | Bảng (hostname, cổng) → zone, cell, cert, digest; có trong RAM mọi node |
| **PoP config store** | Bản sao đủ bundle và cert của mỗi PoP trên NVMe; node nạp từ đây khi cần |
| **Nạp khi cần / LRU** | Node chỉ dựng runtime của zone khi có request đầu tiên, bỏ zone ít dùng nhất khi hết ngân sách RAM |
| **Làm nóng trước** | Nạp sẵn runtime của zone nóng (theo danh sách Analytics tính) sau khi node khởi động |
| **Zone nóng / nguội** | Zone đang có traffic trên node (runtime trong RAM) / chưa có |
| **Cell** | Nhóm node trong mỗi PoP phục vụ một tập zone, có VIP riêng ([ADR-0041](../09-adr/0041-cells-and-zone-placement.md)) |
| **Pool cell** | Nhóm cell dành cho một hoặc vài gói: mật độ cao (zone không trả phí), chuẩn (Pro, Business), Enterprise |
| **Cell cách ly** | Cell nhận zone đang bị tấn công để không ảnh hưởng zone khác |
| **VIP** | Virtual IP: địa chỉ IP dùng chung cho một nhóm node sau load balancer L4 |
| **Anycast** | Nhiều PoP quảng bá cùng một dải IP; mạng tự đưa người dùng tới PoP gần nhất |
| **Proxy L4** | Sản phẩm riêng (Mở rộng) cho TCP/UDP không phải HTTP: chống DDoS L3/L4, access list, không kiểm nội dung |
| **Request ID** | Định danh duy nhất của request, in trên trang chặn, khoá tra cứu và bỏ trùng |
| **status_source** | Ai sinh ra status code của một response: origin, cache, edge_policy, edge_request, client_abort, origin_unreachable, edge_platform |
| **edge_error** | Mã chi tiết khi WAAP tự sinh response thay cho origin (ví dụ `origin_connect_timeout`) |
| **edge_us** | Thời gian do WAAP gây ra cho một request, không gồm chờ origin và client |
| **Under Attack mode (UAM)** | Chế độ challenge mọi request chưa có clearance token |
| **Challenge** | Thử thách edge đưa ra để phân biệt người và bot: JS (proof-of-work), tương tác (CAPTCHA tự phát) |
| **Proof-of-work** | JS challenge buộc trình duyệt tính một bài toán nhỏ, tốn chi phí cho bot chạy hàng loạt |
| **Clearance token** | Token có chữ ký cấp sau khi qua challenge, kiểm được ở mọi node |

## 12.4 Rule và WAF

| Thuật ngữ | Nghĩa |
|---|---|
| **Managed ruleset** | Bộ rule do NCC duy trì: OWASP CRS + system ruleset |
| **CRS** | OWASP Core Rule Set: bộ rule WAF mã nguồn mở viết bằng SecLang |
| **System ruleset** | Bộ rule SecLang riêng của NCC |
| **Lớp rule khẩn** | Bộ rule nhỏ của NCC chạy trước CRS, đi fast lane không cần build lại CRS |
| **Ruleset release** | Quy trình phát hành CRS, system ruleset, dữ liệu bot như phần mềm: regression, canary, rollback ([5.8.7](../05-building-blocks/08-components/07-cert-ruleset-release.md)) |
| **SecLang** | Ngôn ngữ rule của ModSecurity; Coraza và một số engine thuần Rust cũng hỗ trợ |
| **CEL** | Common Expression Language: ngôn ngữ biểu thức dùng cho custom rule ([ADR-0014](../09-adr/0014-cel-as-rule-language.md)) |
| **Custom rule** | Rule khách viết bằng biểu thức, qua builder hoặc text |
| **AST** | Cây cú pháp của biểu thức rule; dạng chung của builder và text |
| **IR (custom rule)** | Dạng trung gian Control biên dịch từ CEL, edge thực thi |
| **Phần pre / phần inspect** | Custom rule chạy ở L7 (không cần body, đứng trước rule đầu tiên cần body) / chạy ở WAF ([ADR-0034](../09-adr/0034-pre-filter-and-rule-split-across-tiers.md)) |
| **Paranoia level (PL)** | Mức độ chặt của CRS (1–4); càng cao càng bắt nhiều, càng dễ chặn nhầm |
| **Anomaly scoring** | CRS cộng điểm từ nhiều rule rồi so với ngưỡng thay vì chặn ngay ở rule đầu tiên |
| **Exception / exclusion** | Bỏ qua một rule cho một phạm vi cụ thể (path, tham số, header) |
| **Log-only / monitor** | Chế độ đánh giá và ghi event nhưng không thực thi hành động |
| **False positive (FP)** | Request hợp lệ bị chặn nhầm |
| **Virtual patching** | Chặn khai thác một lỗ hổng (CVE) ở WAF trong khi khách chưa vá ứng dụng |
| **`go-ftw`** | Framework chạy test regression cho CRS trên một engine |
| **`edge-verify`** | Công cụ dùng chung code với edge để tự kiểm snapshot trước khi ký (self-test) |

## 12.5 Bot management và API security

| Thuật ngữ | Nghĩa |
|---|---|
| **bot.score / bot.class** | Điểm 1–99 (thấp là tự động) và lớp `verified_bot`, `likely_human`, `likely_automated`, `automated` |
| **bot.category / bot.signals** | Loại bot (search engine, AI crawler, giám sát…) / danh sách tín hiệu dẫn tới điểm |
| **Bot đã xác minh** | Bot tốt được xác nhận qua dải IP công bố, rDNS hoặc chữ ký của agent |
| **AI crawler** | Bot thu thập nội dung cho huấn luyện AI; khách chọn cho phép, challenge hay chặn |
| **JA4** | Fingerprint của TLS ClientHello, dùng nhận diện client |
| **Fingerprint HTTP/2** | Dấu vết từ SETTINGS, thứ tự pseudo-header của client HTTP/2 |
| **rDNS** | Reverse DNS: tra tên miền từ IP, dùng xác minh bot tìm kiếm |
| **Headless browser** | Trình duyệt chạy không giao diện, thường dùng cho bot tinh vi |
| **Proxy dân cư** | Mạng proxy dùng IP nhà dân để che bot |
| **ATO** | Account takeover: chiếm tài khoản, thường bằng credential stuffing |
| **Credential stuffing** | Thử hàng loạt cặp tài khoản/mật khẩu bị lộ từ nơi khác |
| **API discovery** | Học danh sách endpoint từ traffic thật |
| **Kho API** | Danh sách endpoint chính thức của khách trong Management (từ OpenAPI hoặc discovery đã duyệt) |
| **Shadow / zombie API** | Endpoint có traffic nhưng không khai báo / đã deprecated mà vẫn được gọi |
| **api.consumer** | Danh tính đã băm của client API (từ JWT `sub`, API key, cert), khoá cho rate limit và analytics |
| **JWT / JWKS** | JSON Web Token / tập khoá công khai để kiểm chữ ký JWT |
| **OpenAPI** | Chuẩn mô tả API REST; nguồn của schema kiểm ở edge |
| **BOLA** | Broken Object Level Authorization: truy cập đối tượng của người khác bằng cách đổi ID |

## 12.6 CDN và DNS

| Thuật ngữ | Nghĩa |
|---|---|
| **CDN** | Content Delivery Network: phục vụ nội dung từ cache gần người dùng; ở đây nằm trong tầng L7 |
| **Purge** | Xoá nội dung khỏi cache theo URL, prefix hoặc tag |
| **Origin shield / tiered cache** | Một lớp cache trung gian giữa PoP và origin, giảm số request về origin |
| **Hit ratio** | Tỉ lệ request (hoặc byte) phục vụ từ cache |
| **stale-if-error / stale-while-revalidate** | Phục vụ bản cache cũ khi origin lỗi / trong lúc làm mới ở nền |
| **DNS điều hướng** | DNS của NCC trả IP của PoP (VIP của cell) gần và khoẻ cho hostname edge |
| **DNS authoritative** | Khi khách chuyển quản lý DNS của domain sang NCC (Mở rộng) |
| **Bản ghi proxied** | Bản ghi DNS bật proxy: trả IP của edge thay vì IP origin |
| **CNAME** | Bản ghi DNS trỏ một tên sang tên khác; khách CNAME hostname tới hostname edge của NCC |
| **EDNS Client Subnet (ECS)** | Resolver gửi kèm dải IP của người dùng để DNS chọn PoP chính xác hơn |
| **DNSSEC** | Ký số bản ghi DNS để chống giả mạo |
| **Chế độ đứng sau CDN** | WAAP nhận traffic đã qua CDN của khách, lấy IP client từ header tin cậy |
| **Peering / IX** | Trao đổi traffic trực tiếp giữa các mạng tại điểm trao đổi Internet (IX), thường rẻ hơn transit |
| **Transit** | Băng thông mua từ nhà mạng để ra Internet, tính theo p95 |

## 12.7 Cấu hình, phân phối, rollout

| Thuật ngữ | Nghĩa |
|---|---|
| **PolicyVersion** | Bản policy bất biến của một zone, sinh ra khi được duyệt; lưu là một tài liệu jsonb có `content_hash` |
| **Đối tượng nháp** | Rule, list, exception… người dùng đang sửa, mỗi cái một dòng; publish thì gom thành PolicyVersion |
| **Unique scope** | Phạm vi của ràng buộc unique trong `metap-go`: `tenant` (mặc định, duy nhất trong tenant) hoặc `global` (duy nhất trong cả bảng) |
| **System field** | Field do nền tảng ghi (`lastUsedAt`, bộ đếm), người dùng không ghi được; cập nhật không tăng version, không audit |
| **Hostname claim** | Bản ghi trong registry toàn cục giữ quyền một hostname cho một zone, chống hai org cùng dùng |
| **Snapshot / bundle** | Bản config đã biên dịch, có chữ ký, edge nạp nguyên khối; bundle là phần của một zone hoặc một tầng |
| **Content-addressed / digest** | Định danh dữ liệu bằng hash SHA-256 của chính nó; cùng nội dung thì cùng digest |
| **Bucket manifest / top manifest** | Cây digest cấu hình: 4.096 bucket theo hash zone, top manifest ký liệt kê digest các bucket ([ADR-0040](../09-adr/0040-config-distribution-at-scale.md)) |
| **Regional relay** | Cache theo digest đặt theo region, node và PoP store kéo bundle qua đây |
| **Anti-entropy** | So sánh định kỳ digest để phát hiện và sửa chỗ thiếu khi đồng bộ |
| **Lớp áp dụng A/B/C/D** | Đổi nóng / build nền rồi đổi nóng / reload process / nâng phần mềm ([8.15](../08-cross-cutting/15-config-propagation-and-apply.md)) |
| **Node profile** | Cấu hình của node hoặc PoP (cổng, worker, RAM, cache đĩa), rollout như phần mềm |
| **Fast lane** | Đường phân phối khẩn bỏ qua các đợt rollout |
| **Canary** | Đợt rollout đầu tiên trên một nhóm node nhỏ (hoặc một cell) hoặc ở chế độ log-only |
| **Gate** | Điều kiện tự động để một đợt rollout đi tiếp |
| **Self-test** | Control chạy snapshot trên traffic mẫu bằng `edge-verify` trước khi ký |
| **Last-good** | Snapshot gần nhất đã nạp thành công, giữ để quay về |
| **ACK / NACK** | Node báo đã nạp thành công / từ chối một version |
| **Fleet registry** | Danh sách node, danh tính, sức khoẻ, version đang chạy ([5.8.6](../05-building-blocks/08-components/06-rollout-distribution-fleet.md)) |
| **N+1** | Dư một node so với năng lực cần, để mất một node vẫn đủ |
| **`PolicyVersionPublished` / `EntitlementsChanged`** | Sự kiện Management phát cho Control khi có policy mới / khi gói đổi |

## 12.8 Analytics, cảnh báo, giám sát

| Thuật ngữ | Nghĩa |
|---|---|
| **Security event / access log** | Bản ghi request bị chặn hoặc khớp rule (không lấy mẫu) / bản ghi mọi request (lấy mẫu theo gói) |
| **Mẫu request** | Request đầy đủ (đã che) lưu cho replay |
| **Enricher** | Bước trong stream processor gắn thêm thông tin (geo, ASN, zone) vào bản ghi |
| **Partition / consumer lag** | Phần chia của topic Kafka / độ trễ giữa bản ghi mới nhất và bản ghi consumer đã xử lý |
| **Materialized view (MV)** | Bảng gộp ClickHouse tự cập nhật khi có dữ liệu mới (`traffic_1m`, `rule_1h`, `usage_1h`…) |
| **SummingMergeTree** | Kiểu bảng ClickHouse tự cộng các dòng cùng khoá; dùng cho số đo usage |
| **At-least-once / idempotent** | Bản ghi có thể đến nhiều lần / xử lý lại không làm sai kết quả (bỏ trùng theo khoá) |
| **SIEM export** | Đẩy log liên tục sang hệ thống SIEM của khách (JSON, CEF, LEEF) |
| **Replay** | Chạy policy nháp trên traffic lịch sử để mô phỏng tác động |
| **Baseline** | Mức bình thường của một chỉ số, tính từ lịch sử để phát hiện bất thường |
| **EWMA / median, MAD** | Trung bình trượt có trọng số mũ / trung vị và độ lệch tuyệt đối trung vị; cách tính baseline và độ lệch ([5.8.10](../05-building-blocks/08-components/10-alerting-rules-and-algorithms.md)) |
| **Alert template** | Cảnh báo có công thức tường minh, khách chỉnh ngưỡng (W, D, O, BOT, API ở 12.13) |
| **Deliveries** | Hàng đợi gửi cảnh báo trên PostgreSQL, worker lấy việc bằng `SKIP LOCKED` |
| **SLI / SLO** | Chỉ số mức dịch vụ / mục tiêu cho chỉ số đó ([8.12](../08-cross-cutting/12-platform-monitoring.md)) |
| **Error budget / burn rate** | Phần lỗi cho phép trong kỳ theo SLO / tốc độ tiêu phần đó; cảnh báo theo burn rate |
| **Probe tổng hợp** | Request giả chạy liên tục trên zone canary để kiểm từng hành vi từ đầu tới cuối (PR1–PR8) |
| **p95 / p99** | Phân vị 95 / 99 của một phân bố (ví dụ độ trễ) |
| **P1 / P2 / P3** | Mức cảnh báo cho NOC: gọi on-call ngay / trong giờ (ngoài giờ trong 30 phút) / ticket |
| **SEV1 / SEV2 / SEV3** | Mức sự cố khi đã xảy ra (nhiều PoP hoặc mất dữ liệu / một PoP hoặc một plane / một thành phần), quyết định quy trình xử lý và báo khách |
| **Page / ticket** | Cảnh báo gọi người trực ngay / tạo việc xử lý trong giờ |
| **Runbook** | Các bước xử lý cụ thể cho một tình huống vận hành ([8.13](../08-cross-cutting/13-operational-behaviors.md)) |
| **RUM** | Real user monitoring: đo trải nghiệm thật trên trình duyệt người dùng |
| **Realtime gateway** | Thành phần đẩy số liệu từng giây, event mẫu, cảnh báo tới portal qua WebSocket ([5.8.13](../05-building-blocks/08-components/13-realtime-gateway.md)) |
| **Live tap** | Phần của enricher chỉ ghi số liệu và event (có trần) của zone đang có người xem vào topic `live` |
| **Zone aggregator** | Process gom số liệu của một zone đang được xem, giữ ring buffer 5 phút, dừng khi hết người xem |
| **Token realtime** | JWT hạn 5 phút Management cấp cho phiên portal, ghi tenant và zone được xem |

## 12.9 Thương mại và gói giá

| Thuật ngữ | Nghĩa |
|---|---|
| **Gói** | Dùng thử, Free có trần, Pro, Business, Enterprise ([8.18](../08-cross-cutting/18-segments-and-plans.md)) |
| **Dùng thử** | 14 ngày tính năng Business, 1 zone, trần cứng 50 GB / 5 triệu request |
| **Free có trần** | Gói 0 đ có trần cứng; chưa mở khi ra mắt với hồ sơ công ty phần mềm |
| **Entitlements** | Giới hạn và tính năng theo gói, dạng cấu hình ([ADR-0007](../09-adr/0007-log-by-subscription-entitlements.md)) |
| **Phí nền** | Phần giá cố định theo zone hoặc tenant, trả cho giá trị bảo mật |
| **Hạn mức** | Lượng băng thông sạch và request đi kèm phí nền mỗi tháng |
| **Phí vượt** | Tiền tính cho phần dùng vượt hạn mức theo đơn giá công bố |
| **Trần cứng** | Giới hạn edge thực thi khi chạm (gói không trả phí); gói trả phí không có trần, chỉ tính phí vượt |
| **Traffic sạch** | Traffic tới client không bị edge chặn hay challenge; chỉ phần này được tính phí |
| **Chế độ chuyển tiếp tối giản** | Trạng thái zone hết dùng thử hoặc quá hạn thanh toán: không WAF, không cache, trần 1 GB/ngày trong 30 ngày |
| **Zone phụ** | Zone thêm trong cùng account (staging, domain phụ), giá thấp hơn zone chính |
| **Metering** | Đo usage theo zone để tính tiền |
| **UsageCounter** | Bản ghi đếm usage mỗi phút mỗi zone từ node, không lấy mẫu, có số thứ tự ([ADR-0043](../09-adr/0043-usage-based-pricing-and-metering.md)) |
| **UsageReport / QuotaStateChanged** | Sự kiện Analytics gửi Management: usage theo giờ / zone chạm 80%, 100% hạn mức hoặc trần cứng |
| **Hồ sơ hạ tầng** | Bộ đơn giá đầu vào: công ty phần mềm (thuê băng thông, colo, IP, chống DDoS) hoặc nhà mạng, datacenter có sẵn |
| **Chi phí biên** | Chi phí thêm cho mỗi đơn vị dùng (GB, triệu request, bản ghi log) |
| **Chi phí cố định** | Chi phí không phụ thuộc số khách: node tối thiểu, cam kết băng thông, đội ngũ |
| **Chi phí phục vụ (cost to serve)** | Chi phí phân bổ cho một zone hoặc một gói mỗi tháng |
| **Điểm hoà vốn** | Doanh thu cần để bù chi phí cố định cộng biến đổi ở một giai đoạn |
| **Thứ tự hy sinh** | Thứ tự cố định hạ cấp tài nguyên theo gói khi PoP quá tải, từ zone không trả phí lên |
| **White-label** | Bán sản phẩm dưới thương hiệu của đối tác (ví dụ nhà mạng) |
| **SLA** | Cam kết mức dịch vụ trong hợp đồng, kèm bồi hoàn khi vi phạm |
| **TAM** | Technical Account Manager: người kỹ thuật phụ trách riêng một khách Enterprise |
| **Trust and Safety** | Bộ phận xử lý lạm dụng (phishing, malware) trên nền tảng |

## 12.10 Bảo mật và tuân thủ

| Thuật ngữ | Nghĩa |
|---|---|
| **mTLS** | TLS hai chiều: cả hai bên trình cert; dùng giữa node và Control, L7 và WAF, edge và origin |
| **PKI** | Hạ tầng cấp và thu hồi cert (CA cho node) |
| **Node identity service** | Dịch vụ ở Control cấp, gia hạn cert node sau khi kiểm fleet registry; node không gọi OpenBao trực tiếp ([8.19](../08-cross-cutting/19-node-identity-and-mtls.md)) |
| **Bootstrap token** | Token một lần dùng để node enroll lần đầu |
| **Re-enroll credential** | Khoá riêng của node chỉ dùng để xin lại cert khi cert đã hết hạn do offline (≤ 30 ngày) |
| **Trust bundle** | Danh sách CA được tin, phát xuống node và dịch vụ; chứa CA hiện tại và CA kế tiếp khi xoay |
| **Origin CA** | CA của NCC cấp cert client cho edge khi xác thực với origin; khách cấu hình tin CA này |
| **CRL / OCSP** | Hai cách thu hồi cert truyền thống (danh sách thu hồi / hỏi trạng thái); không dùng cho cert node, thay bằng thời hạn ngắn |
| **TPM** | Chip bảo mật trên máy chủ, giữ private key không đọc ra được |
| **SPIFFE / SPIRE** | Chuẩn danh tính workload ngắn hạn / bản cài đặt của nó; SAN của cert node theo dạng SPIFFE ID |
| **Keyless** | Private key cert ở lại hạ tầng khách; edge gọi sang để ký khi bắt tay TLS |
| **BYOIP** | Bring your own IP: khách đưa dải IP của mình để edge quảng bá |
| **HMAC** | Mã xác thực thông điệp bằng khoá bí mật; dùng ký webhook |
| **SSO / SAML / OIDC** | Đăng nhập một lần / hai chuẩn giao thức cho SSO |
| **SCIM** | Chuẩn đồng bộ user, nhóm từ identity provider |
| **RBAC / ABAC** | Phân quyền theo vai trò / theo thuộc tính (ví dụ giới hạn theo zone) |
| **4-eyes** | Thay đổi phải có người thứ hai duyệt |
| **PII** | Dữ liệu định danh cá nhân |
| **Nghị định 13/2023/NĐ-CP / GDPR** | Quy định bảo vệ dữ liệu cá nhân của Việt Nam / EU |
| **PCI DSS / SOC 2** | Chuẩn bảo mật cho dữ liệu thẻ thanh toán / khung báo cáo kiểm soát bảo mật |
| **Data residency** | Dữ liệu của tenant phải nằm trong một region cụ thể |
| **CVE** | Mã định danh công khai của một lỗ hổng |
| **XSS / SQLi / XXE / SSRF** | Các loại tấn công web: chèn script / chèn SQL / thực thể XML ngoài / buộc server gọi tới địa chỉ kẻ tấn công chọn |

## 12.11 Công nghệ và sản phẩm

Lựa chọn và lý do ở [5.7](../05-building-blocks/07-technology-map.md) và các ADR liên quan.

| Tên | Là gì, dùng ở đâu |
|---|---|
| **Pingora** | Framework proxy viết bằng Rust (Cloudflare); phương án nền cho tầng L7 |
| **Envoy / `ext_proc`** | Proxy C++ / API của Envoy gửi request sang dịch vụ ngoài để xử lý; phương án nền L7 |
| **HAProxy / SPOE** | Load balancer, proxy C / giao thức gửi request sang agent ngoài; phương án nền L7 |
| **NGINX / OpenResty** | Web server, proxy C / bản NGINX kèm Lua; phương án nền L7, mạnh về cache |
| **Coraza** | Engine WAF viết bằng Go, chạy SecLang và CRS; engine mặc định đề xuất |
| **libmodsecurity v3** | Engine WAF C++ (ModSecurity), engine tham chiếu của CRS |
| **CoreDNS** | DNS server Go, mở rộng bằng plugin; dùng cho DNS điều hướng |
| **Valkey** | Kho key-value tương thích Redis, license BSD; đồng bộ counter hot key |
| **OpenBao** | Quản lý bí mật, PKI, ký (nhánh mã nguồn mở của Vault) |
| **cert-manager** | Controller Kubernetes tự cấp và gia hạn cert cho dịch vụ trung tâm |
| **Vector** | Log shipper trên node và ingest gateway |
| **Elixir / BEAM / OTP** | Ngôn ngữ chạy trên máy ảo Erlang (BEAM), mỗi thực thể là một process nhẹ, OTP supervisor tự khởi động lại khi lỗi; dùng cho realtime gateway và notifier ([ADR-0044](../09-adr/0044-realtime-gateway-and-notifier-runtime.md)) |
| **Phoenix Channels / PubSub / Presence** | Thư viện Elixir: kênh WebSocket theo topic / phát message trong cụm / theo dõi ai đang kết nối |
| **Broadway / Oban** | Thư viện Elixir: consumer Kafka có backpressure / job queue trên PostgreSQL (dùng bản mã nguồn mở) |
| **Centrifugo** | Server realtime viết bằng Go, phương án dự phòng cho realtime gateway |
| **Apache Kafka (KRaft) / Strimzi** | Bus telemetry / operator chạy Kafka trên Kubernetes |
| **ClickHouse** | Cơ sở dữ liệu cột cho hot search và số liệu gộp |
| **Parquet** | Định dạng file cột, dùng cho archive và mẫu request |
| **PostgreSQL / CloudNativePG** | Cơ sở dữ liệu cho Management, Control / operator chạy PostgreSQL trên Kubernetes |
| **RabbitMQ** | Message broker cho sự kiện điều khiển giữa plane |
| **SeaweedFS** | Object storage chuẩn S3 tự host |
| **LMDB** | Kho key-value nhúng, đề xuất cho PoP config store |
| **MMDB** | Định dạng file dữ liệu geo/ASN |
| **OpenTelemetry, VictoriaMetrics, VictoriaLogs, Jaeger, Alertmanager, Grafana** | Bộ quan sát nền tảng nội bộ NCC |
| **`lego`, `cel-go`, `franz-go`, `quic-go`** | Thư viện Go: ACME / CEL / Kafka client / QUIC, HTTP/3 |

## 12.12 Viết tắt

| Viết tắt | Nghĩa |
|---|---|
| ACME | Giao thức tự động cấp cert (Let's Encrypt và CA khác) |
| ASN | Số hiệu mạng tự trị; dùng nhận diện nhà mạng, datacenter |
| CI | Continuous integration |
| CRUD | Tạo, đọc, sửa, xoá |
| DR | Disaster recovery |
| DSL | Ngôn ngữ chuyên dụng (domain-specific language) |
| FFI | Gọi hàm giữa hai ngôn ngữ (foreign function interface) |
| GC | Garbage collection: thu gom bộ nhớ tự động, có thể gây khựng |
| HA | High availability |
| IaC | Infrastructure as code (Terraform, GitOps) |
| ITSM, SOAR | Hệ thống quản lý sự cố, ticket / nền tảng tự động hoá phản ứng bảo mật |
| KV | Key-value |
| NIC | Card mạng |
| NOC / SOC | Trung tâm vận hành mạng / trung tâm vận hành bảo mật |
| NVMe | Ổ SSD giao tiếp PCIe, dùng cho cache và PoP config store |
| OOM | Hết bộ nhớ (out of memory) |
| QUIC | Giao thức truyền tải trên UDP mà HTTP/3 dùng |
| RE2 | Engine regex thời gian tuyến tính, chống ReDoS |
| RPO / RTO | Lượng dữ liệu tối đa có thể mất / thời gian tối đa để khôi phục |
| RTT | Thời gian khứ hồi mạng |
| SAN | Tên phụ trong cert, cho một cert phủ nhiều hostname |
| SNI | Tên miền client gửi trong TLS ClientHello, dùng chọn cert |
| TTFB | Thời gian tới byte đầu tiên |
| TTL | Thời gian sống của bản ghi (DNS, cache) |
| UA | User-Agent |
| VRL | Vector Remap Language: ngôn ngữ biến đổi bản ghi của Vector |
| XDP | Xử lý gói tin sớm trong kernel Linux, cho lọc L3/L4 tốc độ cao |

## 12.13 Quy ước mã trong tài liệu

Mỗi tiền tố chỉ có một nghĩa trong toàn bộ tài liệu.

| Mã | Nghĩa | Ở đâu |
|---|---|---|
| **M1…M10 / V1…V6 / AI1…AI8** | Năng lực của WAAP hiện đại / vị trí đặt AI/ML / nguyên tắc AI | [requirements §4.1, §4.2](../../requirements/04-product-direction/00-index.md) |
| **TH0…TH4 / HR1…HR8** | Mức tự hành của SOC / hàng rào an toàn cho hành động tự động | [requirements §4.3](../../requirements/04-product-direction/03-autonomous-soc.md) |
| **U1…U7 / TD0…TD5 / LC1…LC7** | Mục đích dùng dữ liệu / tầng dữ liệu / lớp lọc dữ liệu nhạy cảm | [requirements §4.4](../../requirements/04-product-direction/04-data-strategy.md) |
| **H1…H8** | Hồ sơ khách điển hình khi onboarding | [requirements §3.1](../../requirements/03-business-analysis/01-onboarding.md#6-hồ-sơ-khách-điển-hình-đề-xuất-chờ-duyệt) |
| **KB1-A1…KB13-J3** | Kịch bản của từng nghiệp vụ: `KB<số trang>-<trục><số>`, ví dụ KB1-C4 là trục C (cách trỏ) của onboarding (§3.1) | [requirements §3](../../requirements/03-business-analysis/00-index.md) |
| **OB1…OB12 / OG1…OG4 / CB1…CB7** | Bước / cổng / việc chuẩn bị trong bản nháp onboarding chưa duyệt (Phụ lục A) | [requirements §3.1](../../requirements/03-business-analysis/01-onboarding.md) |
| **Q1–Q6** | Mục tiêu chất lượng | [1.3](../01-introduction/00-index.md#13-mục-tiêu-chất-lượng) |
| **F1…F12, Fx.y, Fx.y.z** | Nhóm chức năng, chức năng, chức năng lá | [requirements §1](../../requirements/01-functional-decomposition.md) |
| **PM1…PM7 / PC1…PC8 / PE1…PE4 / PA1…PA8** | Thành phần chức năng của Management / Control / Edge / Analytics trong phân rã theo plane | [requirements §1](../../requirements/01-functional-decomposition.md#phân-rã-theo-plane) |
| **LK-1…LK-28** | Liên kết giữa các plane (chức năng phát → chức năng nhận, hợp đồng, kiểu, hành vi khi hỏng) | [requirements §1](../../requirements/01-functional-decomposition.md#liên-kết-giữa-các-plane) |
| **FR-x-nn / BR-n / DT-n / NFR-n / IR-n / OQ-n** | Yêu cầu chức năng / quy tắc nghiệp vụ / yêu cầu dữ liệu / phi chức năng / giao tiếp ngoài / câu hỏi mở của requirements (OQ-1…OQ-15 ở §2, OQ-16…OQ-20 ở Phụ lục A của §3.1, OQ-21…OQ-126 và OQ-143…OQ-165 ở các trang của §3, OQ-127…OQ-142 ở §4) | [requirements §2](../../requirements/02-requirements.md) |
| **QS-1…QS-41** | Kịch bản chất lượng đo được | [10.3](../10-quality/00-index.md#103-kịch-bản) |
| **R1…R29** | Rủi ro | [11.1](../11-risks/00-index.md) |
| **A1…A17 / B1…B10** | Tình huống vận hành tự xảy ra / thao tác của người vận hành, mỗi cái có runbook | [8.13](../08-cross-cutting/13-operational-behaviors.md) |
| **S1…S12** | SLI của nền tảng | [8.12](../08-cross-cutting/12-platform-monitoring.md) |
| **PR1…PR8** | Probe tổng hợp | [8.12 §4](../08-cross-cutting/12-platform-monitoring.md#4-probe-tổng-hợp-theo-hành-vi) |
| **P1 / P2 / P3** | Mức cảnh báo cho NOC | [8.13](../08-cross-cutting/13-operational-behaviors.md) |
| **W1…W6, D1…D6, O1…O3, BOT1…BOT3, API1…API3** | Alert template cho khách: WAF, DDoS, origin, bot, API | [5.8.10](../05-building-blocks/08-components/10-alerting-rules-and-algorithms.md) |
| **BS1…BS7** | Lớp tín hiệu bot | [8.16](../08-cross-cutting/16-bot-management.md) |
| **G1…G12** | Giả định ước lượng tải | [7.5](../07-deployment/05-capacity-and-sharding.md#1-giả-định) |
| **L1…L4** (sharding) | Bốn lớp sharding: routing index, PoP store, runtime node, cell | [7.5 §4](../07-deployment/05-capacity-and-sharding.md#4-sharding-bốn-lớp) |
| **T1…T13** | Tiêu chí chọn nền L7 và engine WAF | [ADR-0031](../09-adr/0031-edge-l7-and-waf-engine-options.md) |
| **N1…N5 / E1…E3 / I1…I3 / K1…K5** | Phương án nền L7 / engine WAF / cách L7 gọi WAF / tổ hợp cho spike | [ADR-0031](../09-adr/0031-edge-l7-and-waf-engine-options.md) |
| **MG1…MG5 / DB1…DB4** | Thiếu sót của `metap-go` tìm thấy khi thiết kế (đã sửa trừ MG3) / quyết định của database Management | [5.8.14](../05-building-blocks/08-components/14-management-database.md) |
| **IA1…IA13** | Điểm còn mở của hợp đồng Inspection API | [5.5](../05-building-blocks/05-contracts.md#inspection-api-l7--waf) |
| **C1…C4** | Phương án cell | [ADR-0041](../09-adr/0041-cells-and-zone-placement.md) |
| **Lớp A/B/C/D** | Lớp áp dụng cấu hình trên edge | [8.15](../08-cross-cutting/15-config-propagation-and-apply.md) |
| **SEV1, SEV2, SEV3** | Mức sự cố | [8.13](../08-cross-cutting/13-operational-behaviors.md) |
