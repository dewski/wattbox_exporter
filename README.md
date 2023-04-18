# wattbox_exporter

Update `WATTBOX_HOST` with the IP address to your Wattbox. Make sure to set a static IP address for your Wattbox.

```
docker run -p 8181:8181 -e WATTBOX_HOST="1.1.1.1" -e WATTBOX_USER="username" -e WATTBOX_PASSWORD="password" -e POLL_DURATION="30s" dewski/wattbox_exporter:latest
```
