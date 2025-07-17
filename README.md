| Feature                  | Mechanism                                                                 |
| ------------------------ | ------------------------------------------------------------------------- |
| Container service checks | Ping service via TCP/Unix socket, or check Podman container state via CLI |
| Backup process check     | Backup script `curl http://localhost:8080/api/backup/status -X POST`      |
| Disk usage monitoring    | Use `syscall.Statfs` or `os/exec("df")`                                   |
| Last backup check        |                                                                           |
| Web dashboard            | Serve basic HTML page with `/status` route                                |
| Email alerting           | Use `net/smtp`                                                            |
| Cron-like jobs           | `time.Ticker` in goroutines                                               |
| Persistence              | JSON file (for backup history etc.)                                       |
