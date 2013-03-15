package main

import (
    "net/http"
    "fmt"
	"github.com/maponet/utils/log"
	"bytes"
	"net"
	"io"
	"encoding/base64"
	"time"
)

func repoProxy(out http.ResponseWriter, in *http.Request) {

// -----> remote request
	//var username, password string
	//username = "default"
	//password = "defaultdefault"

	remoteUrl := "opam.kino3d.org:80"

	log.SetLevel("ERROR")

	remoteHost := remoteUrl
	getHeader := fmt.Sprintf("GET %s HTTP/1.1\n", in.RequestURI)
	log.Debug("GET %s HTTP/1.1", in.RequestURI)

	rConn, err := net.Dial("tcp", remoteHost)
	if err != nil {
		panic(err)
	}
	defer rConn.Close()

	bufferLength := 1430

	encoder := base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/")
	encodedString := encoder.EncodeToString([]byte("default:defaultdefault"))

	buf := &bytes.Buffer{}
	buf.Write([]byte(getHeader))
	buf.Write([]byte(fmt.Sprintf("Host: %s\n", remoteUrl)))
	buf.Write([]byte(fmt.Sprintf("Authorization: Basic %s\n", encodedString)))
	for k, _ := range in.Header {
		buf.Write([]byte(fmt.Sprintf("%s: %s\n", k, in.Header.Get(k))))
	}
	buf.Write([]byte(fmt.Sprintf("\n")))

	if _, err := rConn.Write(buf.Bytes()); err != nil {
		panic(err)
	}

	conn, _, err := out.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(out, err.Error(), http.StatusInternalServerError)
		log.Debug("DEBUG: EventHandler() error %v\n", err)
		return
	}
	defer conn.Close()

	//bufResult := &bytes.Buffer{}
	//count := 0
	for {
		data := make([]byte, bufferLength)
		rConn.SetReadDeadline(time.Now().Add(3000*time.Millisecond))
		n, err:= rConn.Read(data)
		//log.Debug(string(data[:n]))
		//bufResult.Write(data[:n])
		conn.Write(data[:n])
		//if n > 1 {
		//	if (data[n-2] == 13 && data[n-1] == 10) || (n < bufferLength) {
		//	//if (n < bufferLength) {
		//		log.Debug("found end of body")
		//		break
		//	}
		//}
		if err != nil {
			if err != io.EOF {
				//to, ok := err.(net.Error)
				//if ok {
				//	if to.Timeout() {
				//		log.Error("time out (%v), retrying ...", err)
				//		if count >= 5 {
				//			break
				//		}
				//		count++
				//		//break
				//		continue
				//	}
				//}
				log.Debug("%v", err)
				break
			} else {
				log.Debug("found end of file")
				break
			}
		}
	}

}
