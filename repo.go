package main

import (
    "net/http"
    "fmt"
	"github.com/maponet/utils/log"
	"bytes"
	"net"
	//"io"
	"encoding/base64"
	"strconv"
	"strings"
	//"time"
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

	// prendiamo il controllo della connessione in uscita
	conn, _, err := out.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(out, err.Error(), http.StatusInternalServerError)
		log.Debug("DEBUG: EventHandler() error %v\n", err)
		return
	}
	defer conn.Close()

	// inizio lettura e scrittura dei dati dalla connessione remota a quella locale
	data, chunked, bodySize := readHeaders(rConn)
	conn.Write(data)
	log.Debug("data=%x\nchunked=%v\nbodySize=%v", data, chunked, bodySize)
	if !chunked {
		body := make([]byte, bodySize)
		if bodySize > 0 {
			n, err := rConn.Read(body)
			//fmt.Printf("body=%x", body)
			if err != nil {
				fmt.Printf("header error=%v", err)
			}
			conn.Write(body[:n])
			for n < bodySize {
				bodySize = bodySize - n
				//fmt.Printf("\nread error 1_1 (read only %d to read %d bytes)\n", n, bodySize)
				tmp := make([]byte, bodySize)
				n, _ = rConn.Read(tmp)
				//fmt.Printf("\nread error 1_2 (read only %d to read %d bytes)\n", n, bodySize)
				conn.Write(tmp[:n])
			}
		}
	} else {

		// qui vengono letti i chunck se vengono usati
		for {
			data, chunckSize, lenSize := readChunck(rConn)
			conn.Write(data)

			for chunckSize > uint64(1430) {
				toRead := 1430 - lenSize
				chunckSize = chunckSize - uint64(toRead)
				chunck := make([]byte, toRead)
				lenSize = 0
				n, _ := rConn.Read(chunck)
				conn.Write(chunck[:n])
				for n < toRead {
					toRead = toRead - n
					//fmt.Printf("\nread error 2_1 (read only %d to read %d bytes)\n", n, toRead)
					tmp := make([]byte, toRead)
					n, _ = rConn.Read(tmp)
					//fmt.Printf("\nread error 2_2 (read only %d to read %d bytes)\n", n, toRead)
					conn.Write(tmp[:n])
				}
			}

			chunck := make([]byte, chunckSize+2)
			toRead := chunckSize + 2
			n, _ := rConn.Read(chunck)
			conn.Write(chunck[:n])
			for uint64(n) < toRead {
				toRead = toRead - uint64(n)
				//fmt.Printf("\nread error 3_1 (read only %d to read %d bytes)\n", n, toRead)
				tmp := make([]byte, toRead)
				n, _ = rConn.Read(tmp)
				//fmt.Printf("\nread error 3_2 (read only %d to read %d bytes)\n", n, toRead)
				conn.Write(tmp[:n])
			}

			if chunckSize == uint64(0) {
				break
			}
		}
	}
	return
}

// readHeaders identifica i header della risposta remota
func readHeaders(rconn net.Conn) ([]byte, bool, int) {
	singleByte := make([]byte, 1,1)
	headerData := make([]byte, 0)

	newLine := byte('\n')
	currentLine := make([]byte, 0)

	bodySize := 0
	chunked := false

	var err error

	r := strings.NewReplacer("Content-Length: ", "", "\r", "", "\n", "", "Transfer-Encoding: ", "")

	for {
		for {
			_, err = rconn.Read(singleByte)
			currentLine = append(currentLine, singleByte...)
			headerData = append(headerData, singleByte...)
			if err != nil {
				fmt.Printf("connection read error [%v]\n", err)
				break
			}
			if singleByte[0] == newLine {
				break
			}
		}
		currentListeString := string(currentLine)
		tmp := r.Replace(currentListeString)
		if len(tmp)+2 < len(currentListeString) {
			if tmp == "chunked" {
				chunked = true
			} else {
				bodySize, err = strconv.Atoi(tmp)
				if err != nil {
					fmt.Printf("can't convert %s to int [%v]\n", tmp, err)
				}
			}
		}
		if len(currentLine) <= 2 {
			break
		}
		currentLine = make([]byte, 0)
	}

	return headerData, chunked, bodySize
}

func readChunck(rconn net.Conn) ([]byte, uint64, int){
	singleByte := make([]byte, 1,1)
	currentLine := make([]byte, 0)
	var size uint64
	newLine := byte('\n')
	var err error

	for {
		n, err := rconn.Read(singleByte)
		if err != nil {
			if err != nil || n != 1 {
				fmt.Printf("connection read error [%v]\n", err)
				break
			}
		}
		currentLine = append(currentLine, singleByte[:n]...)
		if singleByte[0] == newLine {
			break
		}
	}

	//fmt.Printf("curent line hex=%x\n", currentLine)
	if len(currentLine) > 2 {
		//fmt.Printf("curent line -2 hex=%x\n", currentLine[:len(currentLine)-2])
		size, err = strconv.ParseUint(string(currentLine[:len(currentLine)-2]), 16, 64)
		if err != nil {
			fmt.Printf("can not get chunck size [%v)]\n", err)
		}
	} else {
		size = uint64(0)
	}
	lenSize := len(currentLine)
	//fmt.Printf("size string = %s, hex=%x\n",string(currentLine[len(currentLine)-2]), currentLine[len(currentLine)-2])
	//fmt.Printf("chunck size=%v\n", size)

	return currentLine, size, lenSize
}
