package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttprouter"
)

var RRDRootPath string = "/empty"

func init() {
	flag.StringVar(&RRDRootPath, "rrd-dir", "./", "Allow queries of rrd files under this path.")
}

var (
	listenAddr = flag.String("listen-address", "127.0.0.1:9191", "Address to listen on for http requests.")
	compress   = flag.Bool("compress", false, "Enable transparent response compression.")
)

func requestError(ctx *fasthttp.RequestCtx, err error) {
	fmt.Fprintf(ctx, "%s", err)
	ctx.SetStatusCode(400)
	ctx.SetContentType("text/plain; charset=utf8")
}

func xportHandler(ctx *fasthttp.RequestCtx, _ fasthttprouter.Params) {
	qargs := ctx.QueryArgs()

	log.Printf("got xport request: %v", qargs)

	var buf bytes.Buffer
	for i, queryPart := range qargs.PeekMulti("q") {
		if i != 0 {
			buf.WriteByte(' ')
		}
		buf.Write(queryPart)
	}

	cleanedArgs, err := CleanXport(buf.String(), CleanOpts{RRDRootPath: RRDRootPath})
	if err != nil {
		requestError(ctx, err)
		return
	}

	if len(cleanedArgs) == 0 {
		requestError(ctx, errors.New("empty query"))
		return
	}

	fullCmdArgs := []string{"xport"}

	wantJson := true
	formatBytes := qargs.Peek("format")
	if formatBytes != nil {
		switch string(formatBytes) {
		case "json":
			wantJson = true
		case "xml":
			wantJson = false
		default:
			requestError(ctx, fmt.Errorf("invalid format"))
		}
	}
	if wantJson {
		fullCmdArgs = append(fullCmdArgs, "--json")
	}

	startBytes := qargs.Peek("start")
	stepBytes := qargs.Peek("step")
	endBytes := qargs.Peek("end")

	if len(startBytes) != 0 {
		fullCmdArgs = append(fullCmdArgs, "--start", string(startBytes))
	}
	if len(endBytes) != 0 {
		fullCmdArgs = append(fullCmdArgs, "--end", string(endBytes))
	}
	if len(stepBytes) != 0 {
		fullCmdArgs = append(fullCmdArgs, "--step", string(stepBytes))
	}
	fullCmdArgs = append(fullCmdArgs, "--")
	fullCmdArgs = append(fullCmdArgs, cleanedArgs...)

	log.Printf("running rrdtool: %v", fullCmdArgs)

	cmd := exec.Command("rrdtool", fullCmdArgs...)

	out, err := cmd.Output()
	if err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			requestError(ctx, fmt.Errorf("%s", string(err.Stderr)))
		} else {
			requestError(ctx, fmt.Errorf("query failed: %s", err))
		}
		return
	}

	ctx.Write(out)
	if wantJson {
		ctx.SetContentType("application/json; charset=utf8")
	} else {
		ctx.SetContentType("application/xml; charset=utf8")
	}
}

func pingHandler(ctx *fasthttp.RequestCtx, _ fasthttprouter.Params) {
	log.Printf("got ping from %s@%s", ctx.UserAgent(), ctx.RemoteIP())
	io.WriteString(ctx, "\"pong\"")
	ctx.SetContentType("text/json; charset=utf8")
}

var indexHTML string = `
<html>
<body>
<style> a { text-decoration: none }</style>

<h1>rrdsrv</h1>

API:

<ul>
  <li><a href="/api/v1/ping">/api/v1/ping</a></li>
  <li><a href="/api/v1/xport">/api/v1/xport?q=$query[&amp;format=$format&amp;start=$start&amp;end=$end&amp;step=$step]</a></li>
</ul> 

Make an export query:

<form action="/api/v1/xport" accept-charset="UTF-8">
  <textarea name="q" cols="80" rows="3"></textarea>
  <br>
  <button type="submit">export</button>
</form>

</body>
</html>
`

func indexHandler(ctx *fasthttp.RequestCtx, _ fasthttprouter.Params) {
	io.WriteString(ctx, indexHTML)
	ctx.SetContentType("text/html; charset=utf8")
}

func main() {
	var err error

	flag.Parse()

	RRDRootPath, err = filepath.Abs(RRDRootPath)
	if err != nil {
		log.Fatalf("unable to get the absolute path of %q", RRDRootPath)
	}

	router := fasthttprouter.New()
	router.GET("/", indexHandler)
	router.GET("/api/v1/ping", pingHandler)
	router.GET("/api/v1/xport", xportHandler)

	h := router.Handler
	if *compress {
		h = fasthttp.CompressHandler(router.Handler)
	}

	log.Printf("listening on %s", *listenAddr)
	log.Printf("serving rrds under %s", RRDRootPath)
	if err := fasthttp.ListenAndServe(*listenAddr, h); err != nil {
		log.Fatalf("error in ListenAndServe: %s", err)
	}
}
