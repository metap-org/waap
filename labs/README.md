0 - trước khi execute cần update deployment, network topology trong tài liệu kiến trúc
1 - dựng hạ tầng lab triển khai 4 plane
2 - network giữa các plane phải cô lập, các giao tiếp giữa các plane phải đảm bảo security. chia public + private network cẩn thận.
3 - setup luồng gitops, devsecops trong mỗi plane
4 - các service cần chạy bare metal hay gì thì cần phải có bộ cài riêng kiểu như network như nào, hệ điều hành gì, phần mềm gì phân quyền như nào, disk chia những gì, hardening đầy đủ và có chuẩn chung.
5 - đảm bảo phải có secret manager, tư động rollout secret tùy nghiệp vụ, hạ tầng.
