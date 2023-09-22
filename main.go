package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
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
		log.Fatalf("init docker client error: %v", err)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/logs", handleLogs)

	log.Printf("start to listen on: [%v]", PORT)
	log.Fatal(http.ListenAndServe(PORT, mux))
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	var (
		Cid    string
		Follow bool   = true
		Since  string = ""
		Tail   string = "all"
		Until  string = ""
	)

	if Cid = r.URL.Query().Get("cid"); Cid == "" {
		http.Error(w, fmt.Sprintf("error: %v.", "container id is not present"), http.StatusBadRequest)
		return
	}
	if r.URL.Query().Has("follow") {
		Follow, _ = strconv.ParseBool(r.URL.Query().Get("follow"))
	}
	if r.URL.Query().Has("since") {
		Since = r.URL.Query().Get("since")
	}
	if r.URL.Query().Has("tail") {
		Tail = r.URL.Query().Get("tail")
	}
	if r.URL.Query().Has("until") {
		Until = r.URL.Query().Get("until")
	}
	err := readLogs(w, Cid, Follow, Since, Tail, Until)
	if err != nil {
		http.Error(w, fmt.Sprintf("error: %v.", err), http.StatusInternalServerError)
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

	log.Printf("start docker container (id: %v) logs with opts: %v", cid, options)

	// refer:
	// - https://github.com/gin-gonic/examples/blob/master/send_chunked_data/send_chunked_data.go
	// - https://github.com/google/gvisor/blob/master/pkg/linewriter/linewriter.go
	// - https://github.com/docker/cli/blob/master/cli/command/container/logs.go

	writer.Header().Set("Transfer-Encoding", "chunked")
	writer.Header().Set("Content-Type", "text/plain")

	resp, _ := dockerClient.ContainerInspect(cxt, cid)
	if resp.Config.Tty {
		// _, err = io.Copy(os.Stdout, responseBody)
		_, err = io.Copy(NewFlushLineWriter(writer), responseBody)
		return err
	} else {
		// _, err = stdcopy.StdCopy(os.Stdout, os.Stderr, responseBody)
		_, err = stdcopy.StdCopy(NewFlushLineWriter(writer), NewFlushLineWriter(writer), responseBody)
		return err
	}
}

type FlushLineWriter struct {
	Out io.Writer
}

func NewFlushLineWriter(writer io.Writer) io.Writer {
	return &FlushLineWriter{
		Out: writer,
	}
}

func (w *FlushLineWriter) Write(p []byte) (n int, err error) {
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
