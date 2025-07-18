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


start forgejo with podman:

``` shell
podman run --replace -d --name forgejo -p 3000:3000 -p 2222:22 -v /opt/compose/forgejo:/data -v /etc/timezone:/etc/timezone:ro -v /etc/localtime:/etc/localtime:ro codeberg.org/forgejo/forgejo:12.0.0
```

health check:

``` shell
curl -f "http://localhost:3000/user/login"
```
