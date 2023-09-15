# DockerWebLogs
It simply texts docker logs to web console.

## How to run it

Firstly use command line: `go run .` or `go build && ./DockerWebLogs`
Then open a brower and type: http://localhost:90000/logs?cid=xxx> or user cURL: `curl -v "http://localhost:9000/logs?cid=xxx" --output -`. Note: change the 'xxx" to a real container id.

![screenshot-brower](screenshots/browser.png)
![screenshot-cmdline](screenshots/cURL.png)
