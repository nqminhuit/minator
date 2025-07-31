# Minator

**Minator** is a lightweight monitoring and alerting tool for containerized services and infrastructure, built in Go. It provides periodic health checks, dashboard visualization, and alerting for services like Forgejo, PostgreSQL, hardware usage, backups, and more.

## Features

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

## Quickstart

Start all base services (the ones that need monitored)

``` shell
make infra
```

## Running Minator

You can run the monitoring server in development mode:

```shell
make
```

See available Makefile commands:

```shell
make help
```

## Usage

- Access the web dashboard at `http://localhost:18080/status` (default port).
- Service statuses are checked periodically (see `monitor/monitor.go`) and visualized in the dashboard.
- Status is persisted in `/tmp/status.json`.
- API to send health status to Minator:

``` shell
curl "localhost:18080/api/service/status" \
    -H "content-type:application/json" \
    -d '{
        "name": "Backup",
        "status": "inprogress",
        "details": {
            "previousBackup":"Forgejo",
            "currentBackup":"PostgreSQL"
        }
    }'
```

- for siplicity, a target was introduced in Makefile, simply call:

``` shell
make sample-test
```

## Configuration

- Change port by setting the `PORT` environment variable.
- Configure monitored services in the `monitor` package (add new checks as needed).

## License

This project does not yet specify a license.  
Please add a license file appropriate for your use-case.

## Contact

Maintainer: [nqminhuit](https://github.com/nqminhuit)
