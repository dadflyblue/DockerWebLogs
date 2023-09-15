package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gin-gonic/gin"
)

var (
	DOCKER_SOCKET string
	PORT          string
	dockerClient  *client.Client
	cxt           = context.Background()
)

func init() {
	DOCKER_SOCKET = readEnvString("DOCKER_SOCKET", "/var/run/docker.sock")
	PORT = readEnvString("PORT", ":9000")

	client, err := client.NewClientWithOpts(
		client.WithDialContext(func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", DOCKER_SOCKET)
		}),
	)
	if err == nil {
		dockerClient = client
	} else {
		log.Default().Fatalf("init docker client error: %v", err)
	}
}

func main() {
	r := gin.Default()
	r.GET("/logs", logs)

	log.Default().Fatal(http.ListenAndServe(PORT, r))
}

type ContainerIdBasedReq struct {
	ContainerId string `form:"cid" binding:"required"`
}

type ContainerLogsReq struct {
	ContainerIdBasedReq
	Follow bool   `form:"follow,default=true"`
	Since  string `form:"since"`
	Tail   string `form:"tail,default=all"`
	Until  string `form:"until"`
}

func logs(c *gin.Context) {
	var p ContainerLogsReq
	if err := c.ShouldBindQuery(&p); err != nil {
		c.Status(http.StatusBadRequest)
	} else {
		err := readLogs(c.Writer, p.ContainerId, p.Follow, p.Since, p.Tail, p.Until)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
	}
}

func readEnvString(key string, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	} else {
		return def
	}
}

func readLogs(writer http.ResponseWriter, cid string, follow bool, since string, tail string, until string) error {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Since:      since,
		Until:      until,
		Timestamps: false,
		Follow:     follow,
		Tail:       tail,
		Details:    false,
	}
	responseBody, err := dockerClient.ContainerLogs(cxt, cid, options)
	if err != nil {
		return err
	}
	defer responseBody.Close()

	log.Default().Printf("start to read docker logs with opt: %v", options)

	// refer:
	// - https://github.com/gin-gonic/examples/blob/master/send_chunked_data/send_chunked_data.go
	// - https://github.com/google/gvisor/blob/master/pkg/linewriter/linewriter.go
	// - https://github.com/docker/cli/blob/master/cli/command/container/logs.go

	writer.Header().Set("Transfer-Encoding", "chunked")
	writer.Header().Set("Content-Type", "text/plain")
	writer.WriteHeader(http.StatusOK)

	resp, _ := dockerClient.ContainerInspect(cxt, cid)
	if resp.Config.Tty {
		// _, err = io.Copy(os.Stdout, responseBody)
		_, err = io.Copy(newLineWriter(writer), responseBody)
		return err
	} else {
		// _, err = stdcopy.StdCopy(os.Stdout, os.Stderr, responseBody)
		_, err = stdcopy.StdCopy(newLineWriter(writer), newLineWriter(writer), responseBody)
		return err
	}
}

type LineWriter struct {
	Out io.Writer
}

func newLineWriter(writer io.Writer) io.Writer {
	return &LineWriter{
		Out: writer,
	}
}

func (w *LineWriter) Write(p []byte) (n int, err error) {
	total := 0
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 { // no line in this buff
			return w.Out.Write(p)
		}

		i++ // output includes \n
		n, err := w.Out.Write(p[:i])
		if err != nil {
			return total, err
		}
		total += n
		w.Out.(http.Flusher).Flush() // fush new line

		p = p[i:]
	}
	return total, nil
}
